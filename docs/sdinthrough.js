import { iio } from "./iio.js";
function init() {
    if (typeof Worker === "undefined")
        return;
    try {
        let code = localStorage.getItem("sdinthroughCode");
        if (code != null && code != "")
            eCode.value = code;
    }
    catch (e) { }
    eCode.onkeydown = (ev) => {
        if (ev.key == "Tab") {
            ev.preventDefault();
            eCode.setRangeText("\t", eCode.selectionStart, eCode.selectionEnd, "end");
            return;
        }
        if ((ev.shiftKey || ev.ctrlKey) && ev.key == "Enter") {
            ev.preventDefault();
            judge();
            return;
        }
    };
    eJudgeButton.onclick = judge;
    judge();
}
var workerState;
(function (workerState) {
    workerState[workerState["terminated"] = 0] = "terminated";
    workerState[workerState["starting"] = 1] = "starting";
    workerState[workerState["running"] = 2] = "running";
    workerState[workerState["idle"] = 3] = "idle";
})(workerState || (workerState = {}));
let worker;
let timeoutID = 0;
let state = workerState.terminated;
let testcase = "";
let testinput = "";
let testoutput = "";
let log = "";
function onWorkerMessage(ev) {
    if (ev.target != worker || state == workerState.terminated)
        return;
    let msg = ev.data;
    if (state == workerState.starting) {
        clearTimeout(timeoutID);
        if (msg != "started") {
            eJudgeResult.innerText = "Error: " + msg;
            state = workerState.terminated;
            worker.terminate();
            return;
        }
        eJudgeResult.innerText = "The worker is running...";
        state = workerState.idle;
        judge();
        return;
    }
    if (state != workerState.running)
        iio.Panic("sdinthrough.UnexpectedRunningState state=" + state);
    if (msg == "done") {
        clearTimeout(timeoutID);
        state = workerState.idle;
        return;
    }
    if (msg == "success") {
        eJudgeResult.innerText = "Congratulations, you solved the demo challenge!";
        return;
    }
    let directive = msg.slice(0, msg.indexOf(" "));
    let param = msg.slice(directive.length + 1);
    if (directive == "error") {
        if (testcase != "") {
            eJudgeResult.innerText = `== ${testcase} ==\n${testinput}\n== error ==\n${param}`;
        }
        else {
            eJudgeResult.innerText = `== error ==\n${param}`;
        }
        return;
    }
    if (directive == "log") {
        log += param;
        return;
    }
    if (directive == "test") {
        testcase = param.slice(0, param.indexOf("\n"));
        testinput = param.slice(testcase.length + 1);
        log = "";
        return;
    }
    if (directive == "want") {
        testoutput = param;
        return;
    }
    if (directive == "result") {
        let verdict = "wrong answer";
        let testoutputNS = testoutput.replace(/[ \t\n]/g, "");
        let got = param;
        let gotNS = got.replace(/[ \t\n]/g, "");
        if (testoutputNS == gotNS) {
            verdict = "mismatching whitespace (symbolized for clarity)";
            testinput = testinput.replaceAll(" ", "·");
            testinput = testinput.replaceAll("\t", "»");
            testinput = testinput.replaceAll("\n", "↵\n");
            testoutput = testoutput.replaceAll(" ", "·");
            testoutput = testoutput.replaceAll("\t", "»");
            testoutput = testoutput.replaceAll("\n", "↵\n");
            got = got.replaceAll(" ", "·");
            got = got.replaceAll("\t", "»");
            got = got.replaceAll("\n", "↵\n");
        }
        let r = `== ${testcase} ==\n${testinput}\n== got output ==\n${got}\n== want output ==\n${testoutput}\n== verdict ==\n${verdict}`;
        if (log != "")
            r += "\n\n== log ==\n" + log;
        eJudgeResult.innerText = r;
    }
}
function terminate() {
    if (state == workerState.starting) {
        eJudgeResult.innerText =
            "Error: couldn't start the web worker.\nEither there are network issues or your browser doesn't support them. Try Chrome?\nSorry, I can't programmatically tell what is the issue, the javascript API doesn't expose it. :(";
        state = workerState.terminated;
        worker.terminate();
        return;
    }
    if (state != workerState.running)
        iio.Panic("sdinthrough.UnexpectedTerminatingState state=" + state);
    state = workerState.terminated;
    worker.terminate();
    let r = "== verdict ==\nsdinthrough.Timeout";
    if (testcase != "") {
        r = `== ${testcase} ==\n${testinput}\n== want output ==\n${testoutput}\n== verdict ==\nsdinthrough.Timeout`;
    }
    if (log != "")
        r += "\n\n== log ==\n" + log;
    eJudgeResult.innerText = r;
}
function judge() {
    try {
        localStorage.setItem("sdinthroughCode", eCode.value);
    }
    catch (e) { }
    if (state == workerState.starting || state == workerState.running)
        return;
    if (state == workerState.terminated) {
        state = workerState.starting;
        eJudgeResult.innerText = "Starting the web worker...";
        worker = new Worker("sdinthroughworker.js");
        worker.onmessage = onWorkerMessage;
        timeoutID = setTimeout(terminate, 3000);
        return;
    }
    worker.postMessage(eCode.value);
    testcase = "";
    state = workerState.running;
    timeoutID = setTimeout(terminate, 500);
}
init();
