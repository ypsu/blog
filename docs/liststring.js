"use strict";
function join() {
    if (eJoinInput.value == "") {
        eJoinResult.textContent = 'Result: ""';
        return;
    }
    let lines = eJoinInput.value.split("\n");
    let maxcommas = 0;
    for (let line of lines) {
        let commas = 0;
        for (let ch of line) {
            if (ch == ",") {
                commas++;
            }
            else if (ch == " ") {
                maxcommas = Math.max(maxcommas, commas);
                commas = 0;
            }
            else {
                commas = 0;
            }
        }
    }
    let sep = ",";
    for (let i = 0; i < maxcommas; i++)
        sep += ",";
    eJoinResult.textContent = 'Result: "' + sep + " " + lines.join(sep + " ") + '"';
}
function split() {
    let s = eSplitInput.value;
    let si = s.indexOf(" ");
    if (si == -1) {
        eSplitResult.textContent = "Result:";
        return;
    }
    let ss = s.split(s.substring(0, si + 1)).slice(1);
    eSplitResult.textContent = "Result:\n" + ss.join("\n");
}
function init() {
    eJoinInput.onkeyup = join;
    eSplitInput.onkeyup = split;
    join();
    split();
}
init();
