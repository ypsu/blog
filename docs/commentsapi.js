let utf8encoder = new TextEncoder('utf-8')
let signatures = new Map()
let savedmsg = ''
let triggertime = 0

function tohex(arr) {
  return (new Uint8Array(arr)).reduce((a, b) => a + b.toString(16).padStart(2, '0'), '')
}

function escapeHTML(unsafe) {
  return unsafe.replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
}

function markdown(str) {
  let h = ''
  for (let block of str.split('\n\n')) {
    block = escapeHTML(block)
    if (block.length == 0) continue
    if (block[0] == ' ') {
      h += `<pre>${block}</pre>\n`
    } else if (block[0] == '#') {
      h += `<p style=font-weight:bold>${block}</p>\n`
    } else if (block.startsWith('&gt;')) {
      block = block.substr(4).replaceAll('\n&gt;', '\n').replaceAll('\n\n', '</p><p>')
      h += `<blockquote><p>${block}</p></blockquote>\n`
    } else if (block[0] == '-') {
      h += '<ul>'
      block = block.substr(1)
      for (let subblock of block.split('\n-')) {
        h += `<li>${subblock}`
      }
      h += '</ul>'
    } else {
      h += `<p>${block}</p>\n`
    }
  }
  return h
}

async function commentpost() {
  let msg = hcommenttext.value
  if (!signatures.has(msg)) {
    hcommentnote.innerText = "internal error; preview button didn't work? see js console"
    return
  }
  hcommentnote.innerText = 'contacting the server...'
  let request = {
    msg: msg,
    post: commentPost,
    id: commentID,
    signature: signatures.get(msg).signature,
  }
  let response
  try {
    response = await fetch('/commentsapi', {
      method: 'POST',
      body: new URLSearchParams(request).toString(),
      headers: {
        'content-type': 'application/x-www-form-urlencoded',
      },
    })
  } catch (e) {
    hcommentnote.innerText = 'error: ' + e
    return
  }
  if (!response.ok) {
    hcommentnote.innerText = 'error: ' + response.statusText
    hcommentnote.innerText = 'error: ' + (await response.text())
    return
  }
  hcommenttext.value = ''
  try {
    localStorage.removeItem(`comments.${commentPost}`)
  } catch (e) {
    // ignore the lack of localstorage.
  }
  window.location.reload()
}

async function commentpreview() {
  hpostbutton.disabled = true
  hpreview.innerHTML = markdown(hcommenttext.value)
  let msg = hcommenttext.value
  savedmsg = msg
  if (msg == '') {
    try {
      localStorage.removeItem(`comments.${commentPost}`)
    } catch (e) {
      // ignore the lack of localstorage.
    }
  }
  if (signatures.has(msg)) {
    savedmsg = msg
    triggertime = signatures.get(msg).t
    updatecommentsbuttons()
    return
  }
  if (msg == '') return
  let encoded = utf8encoder.encode(msg)
  if (encoded.length > 2000) {
    hcommentnote.innerText = `error: text too long: ${encoded.length} bytes, max: 2000 bytes`
    savedmsg = ''
    return
  }
  hcommentnote.innerText = 'contacting the server...'
  let msghash = tohex(await crypto.subtle.digest('SHA-256', encoded))
  let response
  try {
    response = await fetch('/commentsapi', {
      method: 'POST',
      body: `sign=${msghash}&post=${commentPost}&id=${commentID}`,
      headers: {
        'content-type': 'application/x-www-form-urlencoded',
      },
    })
  } catch (e) {
    hcommentnote.innerText = 'error: ' + e
    return
  }
  if (!response.ok) {
    hcommentnote.innerText = 'error: ' + response.statusText
    hcommentnote.innerText = 'error: ' + (await response.text())
    return
  }
  hcommentnote.innerText = 'signing comment...'
  let signature
  try {
    signature = await response.text()
  } catch (e) {
    hcommentnote.innerText = 'error: ' + e
    return
  }
  if (signature.length <= 65 + 12) {
    hcommentnote.innerText = 'error: invalid signature length'
    return
  }
  let t = parseInt(signature.slice(65))
  if (t != t) {
    hcommentnote.innerText = 'error: invalid trigger time'
    return
  }
  signatures.set(msg, {
    t: t,
    signature: signature
  })
  triggertime = t
  hcommentnote.innerText = ''
  updatecommentsbuttons()

  try {
    // save to localstorage so user can close the tab while they wait for the cooldown.
    let md = {
      message: msg,
      comments: commentID,
      t: t,
      signature: signature,
    }
    localStorage.setItem(`comments.${commentPost}`, JSON.stringify(md))
  } catch (e) {
    // ignore the lack of localstorage.
  }
}

let updatecommentstimeout = null

function updatecommentsbuttons() {
  if (updatecommentstimeout) {
    clearTimeout(updatecommentstimeout)
    updatecommentstimeout = null
  }
  hcommentnote.innerText = ''
  hpreviewbutton.disabled = false
  hpostbutton.disabled = true
  if (savedmsg == '' || hcommenttext.value != savedmsg) return
  let now = Date.now()
  if (now >= triggertime) {
    hpreviewbutton.disabled = true
    hpostbutton.disabled = false
    return
  }
  let d = new Date(triggertime + 60000)
  let hh = String(d.getHours()).padStart(2, 0)
  let mm = String(d.getMinutes()).padStart(2, 0)
  hcommentnote.innerText = `cooldown, posting unlocks after ${hh}:${mm}`
  updatecommentstimeout = setTimeout(updatecommentsbuttons, triggertime - now + 1000)
}

function commentkeyup(e) {
  if (e.key == 'Enter' && (e.altKey || e.ctrlKey || e.metaKey)) {
    commentpreview()
  } else {
    updatecommentsbuttons()
  }
}

function commentsmain() {
  if (!crypto.subtle) {
    hjs4comments.innerText = 'posting comments require the crypto.subtle js api. are you missing https?'
    return
  }

  hjs4comments.hidden = true
  hnewcommentsection.hidden = false
  hpostbutton.disabled = true
  hpostbutton.onclick = commentpost
  hpreviewbutton.onclick = commentpreview
  hcommenttext.onkeyup = commentkeyup
  document.onvisibilitychange = updatecommentsbuttons
  if (hcommenttext.value == '') {
    try {
      let c = JSON.parse(localStorage.getItem(`comments.${commentPost}`))
      if (c) {
        if (commentID == c.comments) {
          signatures.set(c.message, {
            t: c.t,
            signature: c.signature,
          })
        }
        hcommenttext.value = c.message
      }
    } catch (e) {
      // ignore the lack of localstorage.
    }
  }
  commentpreview()
}

commentsmain()
