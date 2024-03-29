export {}

declare var hdemo: HTMLElement
declare var herror: HTMLElement, husers: HTMLElement
declare var hloginname: HTMLInputElement, hloginroom: HTMLInputElement, hloginjoin: HTMLInputElement
declare var hsend: HTMLInputElement, hmessage: HTMLInputElement, hmessages: HTMLElement

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
`

const signalingServer = 'https://iio.ie/sig'

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
]

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
]

const rtcConfig = {
  iceServers: [{
    urls: 'stun:stun.l.google.com:19302',
  }]
}

class client {
  username: string
  conn: RTCPeerConnection
  channel: RTCDataChannel

  constructor(u: string, conn: RTCPeerConnection, ch: RTCDataChannel) {
    this.username = u
    this.conn = conn
    this.channel = ch
  }
}

let clients: client[] = []

function reportError(msg: string) {
  if (msg == '') {
    herror.hidden = true
  } else {
    herror.innerText = msg
    herror.hidden = false
  }
}

// convert continuation passing style into direct style:
// await eventPromise(obj, 'click') will wait for a single click on obj.
function eventPromise(obj: EventTarget, eventName: string) {
  return new Promise(resolve => {
    let handler = (event: Event) => {
      obj.removeEventListener(eventName, handler)
      resolve(event)
    }
    obj.addEventListener(eventName, handler);
  })
}

function pruneDisconnected() {
  for (let i = 0; i < clients.length; i++) {
    if (['disconnected', 'closed'].includes(clients[i].conn.iceConnectionState) == false) continue
    let user = clients[i].username
    clients[i] = clients[clients.length - 1]
    clients.pop()
    i--
    distributeMessage(`${user} disconnected.`)
  }
}

function onmsgKeyup(ev: KeyboardEvent) {
  if (ev.key == 'Enter') sendmessage()
}

function sendmessage() {
  let msg = hmessage.value
  if (msg.length == 0) return
  if (serverChannel != null) {
    serverChannel.send(msg)
  } else {
    distributeMessage(`${hloginname.value}: ${msg}`)
  }
  hmessage.value = ''
}

let active = new Set()

function addmessage(msg: string) {
  let atbottom = Math.abs(hmessages.scrollTop - (hmessages.scrollHeight - hmessages.offsetHeight)) < 1
  hmessages.innerText += msg + '\n'
  if (atbottom) hmessages.scrollTo(0, hmessages.scrollHeight)
  let [user, op, rest] = msg.split(' ')
  if (user == '--') return;
  if (user.endsWith(':')) return;
  if (op == 'disconnected.') {
    active.delete(user)
  } else {
    active.add(user)
  }
  husers.innerText = Array.from(active.keys()).sort().join(', ')
}

function randval(a: string[]) {
  return a[Math.floor(Math.random() * a.length)]
}

let msgHistory: string[]

function distributeMessage(msg: string) {
  msgHistory.push(msg)
  addmessage(msg)
  for (let c of clients) c.channel.send(msg)
}

let aborter: AbortController | null

let usernameRE = /^[a-z0-9_.-]{1,32}$/

async function server() {
  msgHistory = []
  let room = hloginroom.value
  distributeMessage(`${hloginname.value} created the room ${room}.`)
  aborter = new AbortController()
  let failedAttempts = 0
  while (true) {
    // create a description offer and upload it to the signaling service.
    let conn = new RTCPeerConnection(rtcConfig)
    let channel = conn.createDataChannel('datachannel')
    conn.setLocalDescription(await conn.createOffer())
    do {
      await eventPromise(conn, 'icegatheringstatechange')
    } while (conn.iceGatheringState != 'complete');
    let response
    try {
      response = await fetch(`${signalingServer}?set=chatoffer_${room}`, {
        method: 'POST',
        body: conn?.localDescription?.sdp,
        signal: aborter.signal,
      })
    } catch (e) {
      if (aborter.signal.aborted) {
        conn.close()
        break
      }
      if (failedAttempts == 6) throw e
      reportError(`temporarily unavailable (attempt ${failedAttempts}): ` + e)
      await new Promise(f => setTimeout(f, failedAttempts++ * 1000));
      reportError('')
      continue
    }
    failedAttempts = 0
    if (response.status == 204) {
      conn.close()
      continue
    }

    // read the description answer from the next client.
    response = await fetch(`${signalingServer}?get=chatanswer_${room}&timeoutms=500`, {
      method: 'POST',
    })
    if (response.status == 204) {
      conn.close()
      continue
    }
    let sdp = await response.text()
    conn.setRemoteDescription({
      type: 'answer',
      sdp: sdp,
    })
    await eventPromise(channel, 'open')

    // read the username, setup event handlers, notify other clients.
    let username = (await eventPromise(channel, 'message') as MessageEvent).data
    if (!username.match(usernameRE)) {
      channel.send('-- username invalid, connect rejected. --')
      conn.close()
      continue
    }
    if (active.has(username)) {
      channel.send('-- username taken, connect rejected. --')
      conn.close()
      continue
    }
    conn.oniceconnectionstatechange = pruneDisconnected
    channel.onmessage = (ev) => {
      if (ev.data == '/leave') {
        conn.close()
        pruneDisconnected()
      } else {
        distributeMessage(`${username}: ${ev.data}`)
      }
    }
    for (let m of msgHistory) channel.send(m)
    clients.push(new client(username, conn, channel))
    distributeMessage(`${username} joined.`)
  }
}

let serverConn: RTCPeerConnection | null
let serverChannel: RTCDataChannel | null

async function join() {
  if (!hloginname.value.match(usernameRE)) {
    hmessages.innerText = 'invalid username, pick a short alphanumeric identifier.'
    return
  }
  if (!hloginroom.value.match(/^[a-z0-9_-]{1,16}$/)) {
    hmessages.innerText = 'invalid room name, pick a short alphanumeric identifier.'
    return
  }

  // update the ui.
  hloginname.disabled = true
  hloginroom.disabled = true
  hloginjoin.disabled = true
  hloginjoin.innerText = 'joining...'
  hsend.disabled = false
  hmessages.innerText = ''
  hmessage.disabled = false
  hmessage.focus()

  let room = hloginroom.value
  let response = await fetch(`${signalingServer}?get=chatoffer_${room}&timeoutms=900`, {
    method: 'POST',
  })
  if (response.status == 204) {
    // timed out means there is no running server, become a server then.
    hloginjoin.disabled = false
    hloginjoin.innerText = 'close room'
    hloginjoin.onclick = closeRoom
    return server()
  }
  if (response.status != 200) {
    reportError(`unexpected response: ${response.status}`)
    return
  }

  // assuming client mode.
  // send description answer to the server.
  let offer = await response.text()
  serverConn = new RTCPeerConnection(rtcConfig)
  let channelPromise = eventPromise(serverConn, 'datachannel')
  await serverConn.setRemoteDescription({
    type: 'offer',
    sdp: offer,
  })
  serverConn.setLocalDescription(await serverConn.createAnswer())
  do {
    await eventPromise(serverConn, 'icegatheringstatechange')
  } while (serverConn.iceGatheringState != 'complete');
  response = await fetch(`${signalingServer}?set=chatanswer_${room}`, {
    method: 'POST',
    body: serverConn?.localDescription?.sdp,
  })
  serverChannel = (await channelPromise as RTCDataChannelEvent).channel

  // set up event handlers for the connection.
  serverConn.oniceconnectionstatechange = (ev) => {
    if (serverConn?.iceConnectionState != 'disconnected') return
    addmessage('-- server disconnected --')
    leave()
  }
  serverChannel.onmessage = ev => {
    let msg = (ev as MessageEvent).data
    addmessage(msg)
    if (msg == '-- chatroom closed --') leave()
    if (msg == '-- username taken, connect rejected. --') leave()
    if (msg == '-- username invalid, connect rejected. --') leave()
  }

  // send over the username as the first message.
  serverChannel.send(hloginname.value)
  hloginjoin.disabled = false
  hloginjoin.innerText = 'leave'
  hloginjoin.onclick = leave
}

function leave() {
  if (serverConn != null) {
    serverChannel?.send('/leave')
    serverConn.close()
    serverConn = null
    addmessage(`-- left the room ${hloginroom.value} --`)
  }
  hloginname.disabled = false
  hloginroom.disabled = false
  husers.innerText = ''
  active = new Set()
  hloginjoin.innerText = 'join'
  hloginjoin.onclick = join
  hsend.disabled = true
  hmessage.disabled = true
}

function closeRoom() {
  if (aborter != null) aborter.abort()
  let msg = '-- chatroom closed --'
  addmessage(msg)
  for (let c of clients) {
    c.channel.send(msg)
    c.conn.close()
  }
  clients = []
  leave()
}

function closeall() {
  if (hloginjoin.innerText == 'close room') closeRoom()
  if (hloginjoin.innerText == 'leave') leave()
}

function main() {
  try {
    let c = new RTCPeerConnection()
    c.close()
  } catch (e) {
    hdemo.innerHTML = '<p id=herror style=color:red hidden></p>'
    reportError('no support for webrtc in your browser: ' + e)
    return
  }

  window.onbeforeunload = closeall
  window.onerror = (msg, src, line) => reportError(`${src}:${line} ${msg}`)
  window.onunhandledrejection = e => reportError(e.reason)
  hdemo.innerHTML = chatui
  hloginname.value = `${randval(colors)}-${randval(animals)}`
  hloginroom.value = 'default'
  hloginjoin.onclick = join
  hmessage.onkeyup = onmsgKeyup
  hsend.onclick = sendmessage
}

main()
