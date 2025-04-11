export { error, iio, strings }

declare var eError: HTMLElement

type error = string

let strings = {
  // Cut slices s around the first instance of sep, returning the text before and after it.
  Cut: function (s: string, sep: string): [string, string] {
    if (!s) return ["", ""]
    let i = s.indexOf(sep)
    if (i == -1) return [s, ""]
    return [s.substring(0, i), s.substring(i + sep.length)]
  },

  FormatDate: function (date: Date, sep?: string): string {
    if (!sep) sep = "-"
    let year = `${date.getUTCFullYear()}`
    let month = `${date.getUTCMonth() + 1}`.padStart(2, "0")
    let day = `${date.getUTCDate()}`.padStart(2, "0")
    return year + sep + month + sep + day
  },
}

let iio = {
  // Panic notifies the user about the passed in error.
  Panic: function (msg: string) {
    let s = `${msg}`
    eError.innerText = `Error: ${s.trim()}\n\nReload the page to try again.`
    eError.hidden = false
    document.body.classList.add("cbgNeutral")
  },

  // Run runs run() and panics if it returns an error.
  Run: async function (run: () => Promise<error>) {
    let err = await run()
    if (err != "") iio.Panic(err)
  },

  // Fetch returns body or error string.
  // The error string is empty iff there was no error.
  Fetch: async function (input: string | Request, options?: RequestInit): Promise<[string, error]> {
    let req = new Request(input, options)
    try {
      let response = await fetch(req)
      let text = await response.text()
      if (response.status >= 300) return Promise.resolve(["", `${response.status} ${response.statusText}: ${text.trim()}`])
      return Promise.resolve([text, ""])
    } catch (e) {
      return ["", `iio.FetchException: ${e}`.trim()]
    }
  },

  Init: function () {
    window.onerror = (msg, src, line) => iio.Panic(`${src}:${line} ${msg}`)
    window.onunhandledrejection = (e) => iio.Panic(e.reason)
  },
}
