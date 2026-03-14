import { iio } from "./iio.js";
let lastT;
async function clear() {
    let [_, error] = await iio.Fetch(`/eventz?clearuntil=${lastT}`, { method: "POST" });
    if (error == "")
        location.reload();
    return Promise.resolve(error);
}
iio.Init();
iio.Run(() => {
    lastT = JSON.parse(eLastT.textContent).LastT;
    eButton.onclick = () => {
        iio.Run(clear);
    };
    eButton.textContent = `Clear until ${lastT}`;
    if (ePre.textContent == "") {
        eButton.hidden = true;
        ePre.innerHTML = `<i>eventz.NoMessages</i>`;
    }
    return Promise.resolve("");
});
