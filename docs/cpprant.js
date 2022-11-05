window.onerror = _ => {
  hinterface.hidden = true
  hloading.hidden = false
  hloading.innerHTML = 'javascript error occured, see console.'
}

let faces = {
  cat: '🐱',
  dog: '🐶',
}

// states is a list of the following type.
// {
//    t: integer, the time offset in ms from which point this state applies.
//    speaker: html string, the top dialog box.
//    history: html string, the rest of the conversation.
//    editor: html string, the contents of the editor.
// }
// the list is ordered by time so it can be binary searched.
let states = []

function formatelem(actor, text) {
  if (text == '') return ''
  let htext = `<div class=${actor}>${text}</div>`
  let hface = `<div class=face>${faces[actor]}</div>`
  if (actor == 'dog') {
    return hface + htext + '<br>\n'
  } else {
    return htext + hface + '<br>\n'
  }
}

let laststate

function update() {
  let t = haudio.currentTime * 1000
  let [lo, hi] = [0, states.length - 1]
  while (lo <= hi) {
    let mid = Math.floor((lo + hi) / 2)
    if (states[mid].t > t) {
      hi = mid - 1
    } else {
      lo = mid + 1
    }
  }
  let s = states[hi]
  if (s.speaker != laststate.speaker) hspeaker.innerHTML = s.speaker
  if (s.history != laststate.history) hdialog.innerHTML = s.history
  if (s.editor != laststate.editor) heditor.innerHTML = s.editor
  laststate = s
  setTimeout(update, 50)
}

function debugselection() {
  let r = getSelection()
  hdebug.innerText = `start:${r.anchorOffset} end:${r.focusOffset}`
}

function escapehtml(unsafe) {
  return unsafe.replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function main() {
  // pre-build the ui state for each timestamp.
  let offset = 0
  let lastActor = ''
  let speaker = ''
  let history = ''
  let editor = ''
  for (let line of rawdata.split('\n')) {
    line = line.trim()
    if (line == '' || line[0] == '#') continue
    let [duration, directive, ...args] = line.split(' ')
    let [actor, word] = ['', '']
    if (directive == 'cat' || directive == 'dog') {
      [actor, word] = [directive, args[0]]
    }
    if (directive == 'add') {
      editor = editor.slice(0, parseInt(args[0])) + decodeURIComponent(args[1]) + editor.slice(parseInt(args[0]))
    }
    if (directive == 'del') {
      editor = editor.slice(0, parseInt(args[0])) + editor.slice(parseInt(args[0]) + 1)
    }
    if (actor != lastActor) {
      if (speaker != '') {
        history = formatelem(lastActor, speaker) + history
      }
      speaker = ''
      lastActor = actor
    }
    if (word) speaker += word + ' '
    states.push({
      t: offset,
      speaker: formatelem(actor, speaker),
      history: history,
      editor: escapehtml(editor),
    })
    offset += parseInt(duration)
  }
  states[states.length - 1].speaker = `${formatelem('cat', '[cat disconnected]')}<br><br>\n`
  laststate = states[0]

  update()
  haudio.ontimeupdate = update
  hloading.hidden = true
  hinterface.hidden = false
}

main()
