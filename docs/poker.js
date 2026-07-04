let ss = new CSSStyleSheet()
ss.replace(`
  .red { color: red; }
  table {
    border: 1px solid black;
    cursor: default;
    display: inline-block;
    line-height: normal;
  }
  #hgrid { line-height: 0; }
  pre { display: inline; }
  @media screen {
    #hgrid { display: none; }
  }
  @media print {
    h1, h2, p, ul, hr, textarea, #htable, span:not(.red) { display: none; }
  }
`)
document.adoptedStyleSheets.push(ss)

const handtable = htable.innerHTML.trim();
const gengrid = _ => {
  let h = "";
  for (let i = 0; i < 120; i++) h += handtable;
  hgrid.innerHTML = h;
};
gengrid();
