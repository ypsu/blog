let selectline = evt => {
  // update the hash in a hacky way to make sure :target updates and history remains intact:
  // https://github.com/whatwg/html/issues/639#issuecomment-252716663.
  let t = '#' + evt.currentTarget.id;
  if (location.hash == t) {
    history.replaceState(null, null, '#');
    history.pushState(null, null, '#');
  } else {
    history.replaceState(null, null, t);
    history.pushState(null, null, t);
  }
  history.back();
  showtooltip();
}

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
      html += `<span id=line${linecnt} class=line onclick=selectline(event)>${line}</span><span id=tooltip${linecnt} class=tooltip hidden></span>\n`;
    }
    elem.innerHTML = html;
  }
  showtooltip();
}

main();
