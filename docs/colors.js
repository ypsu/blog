let darkPreference = matchMedia("(prefers-color-scheme:dark)")

function setTheme() {
  if (htdark.checked || (htsystem.checked && darkPreference.matches)) {
    document.documentElement.style.colorScheme = "dark"
    document.documentElement.setAttribute("data-theme", "dark")
  } else {
    document.documentElement.style.colorScheme = "light"
    document.documentElement.setAttribute("data-theme", "light")
  }
}

function main() {
  darkPreference.addEventListener("change", setTheme)
  setTheme()

  // highlight the color classes.
  let classes = [
    'Normal',
    'Neutral',
    'Notice',
    'Negative',
    'Positive',
    'Reference',
    'Special',
    'Inverted',
  ]
  let elems = hcolorclasses.children[0].children
  for (let i = 1; i < 8; i++) {
    let c = classes[i]
    let e = elems[i]
    let t = e.innerText
    let [a, b] = t.split(':')
    e.onclick = _ => e.innerText = t
    e.innerHTML = `<span class=cbgDemo${c}>${a}</span>:<span class=cfgDemo${c}>${b}</span>`
  }

  // generate the combinations table.
  let h = ''
  h += '<tr><th>bg\\fg'
  for (let c = 0; c < 8; c++) {
    h += `<th>${classes[c].toLowerCase()}`
  }
  for (let r = 0; r < 8; r++) {
    h += `<tr><th>${classes[r].toLowerCase()}`
    let bg = ''
    if (r != 0) bg = 'cbgDemo' + classes[r]
    for (let c = 0; c < 8; c++) {
      let fg = ''
      if (c != 0) fg = 'cfgDemo' + classes[c]
      h += `<td class='${fg} ${bg}'>sample text`
    }
  }
  hCombinations.innerHTML = h

  // hide the js warning.
  hJSWarning.hidden = true
}

main()
