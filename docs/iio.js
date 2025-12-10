export { iio, iioui, strings };
let strings = {
    // Cut slices s around the first instance of sep, returning the text before and after it.
    Cut: function (s, sep) {
        if (!s)
            return ["", ""];
        let i = s.indexOf(sep);
        if (i == -1)
            return [s, ""];
        return [s.substring(0, i), s.substring(i + sep.length)];
    },
    FormatDate: function (date, sep) {
        if (!sep)
            sep = "-";
        let year = `${date.getUTCFullYear()}`;
        let month = `${date.getUTCMonth() + 1}`.padStart(2, "0");
        let day = `${date.getUTCDate()}`.padStart(2, "0");
        return year + sep + month + sep + day;
    },
};
let iio = {
    // Panic notifies the user about the passed in error.
    Panic: function (msg) {
        let s = `${msg}`;
        eError.innerText = `Error: ${s.trim()}\n\nReload the page to try again.`;
        eError.hidden = false;
        document.body.classList.add("cbgNeutral");
    },
    // Run runs run() and panics if it returns an error.
    Run: async function (run) {
        let err = await run();
        if (err != "")
            iio.Panic(err);
    },
    // Fetch returns body or error string.
    // The error string is empty iff there was no error.
    Fetch: async function (input, options) {
        let req = new Request(input, options);
        try {
            let response = await fetch(req);
            let text = await response.text();
            if (response.status >= 300)
                return Promise.resolve(["", `${response.status} ${response.statusText}: ${text.trim()}`]);
            return Promise.resolve([text, ""]);
        }
        catch (e) {
            return ["", `iio.FetchException: ${e}`.trim()];
        }
    },
    // RegisterGuest creates a new guest account.
    RegisterGuest: async function () {
        let [result, err] = await iio.Fetch("/userapi?action=registerguest", { method: "POST" });
        if (err != "")
            return err;
        iio.User = result.trim();
        renderAccountpageLink();
        return Promise.resolve("");
    },
    Init: function () {
        window.onerror = (msg, src, line) => iio.Panic(`${src}:${line} ${msg}`);
        window.onunhandledrejection = (e) => iio.Panic(e.reason);
        for (let cookie of document.cookie.split("; ")) {
            if (!cookie.startsWith("session="))
                continue;
            iio.User = strings.Cut(cookie.substring("session=".length), ".")[0];
            break;
        }
    },
    User: "",
};
let iioui = {
    // Init sets up the commenting and reaction widgets.
    Init: async function () {
        iio.Init();
        eReactionbox.innerHTML = reactionboxHTML;
        eReactionbox.onclick = (e) => {
            e.stopPropagation();
        };
        document.onclick = () => {
            if (!eReactionbox.hidden)
                eReactionbox.hidden = true;
            if (!eUserinfobox.hidden)
                eUserinfobox.hidden = true;
        };
        document.onkeyup = (event) => {
            if (event.key == "Escape")
                eReactionbox.hidden = true;
            if (event.key == "Escape")
                eUserinfobox.hidden = true;
        };
        if (iio.User != "") {
            let err = await updateUserdata();
            if (err != "")
                return Promise.resolve("iiots.UpdateUserdata: " + err);
        }
        renderAccountpageLink();
        for (let e of document.querySelectorAll(".cReactionLine")) {
            iioui.RenderReactionLine(e);
        }
        for (let e of document.querySelectorAll(".cReply textarea"))
            e.onclick = showCommentButtons;
        for (let e of document.querySelectorAll(".cReply textarea"))
            e.onfocus = showCommentButtons;
        for (let e of document.querySelectorAll(".cComment textarea"))
            e.onclick = showCommentButtons;
        for (let e of document.querySelectorAll(".cComment textarea"))
            e.onfocus = showCommentButtons;
        for (let e of document.querySelectorAll(".cPosterUsername"))
            e.onclick = userClick;
        // Show JS-only features, hide no JS warnings.
        for (let e of document.querySelectorAll(".cNeedsJS"))
            e.classList.toggle("cNeedsJS");
        for (let e of document.querySelectorAll(".cNoJSNote"))
            e.hidden = true;
        return Promise.resolve("");
    },
    // ReactionboxTogglerClick shows/hides the reaction box.
    // It also pre-fills the form with the saved data.
    ReactionboxTogglerClick: function (event) {
        event.stopPropagation();
        if (!eReactionbox.hidden) {
            eReactionbox.hidden = true;
            return;
        }
        eReactionbox.hidden = false;
        let btn = event.target;
        eReactionbox.style.left = `${btn.offsetLeft}px`;
        eReactionbox.style.top = `${btn.offsetTop + btn.offsetHeight}px`;
        let btnParent = btn.parentElement;
        if (reactionboxTarget == btnParent)
            return;
        reactionboxTarget = btnParent;
        let selector = eRBForm.elements["eRBForm"];
        let id = `${reactionboxTarget.dataset.id}-pending`;
        let [pendingReaction, pendingNote] = strings.Cut(userdata[id], " ");
        selector.value = pendingReaction == "" ? "none" : pendingReaction;
        eRBNote.value = pendingNote;
        eRBStatus.hidden = true;
        iioui.RenderReactionbox();
    },
    // RenderReactionLine renders the reaction line for a given comment or reply.
    // It correctly highlights the user's pending (not live, not yet shown to others) reaction+note too.
    RenderReactionLine: function (target) {
        let id = target.dataset.id;
        let h = `<button onclick='iioui.ReactionboxTogglerClick(event)'>☺</button>`;
        let noteh = ``;
        let notes = 0;
        let [liveReaction, liveNote] = strings.Cut(userdata[`${id}-live`], " ");
        let [pendingReaction, pendingNote] = strings.Cut(userdata[`${id}-pending`], " ");
        for (let r in reactionEmojis) {
            let reactid = `${id}-${r}`;
            let cnt = 0;
            if (ReactionCounts[reactid])
                cnt += ReactionCounts[reactid];
            if (r == liveReaction)
                cnt--;
            if (r == pendingReaction)
                cnt++;
            if (cnt == 0)
                continue;
            h += "&nbsp;&nbsp;";
            let cls = "";
            if (r == pendingReaction)
                cls = " class=cbgNeutral";
            h += `<span title=${r}${cls}>&nbsp;${reactionEmojis[r]}${cnt}&nbsp;</span>`;
            let rnotes = [];
            if (ReactionNotes[reactid])
                rnotes = ReactionNotes[reactid];
            for (let note of rnotes) {
                if (r == liveReaction && note == liveNote) {
                    liveNote = "";
                    continue;
                }
                notes++;
                noteh += `<li>${reactionEmojis[r]} ${r}: ${note}\n`;
            }
            if (r == pendingReaction && pendingNote != "") {
                notes++;
                noteh += `<li class=cbgNeutral>${reactionEmojis[r]} ${r}: ${pendingNote}\n`;
            }
        }
        if (notes > 0)
            h += `<details><summary>${notes} notes</summary>\n<ul>${noteh}</ul></details>`;
        target.innerHTML = h;
    },
    // RenderReactionbox updates the submit button based on the user's selection.
    RenderReactionbox: function () {
        if (iio.User == "") {
            eRBUnregistered.hidden = false;
            eRBRegistered.hidden = true;
        }
        else {
            eRBUnregistered.hidden = true;
            eRBRegistered.hidden = false;
            eRBUser.textContent = iio.User;
        }
        let selector = eRBForm.elements["eRBForm"];
        if (selector.value == "") {
            eRBSubmitButton.disabled = true;
            return;
        }
        eRBNote.style.visibility = selector.value == "none" ? "hidden" : "";
        eRBSubmitButton.disabled = false;
    },
    // SubmitReaction submits the currently selected reaction.
    SubmitReaction: async function () {
        if (iio.User == "") {
            eRBStatus.textContent = "Registering as a guest...";
            eRBStatus.hidden = false;
            await iio.RegisterGuest();
        }
        let selector = eRBForm.elements["eRBForm"];
        if (selector.value == "")
            return;
        eRBStatus.textContent = "Sending reaction...";
        eRBStatus.hidden = false;
        let id = `${PostName}.c${reactionboxTarget.dataset.id}`;
        let [result, err] = await iio.Fetch(`/feedbackapi?action=react&id=${id}&reaction=${selector.value}`, {
            method: "POST",
            body: eRBNote.value,
        });
        if (err != "") {
            eRBStatus.textContent = `Error: iiots.Submit: ${err} (try again later?)`;
            return;
        }
        eRBStatus.textContent = "Fetching response...";
        let upderr = await updateUserdata();
        if (upderr != "") {
            eRBStatus.textContent = `Error: iiots.ReupdateUserdata: ${upderr} (try again later?)`;
            return;
        }
        eRBStatus.hidden = true;
        eReactionbox.hidden = true;
        iioui.RenderReactionLine(reactionboxTarget);
        reactionboxTarget = null;
    },
    // PreviewComment converts the markdown reply to HTML and shows it to the user.
    // It also switches back to edit mode if pressed again.
    PreviewComment: async function (id) {
        let editorElem = document.getElementById(`eReplyEditor-${id}`);
        if (editorElem.value.trim() == "")
            return;
        let previewElem = document.getElementById(`eReplyPreview-${id}`);
        let helpElem = document.getElementById(`eReplyHelp-${id}`);
        let b1 = document.getElementById(`eReplyButton1-${id}`);
        let statusElem = document.getElementById(`eReplyStatus-${id}`);
        if (iio.User == "") {
            statusElem.textContent = "Registering as a guest...";
            await iio.RegisterGuest();
        }
        if (editorElem.hidden) {
            editorElem.hidden = false;
            editorElem.focus();
            previewElem.hidden = true;
            helpElem.hidden = false;
            b1.textContent = "Preview";
            statusElem.textContent = "";
            clearTimeout(renderCooldownTimeoutID);
            return;
        }
        let key = id + " " + editorElem.value;
        let result = previewResults.get(key);
        if (!result) {
            statusElem.textContent = "Rendering...";
            let [fetchResult, err] = await iio.Fetch(`/feedbackapi?action=previewcomment&id=${PostName}.c${id}`, {
                method: "POST",
                body: editorElem.value,
            });
            if (err != "") {
                statusElem.textContent = `Error: ${err}.`;
                editorElem.focus();
                return;
            }
            let [status, rest1] = strings.Cut(fetchResult, " ");
            let [cooldownString, rest2] = strings.Cut(rest1, " ");
            let [sig, html] = strings.Cut(rest2, " ");
            result = {
                readyMS: Date.now() + parseInt(cooldownString) + 1,
                status: status,
                sig: sig,
                html: html,
            };
            previewResults.set(key, result);
        }
        let headerid = id;
        if (headerid.endsWith("-0"))
            headerid = headerid.slice(0, -2);
        let header = `<p class=cReplyHeader><em>#c${headerid} by ${iio.User} on ${strings.FormatDate(new Date())}</em></p>\n`;
        statusElem.textContent = "";
        editorElem.hidden = true;
        previewElem.hidden = false;
        helpElem.hidden = true;
        previewElem.innerHTML = header + result.html;
        b1.textContent = "Edit";
        renderCooldown(id);
    },
    // PublishComment persists the comment and clears the UI clutter.
    PublishComment: async function (id) {
        let editorElem = document.getElementById(`eReplyEditor-${id}`);
        let statusElem = document.getElementById(`eReplyStatus-${id}`);
        let buttonsElem = document.getElementById(`eReplyButtons-${id}`);
        let key = id + " " + editorElem.value.trim();
        let result = previewResults.get(key);
        if (!result) {
            iio.Panic("iiots.ResultlessPublish");
            return;
        }
        let [fetchResult, err] = await iio.Fetch(`/feedbackapi?action=comment&id=${PostName}.c${id}&sig=${result.sig}`, {
            method: "POST",
            body: editorElem.value,
        });
        if (err == "409 Conflict: posts.CommentAlreadyExist") {
            let b2 = document.getElementById(`eReplyButton2-${id}`);
            b2.disabled = true;
            statusElem.textContent = "Error: new comment appeared from others, reload first";
            return;
        }
        if (err != "") {
            statusElem.textContent = `Error: ${err}`;
            return;
        }
        if (fetchResult.trim() != "ok") {
            statusElem.textContent = `Error: ${fetchResult}`;
            return;
        }
        editorElem.value = ""; // to keep form data clear even after a reload
        buttonsElem.hidden = true;
    },
    User: "",
};
class previewResult {
    readyMS = 0;
    status = "";
    sig = "";
    html = "";
}
const reactionEmojis = {
    like: "👍",
    informative: "🌱",
    support: "🙏",
    congrats: "🎉",
    dislike: "👎",
    unconvincing: "❓",
    uninteresting: "💤",
    unproductive: "❌",
    unreadable: "🖍️",
    unoriginal: "♻️",
    flag: "🚩",
};
const reactionboxHTML = `<p>What's the strongest emotion you feel about this content?</p>
  <form id=eRBForm oninput=iioui.RenderReactionbox()>
  <label><input type=radio name=eRBForm value=none>none</label><br>
  <label><input type=radio name=eRBForm value=like>👍 like: agree, yes, +1, upvote, general like</label><br>
  <label><input type=radio name=eRBForm value=informative>🌱 informative: educational, insightful, opinion-shifting</label><br>
  <label><input type=radio name=eRBForm value=support>🙏 support: thanks, hugs, pray, love</label><br>
  <label><input type=radio name=eRBForm value=congrats>🎉 congrats: well done, happy for you, party-time</label><br>
  <label><input type=radio name=eRBForm value=dislike>👎 dislike: disagree, no, -1, downvote, general dislike</label><br>
  <label><input type=radio name=eRBForm value=unconvincing>❓ unconvincing: lacks references, bad reasoning</label><br>
  <label><input type=radio name=eRBForm value=uninteresting>💤 uninteresting: boring, too long, spam, offtopic</label><br>
  <label><input type=radio name=eRBForm value=unproductive>❌ unproductive: uncharitable, disrupting, trolling</label><br>
  <label><input type=radio name=eRBForm value=unreadable>🖍️ unreadable: bad grammar, unclear meaning</label><br>
  <label><input type=radio name=eRBForm value=unoriginal>♻️ unoriginal: repost, discussed already</label><br>
  <label><input type=radio name=eRBForm value=flag>🚩 flag: needs moderation, sensitive info, duplicate post</label><br>
  </form>
  <p><input id=eRBNote placeholder="optional max 120 char note" maxlength=120 style="width:calc(100% - 1ch)" onkeyup="if(event.key=='Enter')iioui.SubmitReaction()"></p>
  <button onclick=iioui.SubmitReaction() id=eRBSubmitButton>Submit</button> <em>(anonymous, your username visible only to admins)</em>
  <p id=eRBStatus class=cfgNegative hidden>...</p>
  <p>Current reaction: none<p>
  <p><em>
  <span id=eRBUnregistered>You are not logged in. Log in at <a href=/account>@/account</a>.<br>Otherwise an anonymous guest account will be auto-created.</span>
  <span id=eRBRegistered>You are logged in as <span id=eRBUser></span>.</span>
  <br>
  Note: most reactions will be visible to others only after a day or two.<br>
  See <a href=/feedback>@/feedback</a> for more info.
  </em></p>
`;
let userdata = {};
let reactionboxTarget = null;
let previewResults = new Map();
// updateUserdata fetches and updates iio.Userdata.
async function updateUserdata() {
    let [data, err] = await iio.Fetch(`/feedbackapi?action=userdata&post=${PostName}&ts=${PostRenderTS}`);
    if (err != "")
        return Promise.resolve("iiots.FetchUserdata: " + err);
    userdata = {};
    for (let line of data.split("\n")) {
        if (!line.startsWith("reaction "))
            continue;
        let [id, r] = strings.Cut(line.substring(9), " ");
        userdata[id] = r;
    }
    return Promise.resolve("");
}
// showCommentButtons renders the preview and publish buttons for a reply/comment inputbox.
// It's an onclick implementation.
function showCommentButtons(event) {
    let textbox = event.target;
    let prevelem = textbox.parentElement.previousElementSibling;
    let nextelem = textbox.parentElement.nextElementSibling;
    textbox.onclick = null;
    textbox.onfocus = null;
    textbox.rows = 8;
    let id = textbox.dataset.id;
    textbox.onkeyup = (event) => {
        if (event.ctrlKey && event.key == "Enter")
            iioui.PreviewComment(id);
    };
    let h = "";
    h += `<p id=eReplyButtons-${id}><button id=eReplyButton1-${id} onclick=iioui.PreviewComment("${id}")>Preview</button> <button id=eReplyButton2-${id} onclick=iioui.PublishComment("${id}") disabled>Publish</button> `;
    h += `<span id=eReplyStatus-${id}></span></p>`;
    h += `<div id=eReplyPreview-${id} hidden></div>`;
    prevelem.innerHTML += h;
    nextelem.innerHTML = `<details id=eReplyHelp-${id}><summary>Help:</summary><ul><li>Use #c1-2 to link other comments, <li>- for lists, <li>indent for preformatted text; <li>length limit is 2K bytes; <li>see <a href=/feedback>@/feedback</a> for more info.</ul></details>`;
}
// renderCooldown re-renders the cooldown counter.
let renderCooldownTimeoutID = 0;
function renderCooldown(id) {
    clearTimeout(renderCooldownTimeoutID);
    let b2 = document.getElementById(`eReplyButton2-${id}`);
    let editorElem = document.getElementById(`eReplyEditor-${id}`);
    let statusElem = document.getElementById(`eReplyStatus-${id}`);
    let key = id + " " + editorElem.value.trim();
    let result = previewResults.get(key);
    if (!result) {
        iio.Panic("iiots.ResultlessCooldown");
        return;
    }
    if (result.status == "posts.CommentAlreadyExist") {
        b2.disabled = true;
        statusElem.textContent = "new comment appeared from others, reload first";
        return;
    }
    if (result.status != "ok") {
        b2.disabled = true;
        statusElem.textContent = result.status;
        return;
    }
    let now = Date.now();
    if (now >= result.readyMS) {
        b2.disabled = false;
        statusElem.textContent = "";
        return;
    }
    b2.disabled = true;
    let remtime = result.readyMS - now;
    statusElem.textContent = `cooldown: ~${Math.round(remtime / 1000)}s`;
    renderCooldownTimeoutID = setTimeout(() => renderCooldown(id), Math.min(15000, remtime));
}
// userClick shows/hides the userinfo box.
function userClick(event) {
    event.stopPropagation();
    if (!eUserinfobox.hidden) {
        eUserinfobox.hidden = true;
        return;
    }
    let span = event.target;
    let spanGrandparent = span.parentElement.parentElement;
    let u = span.textContent;
    if (!(u in Userinfos))
        return;
    let [registration, intro] = strings.Cut(Userinfos[u], "\n");
    eUserinfobox.hidden = false;
    eUserinfobox.style.left = `${spanGrandparent.offsetLeft}px`;
    eUserinfobox.style.top = `${span.offsetTop + span.offsetHeight}px`;
    eUserinfobox.style.width = `calc(${spanGrandparent.offsetWidth}px - 2ch)`;
    let h = `Registration date: <em>${registration}</em>`;
    if (intro != "")
        h += `<br><br><em>${intro}</em>`;
    if (u == "legacy-guest")
        h = "This is a comment from the old commenting system, no userinfo available.";
    eUserinfobox.innerHTML = h;
}
// renderAccountpageLink sets the appropriate content to the @/account link at the bottom of the page.
function renderAccountpageLink() {
    if (iio.User == "") {
        eAccountpageLink.innerHTML = "Not logged in. Guest account will be auto-created on interaction. Manage account data at <a href=/account>@/account</a>.";
    }
    else {
        eAccountpageLink.innerHTML = `Logged in as ${iio.User}. Manage account data at <a href=/account>@/account</a>.`;
    }
}
