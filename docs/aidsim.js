'use strict'

// potctx, wealthctx are the potential and wealth canvases.
let potctx, wealthctx

// potdata, wealthdata are the imageData objects for the canvases.
let potdata, wealthdata

// ctxw, ctxh are the width and height of the canvases.
let ctxw, ctxh

// population is an array of {potential, wealth} objects representing all people.
let population = Array(100000)

// population is an array of {potential, wealth} objects representing people that passed the filters.
let considered = Array(100000)

// valueCutoffs, wealthCutoffs are a 100 long array of floats.
// they represent the cutoffs for pre-filtering into the considered array.
let [valueCutoffs, wealthCutoffs] = [Array(101), Array(101)]

// gaussian random taken from stackoverflow:
// https://stackoverflow.com/a/36481059
function gaussrand() {
  let u = 1 - Math.random()
  let v = Math.random()
  return Math.sqrt(-2.0 * Math.log(u)) * Math.cos(2.0 * Math.PI * v)
}

function clamp(v, min, max) {
  if (v < min) return min
  if (v > max) return max
  return v
}

function redraw() {
  let valuefilter = valueCutoffs[hvaluefilter.value]
  let wealthfilter = wealthCutoffs[hwealthfilter.value]

  if (happly.value == 0) {
    happlyout.value = '0%, at baseline'
  } else if (happly.value == 100) {
    happlyout.value = '100%, aid distributed'
  } else {
    happlyout.value = `${happly.value}%`
  }
  hvaluefilterout.value = `${hvaluefilter.value}%, only people with ${Math.floor(valuefilter*100)/100} or more value`
  hwealthfilterout.value = `${hwealthfilter.value}%, only people with ${Math.floor(wealthfilter*100)/100} or less wealth`
  if (hrecv.value == 0) {
    hrecvout.value = 'nobody'
  } else if (hrecv.value == 100) {
    hrecvout.value = 'everyone'
  } else {
    hrecvout.value = `random ${hrecv.value}%`
  }

  let consideredLength = 0
  for (let i = 0; i < population.length; i++) {
    let p = population[i]
    p.considered = false
    if (p.wealth > wealthfilter) continue
    let value = p.potential * Math.log(p.wealth)
    if (value < valuefilter) continue
    considered[consideredLength++] = p
    p.considered = true
  }
  hconsideredout.value = consideredLength

  potdata.data.fill(0)
  wealthdata.data.fill(0)

  let extravalue = 0
  for (let i = 0; i < population.length; i++) {
    if (population[i].considered) continue
    let pot = population[i].potential
    let wealth = population[i].wealth
    let value = pot * Math.log(wealth)
    let x, y, off

    x = Math.floor(pot / 2 * ctxw)
    y = ctxh - Math.ceil(value / 5 * ctxh)
    if (x < ctxw && y >= 0) {
      off = (y * ctxw + x) * 4
      potdata.data[off] = 0xa0
      potdata.data[off + 1] = 0xa0
      potdata.data[off + 2] = 0xa0
      potdata.data[off + 3] = 0xff
    }

    x = Math.floor((wealth - 1) / 5 * ctxw)
    y = ctxh - Math.ceil(value / 5 * ctxh)
    if (x < ctxw && y >= 0) {
      off = (y * ctxw + x) * 4
      wealthdata.data[off] = 0xa0
      wealthdata.data[off + 1] = 0xa0
      wealthdata.data[off + 2] = 0xa0
      wealthdata.data[off + 3] = 0xff
    }
  }

  let selectedLength = Math.floor(consideredLength * hrecv.value / 100)
  hselectedout.value = selectedLength
  let extrawealth = 0
  if (haid100.checked) extrawealth = 100
  if (haid1k.checked) extrawealth = 1000
  if (haid10k.checked) extrawealth = 10 * 1000
  if (haid100k.checked) extrawealth = 100 * 1000
  if (haid1000k.checked) extrawealth = 1000 * 1000
  extrawealth /= selectedLength
  haidppout.value = Math.floor(extrawealth * 1000) / 1000
  extrawealth *= happly.value / 100

  for (let i = 0; i < consideredLength; i++) {
    let pot = considered[i].potential
    let wealth = considered[i].wealth
    let basevalue = pot * Math.log(wealth)
    let value = i < selectedLength ? pot * Math.log(wealth + extrawealth) : basevalue;
    extravalue += value - basevalue
    let x, y, off

    x = Math.floor(pot / 2 * ctxw)
    y = ctxh - Math.ceil(value / 5 * ctxh)
    if (x < ctxw && y >= 0) {
      off = (y * ctxw + x) * 4
      potdata.data[off] = 0xff
      potdata.data[off + 1] = i < selectedLength ? 0 : 0xff
      potdata.data[off + 2] = 0x00
      potdata.data[off + 3] = 0xff
    }

    x = Math.floor((wealth - 1) / 5 * ctxw)
    y = ctxh - Math.ceil(value / 5 * ctxh)
    if (x < ctxw && y >= 0) {
      off = (y * ctxw + x) * 4
      wealthdata.data[off] = 0xff
      wealthdata.data[off + 1] = i < selectedLength ? 0 : 0xff
      wealthdata.data[off + 2] = 0x00
      wealthdata.data[off + 3] = 0xff
    }
  }

  potctx.putImageData(potdata, 0, 0)
  wealthctx.putImageData(wealthdata, 0, 0)

  hextraout.value = Math.floor(extravalue)
}

function main() {
  // initialize the canvasses.
  ctxw = hpotcanvas.clientWidth
  ctxh = hpotcanvas.clientHeight
  hpotcanvas.width = ctxw
  hpotcanvas.height = ctxh
  hwealthcanvas.width = ctxw
  hwealthcanvas.height = ctxh
  potctx = hpotcanvas.getContext('2d')
  wealthctx = hwealthcanvas.getContext('2d')
  potdata = potctx.getImageData(0, 0, ctxw, ctxh)
  wealthdata = wealthctx.getImageData(0, 0, ctxw, ctxh)

  // generate random population data.
  for (let i = 0; i < population.length; i++) {
    population[i] = {
      potential: clamp(gaussrand() + 1, -1e9, 1e9),
      wealth: clamp(gaussrand() + Math.E, 1, 1e9),
    }
  }

  // compute the cutoff values.
  population.sort((a, b) => {
    return a.wealth - b.wealth
  })
  for (let i = 1; i <= 100; i++) {
    let p = i * population.length / 100 - 1
    wealthCutoffs[i] = population[p].wealth
  }
  population.sort((a, b) => {
    return b.potential * Math.log(b.wealth) - a.potential * Math.log(a.wealth)
  })
  for (let i = 1; i <= 100; i++) {
    let p = i * population.length / 100 - 1
    valueCutoffs[i] = population[p].potential * Math.log(population[p].wealth)
  }

  // shuffle the population array so that random selection is just a prefix operation.
  for (let i = population.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [population[i], population[j]] = [population[j], population[i]];
  }

  hpopout.value = population.length
  redraw()
}

hdemo.innerHTML = `
<style>
canvas {
  border: solid;
  border-width: 1px;
  height: 10em;
  width: 100%;
}
</style>
potential to value scatterplot:
<canvas id=hpotcanvas></canvas>
wealth to value scatterplot:
<canvas id=hwealthcanvas></canvas>

population: <output id=hpopout>0</output>,
considered: <output id=hconsideredout>0</output><br>
selected: <output id=hselectedout>0</output>,
aid/person: <output id=haidppout>0</output>
<br>
total extra value: <output id=hextraout>0</output><br>
<label for=happly>drag to 100% to apply (<output id=happlyout></output>): </label>
<input type=range id=happly min=0 max=100 step=1 style=width:100% oninput=redraw() value=0>
<br>
aid available:
<input type=radio name=haid id=haid100 oninput=redraw()><label for=haid100>100</label>
<input type=radio name=haid id=haid1k oninput=redraw()><label for=haid1k>1k</label>
<input type=radio name=haid id=haid10k checked oninput=redraw()><label for=haid10k>10k</label>
<input type=radio name=haid id=haid100k oninput=redraw()><label for=haid100k>100k</label>
<input type=radio name=haid id=haid1000k oninput=redraw()><label for=haid1000k>1000k</label>
<br>
worthiest <output id=hvaluefilterout></output>:
<input type=range id=hvaluefilter min=1 max=100 step=1 style=width:100% oninput=redraw() value=100>
<br>
poorest <output id=hwealthfilterout></output>:
<input type=range id=hwealthfilter min=1 max=100 step=1 style=width:100% oninput=redraw() value=100>
<br>
receivers (<output id=hrecvout></output>):
<input type=range id=hrecv min=1 max=100 step=1 style=width:100% oninput=redraw() value=100>
`

main()
