import { error, iio } from "./iio.js"

declare var LastT: string
declare var eButton: HTMLElement
declare var ePre: HTMLElement

async function clear(): Promise<error> {
  let [_, error] = await iio.Fetch(`/msgz?clearuntil=${LastT}`, { method: "POST" })
  if (error == "") location.reload()
  return Promise.resolve(error)
}

iio.Init()
eButton.onclick = () => {
  iio.Run(clear)
}
eButton.textContent = `Clear until ${LastT}`
if (ePre.textContent == "") {
  eButton.hidden = true
  ePre.innerHTML = `<i>msgz.NoMessages</i>`
}
