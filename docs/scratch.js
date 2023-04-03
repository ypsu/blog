let gridsize = 50
let radius = 2
let lightness = 0.8

// entries contains object with the following 5 fields:
// - hcanvas: the canvas element.
// - hrem:    the output element for the remaining pixel count.
// - hsecret: the element that will be revealed on completion.
// - rem:     the number of covered pixels remaining.
// - text:    the text that is displayed next to the canvas.
// - secret:  the secret text that will be revealed after completion.
// - img:     the data bytes to render.
let entries = []

let textdata = `
it all starts with a black canvas.
i was chatting with a coworker when the topic of "keeping kids busy" came up.

i start with scratching the corners.
the coworker suggested these fancy scratchpad notebooks for them.

then i scratch the edges.
of course my kid had no interest in it.

then i scratch nice rows to make the bulk work simple.
so i gave it a try myself.

it's just scratching, scratching, scratching until everything is gone.
now i can't stop scratching.

then i start scratching from the beginning again.
i ... can't ... stop ... ... send help? ... or more scratchbooks.
`

function clamp(v) {
  if (v < radius) return radius
  if (v > gridsize - radius - 1) return gridsize - radius - 1
  return Math.floor(v)
}

function draw(eid, x, y) {
  let entry = entries[eid]
  if (entry.rem == 0) return
  x = clamp(x)
  y = clamp(y)
  let ctx = entry.hcanvas.getContext('2d')
  let data = entry.img.data
  for (let j = y - radius; j <= y + radius; j++) {
    for (let i = x - radius; i <= x + radius; i++) {
      let k = (j * gridsize + i) * 4
      if (data[k + 3] != 0) continue
      entry.rem--
      data[k + 3] = 255
    }
  }
  let sz = 2 * radius + 1
  ctx.putImageData(entry.img, 0, 0)
  entry.hrem.innerText = `${entry.rem}`
  if (entry.rem == 0) {
    entry.hrem.style.opacity = 0
    entry.hsecret.style.opacity = 1
    entry.hcanvas.style.borderColor = 'green'
  }
}

function mousemove(eid, evt) {
  if ((evt.buttons & 1) != 1) return
  let hcanvas = entries[eid].hcanvas
  let x = evt.offsetX * hcanvas.width / hcanvas.clientWidth
  let y = evt.offsetY * hcanvas.height / hcanvas.clientHeight
  draw(eid, x, y)
}

function touchmove(eid, evt) {
  evt.preventDefault()
  let hcanvas = entries[eid].hcanvas
  let r = hcanvas.getBoundingClientRect()
  let x = (evt.touches[0].clientX - r.x) * gridsize / hcanvas.clientWidth
  let y = (evt.touches[0].clientY - r.y) * gridsize / hcanvas.clientHeight
  draw(eid, x, y)
}

function escapehtml(unsafe) {
  return unsafe.replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// input: h as an angle in [0,360] and s,l in [0,1] - output: r,g,b in [0,255]
function hsl2rgb(h, s, l) {
  let a = s * Math.min(l, 1 - l);
  let f = (n, k = (n + h / 30) % 12) => l - a * Math.max(Math.min(k - 3, 9 - k, 1), -1);
  return [Math.floor(f(0) * 255), Math.floor(f(8) * 255), Math.floor(f(4) * 255)];
}

function main() {
  let entry = {}
  for (let line of textdata.split('\n')) {
    if (line == '') continue
    if (entry.text == undefined) {
      entry.text = line
    } else {
      entry.secret = line
      entries.push(entry)
      entry = {}
    }
  }

  let h = ''
  h += `
    <style>
      canvas {
        background-color: black;
        border: 0.5em solid brown;
        image-rendering: pixelated;
        width: 90%;
      }
      table {
        width: 100%;
      }
      td {
        width: 60%;
      }
    </style>`
  h += `<table>`
  for (let i = 0; i < entries.length; i++) {
    h += `<tr>`
    h += `<td><canvas width=${gridsize} height=${gridsize} ontouchmove=touchmove(${i},event) onmousemove=mousemove(${i},event)></canvas></td>`
    h += `<td>${escapehtml(entries[i].text)}<br><br>`
    h += `<em style=display:grid><div class=crem style=grid-column:1;grid-row:1></div><div class=csecret style=grid-column:1;grid-row:1;opacity:0>${escapehtml(entries[i].secret)}</div></em></td>`
    h += `</tr>`
  }
  h += `</table>`
  hcontent.innerHTML = h

  let canvases = document.getElementsByTagName('canvas')
  let rems = document.getElementsByClassName('crem')
  let secrets = document.getElementsByClassName('csecret')
  for (let i = 0; i < entries.length; i++) {
    entries[i].hcanvas = canvases[i]
    entries[i].hrem = rems[i]
    entries[i].hsecret = secrets[i]
    entries[i].rem = gridsize * gridsize
    let ctx = canvases[i].getContext('2d')
    let img = ctx.getImageData(0, 0, gridsize, gridsize)
    let data = img.data
    for (let y = 0; y < gridsize; y++) {
      for (let x = 0; x < gridsize; x++) {
        let j = y * gridsize + x
        let [red, green, blue] = [255, 255, 255]
        switch (i) {
          case 0:
            [red, green, blue] = hsl2rgb(360 * x / gridsize, 0.8, lightness)
            break
          case 1:
            [red, green, blue] = hsl2rgb(360 * y / gridsize, 0.8, lightness)
            break
          case 2:
            [red, green, blue] = hsl2rgb(360 * (x + y) / gridsize, 0.8, lightness)
            break
          case 3:
            [red, green, blue] = hsl2rgb(360 * Math.hypot(x - gridsize / 2, y - gridsize / 2) / gridsize * 2, 0.8, lightness)
            break
          case 4:
            [red, green, blue] = hsl2rgb(360 * (gridsize + x - y) / gridsize, 0.8, lightness)
            break
          case 5:
            [red, green, blue] = hsl2rgb(360 * (y / gridsize + Math.sin(x / gridsize * 2 * Math.PI) / 8), 0.8, lightness)
            break
        }
        data[j * 4 + 0] = red
        data[j * 4 + 1] = green
        data[j * 4 + 2] = blue
        data[j * 4 + 3] = 0
        let reveal = false
        switch (i) {
          case 4:
            reveal |= Math.floor(y / radius) < 16
          case 3:
            reveal |= Math.floor(y / radius) % 8 == 0
          case 2:
            reveal |= (x < 2 * radius || x >= gridsize - 2 * radius) || (y < 2 * radius || y >= gridsize - 2 * radius)
          case 1:
            reveal |= (x < 2 * radius || x >= gridsize - 2 * radius) && (y < 2 * radius || y >= gridsize - 2 * radius)
        }
        if (reveal) {
          data[j * 4 + 3] = 255
          entries[i].rem--
        }
      }
    }
    entries[i].img = img
    ctx.putImageData(img, 0, 0)
  }
}

main()
