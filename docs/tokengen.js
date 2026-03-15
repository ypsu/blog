function gengrid() {
  let cnt = parseInt(hcount.value)
  let cell = `<span class=ctoken>${htext.value}</span>`
  let h = ""
  for (let i = 0; i < cnt; i++) h += cell
  hgrid.innerHTML = h
  hgrid.style = `font-size:${hscale.value}%;`
}
onbeforeprint = gengrid
ePrintButton.onclick = () => window.print()
