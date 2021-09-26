let selectline = evt => {
  let t = '#' + evt.currentTarget.id;
  if (location.hash == t) {
    location.hash = '';
    history.replaceState(null, null, ' ');
  } else {
    location.hash = t;
  }
  showtooltip();
}

let lasttp = null;
let showtooltip = () => {
  if (lasttp != null) {
    lasttp.hidden = true;
    lasttp.innerHTML = '';
    lasttp = null;
  }
  if (location.hash == '') {
    st.innerHTML = '.line:hover { background-color: yellow }';
  } else {
    st.innerHTML = ':target { background-color: yellow }';
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
