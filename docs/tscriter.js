const g = {
  foundsolution: false,
  currentsegment: 0,
  // segments[i] is an object: {
  //   id: The sequence number of this segment (used to indicate progress).
  //   start: Start time in seconds of the segment.
  //   sentence: The sentence itself. Can be empty to represent time skips.
  // }
  segments: [],
  // Number of non-empty segments.
  segmentcnt: 0,
  prevtext: "",
  ytplayer: null,
}

const handleerror = (ev) => {
  errormsg.innerText = "Error occurred: "
  if (ev.reason) {
    if (ev.reason.stack) {
      errormsg.innerText += ev.reason.stack
    } else if (ev.reason.details) {
      errormsg.innerText += " " + ev.reason.details
    } else if (ev.reason.result) {
      errormsg.innerText += " " + ev.reason.result.error.message
    } else {
      errormsg.innerText += ev.reason
    }
  } else {
    errormsg.innerText += ev
  }
  errormsg.hidden = false
}

const fmt2d = (v) => {
  if (v < 10) return ` ${v}`
  return `${v}`
}

const escapehtml = (unsafe) => {
  return unsafe.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#039;")
}

const handlekey = (e) => {
  if (e.keyCode == 13) {
    if (g.foundsolution) {
      nextsegment(+1)
    } else {
      play()
    }
  } else {
    showfeedback()
  }
}

const ytre = /(?:youtube\.com\/\S*(?:(?:\/e(?:mbed))?\/|watch\?(?:\S*?&?v\=))|youtu\.be\/)([a-zA-Z0-9_-]{6,11})/
let ytlookup = async () => {
  if (hurlbox.value === "") {
    hytresults.innerText = ""
    return
  }
  const m = hurlbox.value.match(ytre)
  if (!m) {
    hytresults.innerText = "Not a valid youtube url."
    return
  }
  const vid = m[1]
  const url = `https://video.google.com/timedtext?type=list&v=${vid}`
  const resp = await fetch(url)
  if (resp.status != 200) {
    hytresults.innerText = `Error ${resp.status} while loading ${url}.`
    return
  }
  const body = await resp.text()
  const dom = new DOMParser().parseFromString(body, "text/xml")
  let r = ""
  let prefix = window.location.href.replace(/\/[^\/]*$/, "")
  for (let track of dom.getElementsByTagName("track")) {
    let u = `${prefix}/tscriter#${vid}`
    u += ":" + track.attributes["name"].value
    u += ":" + track.attributes["lang_code"].value
    r += `<a href='${u}'>${u}</a>`
    if (track.attributes["lang_default"]) {
      r += " (default)"
    }
    r += "<br>"
  }
  if (r == "") r = "No captions."
  hytresults.innerHTML = r
}

const digitordash = /[0-9-]/
const isalphanum = (c) => {
  return c.toLowerCase() != c.toUpperCase() || digitordash.test(c)
}

// Splits a string into list of string parts. The parts alternate between
// the non-significant punctuation and the word parts. Example:
// "hi" => ["", "hi", ""]
// "Hello, world!" => ["", "Hello", ", ", "world", "!"]
const getruns = (s) => {
  let r = []
  let wordmode = false
  let run = ""
  for (let i = 0; i < s.length; i++) {
    if (isalphanum(s[i])) {
      if (wordmode) {
        run += s[i]
      } else {
        r.push(run)
        run = s[i]
        wordmode = true
      }
    } else {
      if (wordmode) {
        r.push(run)
        run = s[i]
        wordmode = false
      } else {
        run += s[i]
      }
    }
  }
  r.push(run)
  if (wordmode) {
    r.push("")
  }
  return r
}

const digraphs = {
  ä: "ae",
  ö: "oe",
  ü: "ue",
  ß: "b",
}

// Algorithm for the feedback rendering:
// - Break down both the expected and entered strings into a list of words.
// - Compare the lists word by word and render the feedback parts.
// - Join the per word feedbacks with the original punctuation.
const showfeedback = () => {
  // Don't run this logic if the input didn't actually change.
  if (g.prevtext == usertext.value) return
  g.prevtext = usertext.value
  g.foundsolution = false
  if (g.currentsegment == g.segments.length) {
    feedbackspan.innerText = ""
    progressbar.style.width = "100%"
    progressmsg.innerText = "Dictation over."
    return
  }
  const id = g.segments[g.currentsegment].id - 1
  progressmsg.innerText = `${id} / ${g.segmentscnt}`
  progressbar.style.width = `${(id / g.segmentscnt) * 100.0}%`
  g.foundsolution = true
  const expected = getruns(g.segments[g.currentsegment].sentence)
  const entered = getruns(usertext.value.normalize())
  if (expected.length < 3) {
    feedbackspan.innerText = "error occured"
    return
  }
  // If the user entered more words than needed, fold the extra words into the
  // last word. This way we don't have to handle them specially.
  while (entered.length > expected.length) {
    entered.pop()
    const w = entered.pop()
    entered[entered.length - 2] += " " + w
  }
  let hintsleft = 3
  const addmissing = (letters, cls) => {
    let r = ""
    while (hintsleft > 0 && letters.length > 0) {
      hintsleft--
      const letter = letters.substr(0, 1)
      letters = letters.substr(1)
      r += `<span class="hiddenhint ${cls}">_</span>`
      r += "<span class=hint hidden>" + escapehtml(letter) + "</span>"
    }
    if (letters.length > 0) {
      r += letters.replace(/./g, `<span class=${cls}>_</span>`)
    }
    return r
  }
  let feedback = ""
  let i
  for (i = 1; i < entered.length; i += 2) {
    feedback += expected[i - 1]
    let expword = expected[i]
    let entword = entered[i]
    while (expword.length > 0 && entword.length > 0) {
      if (expword[0] == entword[0]) {
        feedback += "<span class=good>" + escapehtml(expword[0]) + "</span>"
        expword = expword.substr(1)
        entword = entword.substr(1)
        continue
      }
      if (expword[0].toLowerCase() == entword[0].toLowerCase()) {
        feedback += "<span class=ok>" + escapehtml(expword[0]) + "</span>"
        expword = expword.substr(1)
        entword = entword.substr(1)
        continue
      }
      const dg = digraphs[expword[0]] && digraphs[expword[0]].toLowerCase()
      if (dg && entword.substr(0, dg.length).toLowerCase() == dg) {
        feedback += "<span class=good>" + escapehtml(expword[0]) + "</span>"
        expword = expword.substr(1)
        entword = entword.substr(dg.length)
        continue
      }
      g.foundsolution = false
      const idx = expword.indexOf(entword[0])
      if (idx != -1) {
        feedback += addmissing(expword.substr(0, idx), "bad")
        expword = expword.substr(idx)
        continue
      }
      feedback += "<span class=bad>" + escapehtml(entword[0]) + "</span>"
      entword = entword.substr(1)
    }
    if (entword.length > 0) {
      g.foundsolution = false
      feedback += "<span class=bad>" + escapehtml(entword) + "</span>"
    }
    if (expword.length > 0) {
      g.foundsolution = false
      if (i + 2 >= entered.length) {
        feedback += addmissing(expword, "missing")
      } else {
        feedback += addmissing(expword, "bad")
      }
    }
  }
  for (; i < expected.length; i += 2) {
    g.foundsolution = false
    feedback += expected[i - 1]
    feedback += addmissing(expected[i], "missing")
  }
  feedback += expected[i - 1]
  feedbackspan.innerHTML = feedback
}

const showhint = () => {
  for (let elem of document.getElementsByClassName("hiddenhint")) {
    elem.hidden = true
  }
  for (let elem of document.getElementsByClassName("hint")) {
    elem.hidden = false
  }
  usertext.focus()
}

const nextsegment = (direction) => {
  usertext.value = ""
  g.currentsegment += direction
  while (0 <= g.currentsegment && g.currentsegment < g.segments.length && g.segments[g.currentsegment].sentence == "") {
    g.currentsegment += direction
  }
  if (g.currentsegment < 0) g.currentsegment = 0
  if (g.currentsegment > g.segments.length) {
    g.currentsegment = g.segments.length
  }
  g.prevtext = "---"
  showfeedback()
  play()
  usertext.focus()
}

let timeoutid = null
const playaudio = async () => {
  if (g.currentsegment >= g.segments.length) return
  clearTimeout(timeoutid)
  const start = g.segments[g.currentsegment].start
  audioelem.currentTime = start
  try {
    await audioelem.play()
  } catch (err) {
    errormsg.innerText = "Couldn't start audio. "
    errormsg.innerText += "Please try a different or newer browser."
    errormsg.hidden = false
  }
  if (g.currentsegment + 1 < g.segments.length) {
    const end = g.segments[g.currentsegment + 1].start
    timeoutid = setTimeout(
      () => {
        audioelem.pause()
      },
      ((end - start) * 1000) / (parseFloat(speedselector.value) / 100),
    )
  }
}

let playytcnt = 0
const playyt = async () => {
  if (g.currentsegment >= g.segments.length) return
  playytcnt++
  let thiscnt = playytcnt
  clearTimeout(timeoutid)
  const start = g.segments[g.currentsegment].start
  g.ytplayer.seekTo(start, true)
  g.ytplayer.playVideo()
  while (g.ytplayer.getPlayerState() != 1) {
    await eventpromise(g.ytplayer, "onStateChange")
  }
  if (thiscnt != playytcnt) {
    // While we were waiting, the user moved to a different segment, don't
    // bother trying to start the audio.
    return
  }
  if (g.currentsegment + 1 < g.segments.length) {
    const end = g.segments[g.currentsegment + 1].start
    timeoutid = setTimeout(
      () => {
        g.ytplayer.pauseVideo()
      },
      ((end - start) * 1000) / (parseFloat(speedselector.value) / 100),
    )
  }
}

const changeaudiospeed = () => {
  let fmt = speedselector.value
  if (fmt.length < 3) fmt = "&nbsp" + fmt
  speedspan.innerHTML = fmt
  audioelem.playbackRate = parseFloat(speedselector.value) / 100.0
  // Recalculate the pause point.
  if (g.currentsegment + 1 >= g.segments.length) return
  clearTimeout(timeoutid)
  const end = g.segments[g.currentsegment + 1].start
  const cur = audioelem.currentTime
  timeoutid = setTimeout(
    () => {
      audioelem.pause()
    },
    ((end - cur) * 1000) / (parseFloat(speedselector.value) / 100),
  )
}

const changeytspeed = () => {
  let fmt = speedselector.value
  if (fmt.length < 3) fmt = "&nbsp" + fmt
  speedspan.innerHTML = fmt
  g.ytplayer.setPlaybackRate(parseFloat(speedselector.value) / 100.0)
  // Recalculate the pause point.
  if (g.currentsegment + 1 >= g.segments.length) return
  clearTimeout(timeoutid)
  const end = g.segments[g.currentsegment + 1].start
  const cur = g.ytplayer.getCurrentTime()
  timeoutid = setTimeout(
    () => {
      g.ytplayer.pauseVideo()
    },
    ((end - cur) * 1000) / (parseFloat(speedselector.value) / 100),
  )
}

let play = null
let changespeed = null

// Convert continuation passing style into direct style:
// await eventpromise(thing, 'click') will wait for a single click on thing.
const eventpromise = (thing, type) => {
  return new Promise((resolve, _) => {
    var handler = function (event) {
      resolve(event)
      thing.removeEventListener(type, handler)
    }
    thing.addEventListener(type, handler)
  })
}

const main = async (evt, pastetext) => {
  errormsg.hidden = true
  hmaindiv.hidden = true
  htooldiv.hidden = true
  loadingmsg.hidden = false
  ytplayerdiv.hidden = true
  const cfgurl = location.hash.substr(1)
  if (cfgurl == "" && !pastetext) {
    loadingmsg.hidden = true
    hmaindiv.hidden = false
    return
  }
  htooldiv.hidden = false
  let segmentscnt = 0
  g.segments = []
  g.currentsegment = 0
  const re = new RegExp("^((\\w|-){11}):((\\w|-)*):((\\w|-)*)$")
  const ytparams = !pastetext && cfgurl.match(re)
  if (ytparams) {
    // This is a direct reference to a youtube video along with a language code.
    // ts means transcript.
    const vid = ytparams[1]
    const langcode = ytparams[5]
    const langname = ytparams[3]
    let tsurl = `https://video.google.com/timedtext?v=${vid}`
    tsurl += `&lang=${langcode}&name=${langname}`
    const tsresp = await fetch(tsurl)
    if (tsresp.status != 200) {
      errormsg.innerText = `Error ${tsresp.status} while loading ${tsurl}.`
      errormsg.hidden = false
      return
    }
    const tsbody = await tsresp.text()
    const parser = new DOMParser()
    const ts = new DOMParser().parseFromString(tsbody, "text/xml")
    for (let t of ts.getElementsByTagName("text")) {
      const start = parseFloat(t.attributes["start"].value)
      const dur = parseFloat(t.attributes["dur"].value)
      const dom = parser.parseFromString(t.textContent, "text/html")
      let text = dom.body.textContent.replace("\n", " ")
      text = text.replace(/-/g, " ")
      text = text.replace(/\[[^]]*\]/g, " ")
      text = text.replace(/\([^)]*\)/g, " ")
      if (!text.match(/\w/)) continue
      segmentscnt++
      const segment = {
        id: segmentscnt,
        start: start,
        sentence: text,
      }
      g.segments.push(segment)
      const emptysegment = {
        id: segmentscnt,
        start: start + dur,
        sentence: "",
      }
      g.segments.push(emptysegment)
    }
    await setupytplayer(vid)
  } else {
    // From now on comes the custom transcript parsing.
    let cfgbody = pastetext || ""
    if (cfgbody == "") {
      const cfgresp = await fetch(cfgurl)
      if (cfgresp.status != 200) {
        errormsg.innerText = `Error ${cfgresp.status} while loading ${cfgurl}.`
        errormsg.hidden = false
        return
      }
      cfgbody = await cfgresp.text()
    }
    const cfglines = cfgbody.normalize().trim().split("\n")
    const url = cfglines[0]
    if (url.match(/^(\w|-){11}$/)) {
      // url is a youtube video id.
      await setupytplayer(url)
    } else {
      // url is link to an audio file.
      audioelem.src = url
      audioelem.load()
      play = playaudio
      changespeed = changeaudiospeed
      await eventpromise(audioelem, "canplay")
    }
    for (let i = 1; i < cfglines.length; i++) {
      const [_, start, sentence] = cfglines[i].match(" *([0-9.]*) *(.*)")
      if (sentence != "") segmentscnt++
      const segment = {
        id: segmentscnt,
        start: parseFloat(start),
        sentence: sentence,
      }
      g.segments.push(segment)
    }
  }
  g.segmentscnt = segmentscnt
  changespeed()
  g.prevtext = "---"
  showfeedback()
  loadingmsg.hidden = true
  usertext.focus()
}

let markytready = null
const setupytplayer = async (vid) => {
  if (g.ytplayer == null) {
    let ytwaiter = new Promise((resolve, _) => {
      markytready = resolve
    })
    const tag = document.createElement("script")
    tag.src = "https://www.youtube.com/iframe_api"
    const first = document.getElementsByTagName("script")[0]
    first.parentNode.insertBefore(tag, first)
    await ytwaiter
  }
  // Need to recreate the div to replace otherwise yt doesn't replace it (only
  // matters after a hashchange).
  let div = document.createElement("div")
  div.id = "ytplayerdiv"
  ytplayerdiv.parentNode.replaceChild(div, ytplayerdiv)
  g.ytplayer = new YT.Player("ytplayerdiv", {
    videoId: vid,
    playerVars: {
      cc_load_policy: 3,
      controls: 0,
      rel: 0,
    },
  })
  await eventpromise(g.ytplayer, "onReady")
  ytplayerdiv.style.maxWidth = "99%"
  play = playyt
  changespeed = changeytspeed
}

function onYouTubeIframeAPIReady() {
  markytready()
}

document.onkeydown = (e) => {
  if ((e.altKey || e.ctrlKey || e.metaKey) && e.which == 72) {
    // 'h'
    showhint()
    return false
  } else if ((e.altKey || e.ctrlKey || e.metaKey) && e.keyCode == 13) {
    // Enter
    nextsegment(+1)
    return false
  } else if ((e.altKey || e.ctrlKey || e.metaKey) && e.keyCode == 66) {
    // 'b'
    nextsegment(-1)
    return false
  } else if (e.which == 38) {
    // up arrow
    if (parseInt(speedselector.value) < 150) {
      speedselector.value = parseInt(speedselector.value) + 25
      changespeed()
    }
    return false
  } else if (e.which == 40) {
    // down arrow
    if (parseInt(speedselector.value) > 50) {
      speedselector.value = parseInt(speedselector.value) - 25
      changespeed()
    }
    return false
  }
}

window.onload = main
window.onhashchange = main
audioelem.onerror = handleerror
window.onerror = handleerror
window.onunhandledrejection = handleerror

usertext.onkeyup = handlekey
ehintButton.onclick = showhint
enextButton.onclick = () => nextsegment(+1)
eprevButton.onclick = () => nextsegment(-1)
speedselector.onchange = () => { changespeed(); usertext.focus() }

// usertext has by default a password type. Apparently this disables text
// prediction phones. That would be quite unhelpful during a dictation practice.
usertext.type = "text"
window.addEventListener("paste", (e) => {
  if (e.target == usertext || e.target == hurlbox) return
  main(e, e.clipboardData.getData("text").trim())
})
