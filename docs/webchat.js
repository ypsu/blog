"use strict";
let chatui = `
<p id=herror style=color:red hidden></p>
<p>
  your name: <input id=hloginname style=max-width:10em>,
  room: <input id=hloginroom style=max-width:10em>
  <button id=hloginjoin>join</button>
</p>
<div id=hchat>
  <p>users: <span id=husers></span></p>
  <pre id=hmessages style="height:12em;overflow:scroll"></pre>
  <div style="width:100%;overflow:hidden">
    <button id=hsend style="float:left;margin:0.5em" disabled>send</button>
    <div style=overflow:hidden>
      <input id=hmessage placeholder=message style="width:90%;margin:0.5em" enterkeyhint=send disabled>
    </div>
  </div>
</div>
`;
const signalingServer = 'https://notech.ie/sig';
const colors = [
    'black',
    'blue',
    'brown',
    'cyan',
    'gold',
    'gray',
    'green',
    'magenta',
    'orange',
    'pink',
    'purple',
    'red',
    'silver',
    'white',
    'yellow',
];
const animals = [
    'bear',
    'cat',
    'chicken',
    'cow',
    'deer',
    'dog',
    'fox',
    'hamster',
    'mouse',
    'panda',
    'pig',
    'rabbit',
    'rat',
    'tiger',
];
const rtcConfig = {
    iceServers: [{
            urls: 'stun:stun.l.google.com:19302',
        }]
};
class client {
    username;
    conn;
    channel;
    constructor(u, conn, ch) {
        this.username = u;
        this.conn = conn;
        this.channel = ch;
    }
}
let clients = [];
function reportError(msg) {
    herror.innerText = msg;
    herror.hidden = false;
}
// convert continuation passing style into direct style:
// await eventPromise(obj, 'click') will wait for a single click on obj.
function eventPromise(obj, eventName) {
    return new Promise(resolve => {
        let handler = (event) => {
            obj.removeEventListener(eventName, handler);
            resolve(event);
        };
        obj.addEventListener(eventName, handler);
    });
}
function pruneDisconnected() {
    for (let i = 0; i < clients.length; i++) {
        if (['disconnected', 'closed'].includes(clients[i].conn.iceConnectionState) == false)
            continue;
        let user = clients[i].username;
        clients[i] = clients[clients.length - 1];
        clients.pop();
        i--;
        distributeMessage(`${user} disconnected.`);
    }
}
function onmsgKeyup(ev) {
    if (ev.key == 'Enter')
        sendmessage();
}
function sendmessage() {
    let msg = hmessage.value;
    if (msg.length == 0)
        return;
    if (serverChannel != null) {
        serverChannel.send(msg);
    }
    else {
        distributeMessage(`${hloginname.value}: ${msg}`);
    }
    hmessage.value = '';
}
let active = new Set();
function addmessage(msg) {
    let atbottom = Math.abs(hmessages.scrollTop - (hmessages.scrollHeight - hmessages.offsetHeight)) < 1;
    hmessages.innerText += msg + '\n';
    if (atbottom)
        hmessages.scrollTo(0, hmessages.scrollHeight);
    let [user, op, rest] = msg.split(' ');
    if (user == '--')
        return;
    if (user.endsWith(':'))
        return;
    if (op == 'disconnected.') {
        active.delete(user);
    }
    else {
        active.add(user);
    }
    husers.innerText = Array.from(active.keys()).sort().join(', ');
}
function randval(a) {
    return a[Math.floor(Math.random() * a.length)];
}
let msgHistory;
function distributeMessage(msg) {
    msgHistory.push(msg);
    addmessage(msg);
    for (let c of clients)
        c.channel.send(msg);
}
let aborter;
let usernameRE = /^[a-z0-9_.-]{1,32}$/;
async function server() {
    msgHistory = [];
    let room = hloginroom.value;
    distributeMessage(`${hloginname.value} created the room ${room}.`);
    aborter = new AbortController();
    while (true) {
        // create a description offer and upload it to the signaling service.
        let conn = new RTCPeerConnection(rtcConfig);
        let channel = conn.createDataChannel('datachannel');
        conn.setLocalDescription(await conn.createOffer());
        do {
            await eventPromise(conn, 'icegatheringstatechange');
        } while (conn.iceGatheringState != 'complete');
        let response = await fetch(`${signalingServer}?name=chatoffer_${room}`, {
            method: 'POST',
            body: conn?.localDescription?.sdp,
            signal: aborter.signal,
        });
        if (aborter.signal.aborted) {
            conn.close();
            break;
        }
        if (response.status == 204) {
            conn.close();
            continue;
        }
        // read the description answer from the next client.
        response = await fetch(`${signalingServer}?name=chatanswer_${room}&timeoutms=500`);
        if (response.status == 204) {
            conn.close();
            continue;
        }
        let sdp = await response.text();
        conn.setRemoteDescription({
            type: 'answer',
            sdp: sdp,
        });
        await eventPromise(channel, 'open');
        // read the username, setup event handlers, notify other clients.
        let username = (await eventPromise(channel, 'message')).data;
        if (!username.match(usernameRE)) {
            channel.send('-- username invalid, connect rejected. --');
            conn.close();
            continue;
        }
        if (active.has(username)) {
            channel.send('-- username taken, connect rejected. --');
            conn.close();
            continue;
        }
        conn.oniceconnectionstatechange = pruneDisconnected;
        channel.onmessage = (ev) => {
            if (ev.data == '/leave') {
                conn.close();
                pruneDisconnected();
            }
            else {
                distributeMessage(`${username}: ${ev.data}`);
            }
        };
        for (let m of msgHistory)
            channel.send(m);
        clients.push(new client(username, conn, channel));
        distributeMessage(`${username} joined.`);
    }
}
let serverConn;
let serverChannel;
async function join() {
    if (!hloginname.value.match(usernameRE)) {
        hmessages.innerText = 'invalid username, pick a short alphanumeric identifier.';
        return;
    }
    if (!hloginroom.value.match(/^[a-z0-9_-]{1,16}$/)) {
        hmessages.innerText = 'invalid room name, pick a short alphanumeric identifier.';
        return;
    }
    // update the ui.
    hloginname.disabled = true;
    hloginroom.disabled = true;
    hloginjoin.disabled = true;
    hloginjoin.innerText = 'joining...';
    hsend.disabled = false;
    hmessages.innerText = '';
    hmessage.disabled = false;
    hmessage.focus();
    let room = hloginroom.value;
    let response = await fetch(`${signalingServer}?name=chatoffer_${room}&timeoutms=900`);
    if (response.status == 204) {
        // timed out means there is no running server, become a server then.
        hloginjoin.disabled = false;
        hloginjoin.innerText = 'close room';
        hloginjoin.onclick = closeRoom;
        return server();
    }
    if (response.status != 200) {
        reportError(`unexpected response: ${response.status}`);
        return;
    }
    // assuming client mode.
    // send description answer to the server.
    let offer = await response.text();
    serverConn = new RTCPeerConnection(rtcConfig);
    let channelPromise = eventPromise(serverConn, 'datachannel');
    await serverConn.setRemoteDescription({
        type: 'offer',
        sdp: offer,
    });
    serverConn.setLocalDescription(await serverConn.createAnswer());
    do {
        await eventPromise(serverConn, 'icegatheringstatechange');
    } while (serverConn.iceGatheringState != 'complete');
    response = await fetch(`${signalingServer}?name=chatanswer_${room}`, {
        method: 'POST',
        body: serverConn?.localDescription?.sdp,
    });
    serverChannel = (await channelPromise).channel;
    // set up event handlers for the connection.
    serverConn.oniceconnectionstatechange = (ev) => {
        if (serverConn?.iceConnectionState != 'disconnected')
            return;
        addmessage('-- server disconnected --');
        leave();
    };
    serverChannel.onmessage = ev => {
        let msg = ev.data;
        addmessage(msg);
        if (msg == '-- chatroom closed --')
            leave();
        if (msg == '-- username taken, connect rejected. --')
            leave();
        if (msg == '-- username invalid, connect rejected. --')
            leave();
    };
    // send over the username as the first message.
    serverChannel.send(hloginname.value);
    hloginjoin.disabled = false;
    hloginjoin.innerText = 'leave';
    hloginjoin.onclick = leave;
}
function leave() {
    if (serverConn != null) {
        serverChannel?.send('/leave');
        serverConn.close();
        serverConn = null;
        addmessage(`-- left the room ${hloginroom.value} --`);
    }
    hloginname.disabled = false;
    hloginroom.disabled = false;
    husers.innerText = '';
    active = new Set();
    hloginjoin.innerText = 'join';
    hloginjoin.onclick = join;
    hsend.disabled = true;
    hmessage.disabled = true;
}
function closeRoom() {
    if (aborter != null)
        aborter.abort();
    let msg = '-- chatroom closed --';
    addmessage(msg);
    for (let c of clients) {
        c.channel.send(msg);
        c.conn.close();
    }
    clients = [];
    leave();
}
function closeall() {
    if (hloginjoin.innerText == 'close room')
        closeRoom();
    if (hloginjoin.innerText == 'leave')
        leave();
}
function main() {
    try {
        let c = new RTCPeerConnection();
        c.close();
    }
    catch (e) {
        hdemo.innerHTML = '<p id=herror style=color:red hidden></p>';
        reportError('no support for webrtc in your browser: ' + e);
        return;
    }
    window.onbeforeunload = closeall;
    hdemo.innerHTML = chatui;
    hloginname.value = `${randval(colors)}-${randval(animals)}`;
    hloginroom.value = 'default';
    hloginjoin.onclick = join;
    hmessage.onkeyup = onmsgKeyup;
    hsend.onclick = sendmessage;
}
main();
