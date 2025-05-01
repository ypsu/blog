import { iio } from "./iio.js";
async function clear() {
    let [_, error] = await iio.Fetch(`/eventz?clearuntil=${LastT}`, { method: "POST" });
    if (error == "")
        location.reload();
    return Promise.resolve(error);
}
iio.Init();
eButton.onclick = () => {
    iio.Run(clear);
};
eButton.textContent = `Clear until ${LastT}`;
if (ePre.textContent == "") {
    eButton.hidden = true;
    ePre.innerHTML = `<i>eventz.NoMessages</i>`;
}
