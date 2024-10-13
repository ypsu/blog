let lasttp = null;
let showtooltip = () => {
  if (lasttp != null) {
    lasttp.hidden = true;
    lasttp.innerHTML = '';
    lasttp = null;
  }
  if (location.hash != '') {
    let lineid = location.hash.substr(5);
    let tp = document.getElementById(`tooltip${lineid}`);
    tp.hidden = false;
    tp.innerHTML = '<blockquote><button>copy</button> <button>comment</button> <button>twitter</button> <button>report</button><blockquote>';
    lasttp = tp;
  }
}

let main = () => {
  let linecnt = 0;
  for (let elem of document.getElementsByTagName('p')) {
    let html = '';
    for (let line of elem.innerHTML.split('\n')) {
      linecnt++;
      html += `<span id=line${linecnt} class=line onclick="location.replace('#line${linecnt}')">${line}</span><span id=tooltip${linecnt} class=tooltip hidden></span>\n`;
    }
    elem.innerHTML = html;
  }
  window.onhashchange = showtooltip
  showtooltip();
}

main();
