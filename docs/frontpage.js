function filter() {
  let elems = document.body.childNodes
  let tag = ""
  let phidx = 3  // placeholder index
  if (location.hash.length >= 1) tag = location.hash.substring(1)

  let txt = ""
  txt += "Filter with "
  txt += "<a href=#projects>@#projects</a>, "
  txt += "<a href=#fav>@#fav</a>, or "
  txt += "<a href=#demo>@#demo</a> for the interesting posts."
  if (tag == "") {
    document.querySelector(`li:nth-child(${phidx})`).innerHTML = txt
    for (let e of elems) e.hidden = false
    hSelection.hidden = true
    hFilterMessage.hidden = true
    return
  }
  txt += " use <a href=#>@#</a> to show all."

  let ul = ""
  let tagged = []
  if (tag in tags) tagged = tags[tag]
  for (let li of document.querySelectorAll("li")) {
    if (li.parentElement.id == "hSelection") continue
    if (li.childNodes.length <= 1) continue
    if (!tagged.includes(li.childNodes[1].innerText.substring(2))) continue
    ul += `<li>${li.innerHTML}</li>\n`
  }

  let filterMessage
  filterMessage = "filtered entries:"
  if (tag == "fav") filterMessage = "My favorite posts:"
  if (tag == "demo") filterMessage = "Interactive posts:"
  if (tag == "projects") filterMessage = "My bigger entries that might be useful for others too:"

  document.querySelector(`li:nth-child(${phidx})`).innerHTML = txt
  for (let i = 4; i < elems.length; i++) elems[i].hidden = true
  hSelection.hidden = false
  hFilterMessage.innerText = filterMessage
  hFilterMessage.hidden = false
  hSelection.innerHTML = ul
}

function main() {
  window.onhashchange = filter
  filter()
}

main()
