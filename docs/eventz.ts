import { error, iio } from "./iio.js"

declare var eButton: HTMLElement
declare var eLastT: HTMLElement
declare var ePre: HTMLElement
let lastT: number

async function clear(): Promise<error> {
  let [_, error] = await iio.Fetch(`/eventz?clearuntil=${lastT}`, { method: "POST" })
  if (error == "") location.reload()
  return Promise.resolve(error)
}

iio.Init()
iio.Run(() => {
  lastT = JSON.parse(eLastT.textContent).LastT
  eButton.onclick = () => {
    iio.Run(clear)
  }
  eButton.textContent = `Clear until ${lastT}`
  if (ePre.textContent == "") {
    eButton.hidden = true
    ePre.innerHTML = `<i>eventz.NoMessages</i>`
  }
  return Promise.resolve("")
})
