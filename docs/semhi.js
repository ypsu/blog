let lasttp = null
let showtooltip = () => {
  if (lasttp != null) {
    lasttp.hidden = true
    lasttp.innerHTML = ""
    lasttp = null
  }
  if (location.hash != "") {
    let lineid = location.hash.substr(5)
    let tp = document.getElementById(`tooltip${lineid}`)
    tp.hidden = false
    tp.innerHTML = "<blockquote><button>copy</button> <button>comment</button> <button>twitter</button> <button>report</button><blockquote>"
    lasttp = tp
  }
}

let main = () => {
  let ss = new CSSStyleSheet()
  ss.replace(`
    .line:hover { background-color: var(--bg-neutral) }
    :target { background-color: var(--bg-notice) }
  `)
  document.adoptedStyleSheets.push(ss)

  let linecnt = 0
  for (let elem of document.getElementsByTagName("p")) {
    let html = ""
    let startline = 0
    for (let line of elem.innerHTML.split("\n")) {
      linecnt++
      html += `<span id=line${linecnt} class=line>${line}</span><span id=tooltip${linecnt} class=tooltip hidden></span>\n`
    }
    elem.innerHTML = html
    for (let i = startline + 1; i <= linecnt; i++) {
      document.getElementById(`line${i}`).onclick = () => location.replace(`#line${i}`)
    }
  }
  window.onhashchange = showtooltip
  showtooltip()
}

main()
