interface WorkerScope {
  onmessage: ((this: WorkerScope, ev: MessageEvent<string>) => any) | null
  postMessage(message: string): void
}

const ctx = self as unknown as WorkerScope

let testdata: string[]
let task: string
let testcase: string
let input: string
let logCalls: number
let logBytes: number
let logStopped: boolean

function log(...args: any[]) {
  if (logStopped) return
  let formatted = args.map((arg) => String(arg))
  let msg = formatted.join(" ")
  if (!msg.endsWith("\n")) msg += "\n"
  logCalls++
  logBytes += msg.length
  if (logCalls > 1000 || logBytes > 20000) {
    logStopped = true
    msg = "[too much log data, dropping further logs]\n"
  }
  ctx.postMessage("log " + msg)
}

ctx.onmessage = (event: MessageEvent<string>) => {
  let code = event.data
  let fn: Function
  try {
    fn = new Function(code)
  } catch (e) {
    if (e instanceof Error) {
      ctx.postMessage(
        "error " +
          e.message +
          "\n\n(Apologies for the rudimentary error message.\nJavascript eval() doesn't tell me the line number of the error.\nUse the full version for serious programming.)",
      )
    } else {
      ctx.postMessage("error " + String(e))
    }
    ctx.postMessage("done")
    return
  }

  try {
    let i
    for (i = 0; i < testdata.length; i += 4) {
      logCalls = 0
      logBytes = 0
      logStopped = false
      task = testdata[i]
      testcase = testdata[i + 1]
      input = testdata[i + 2]
      let want = testdata[i + 3]
      if (want.startsWith("ERROR")) want = "ERROR"
      ctx.postMessage(`test ${task}.${testcase} (${i / 4 + 1}/${testdata.length / 4})\n${input}`)
      ctx.postMessage(`want ${want}`)
      let got = fn()
      if (typeof got != "string") got = "non-string return value"
      if (got == "SKIP") continue
      let shortgot = got
      if (got.startsWith("ERROR")) shortgot = "ERROR"
      if (shortgot != want) {
        ctx.postMessage("result " + got)
        break
      }
    }
    if (i == testdata.length) ctx.postMessage("success")
  } catch (e) {
    if (e instanceof Error) {
      ctx.postMessage(
        "error " +
          e.message +
          "\n\n(Apologies for the rudimentary error message.\nJavascript eval() doesn't tell me the line number of the error.\nUse the full version for serious programming.)",
      )
    } else {
      ctx.postMessage("error " + String(e))
    }
  }
  ctx.postMessage("done")
}

async function init() {
  let status = async () => {
    try {
      let rsp = await fetch("sdinthrough.data")
      if (!rsp.ok) return "sdinthrough.StartDataFetch: " + rsp.status
      let data = await rsp.text()
      testdata = data.split("===\n")
    } catch (err) {
      return "sdinthrough.FetchData: " + String(err)
    }
    return "started"
  }
  ctx.postMessage(await status())
}

init()

/*
!tags unsafe-eval
*/
