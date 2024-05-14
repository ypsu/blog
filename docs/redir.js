const defaultrules = `
# i/ maps to golang's github issues.
rule i [a-zA-Z] https://github.com/golang/go/issues?q=
rule i .* https://github.com/golang/go/issues/
test i https://github.com/golang/go/issues/
test i/123 https://github.com/golang/go/issues/123
test i/is:closed https://github.com/golang/go/issues?q=is:closed

# cs/ searches in golang codebase.
rule cs .* https://sourcegraph.com/search?q=repo:%5Egithub%5C.com/golang/go%24+
test cs/regexp.compile https://sourcegraph.com/search?q=repo:%5Egithub%5C.com/golang/go%24+regexp.compile

# godoc/ searches go documentation.
rule godoc . https://pkg.go.dev/search?q=
test godoc/regexp.compile https://pkg.go.dev/search?q=regexp.compile

# iio/ redirects to iio.ie.
rule iio .* https://iio.ie/
test iio/redir https://iio.ie/redir

# twitter.com/ redirects to nitter.
rule twitter.com .* https://nitter.poast.org/
test twitter.com/carterjwm/status/849813577770778624 https://nitter.poast.org/carterjwm/status/849813577770778624

# youtube.com/ and youtu.be/ redirects to an invidious instance.
rule youtu.be .* https://yewtu.be/watch?v=
rule youtube.com ^watch.*v=([a-zA-Z0-9-]*).* https://yewtu.be/watch?v=$1
rule www.youtube.com ^watch.*v=([a-zA-Z0-9-]*).* https://yewtu.be/watch?v=$1
test youtu.be/9bZkp7q19f0 https://yewtu.be/watch?v=9bZkp7q19f0
test youtube.com/watch?v=9bZkp7q19f0 https://yewtu.be/watch?v=9bZkp7q19f0
test www.youtube.com/watch?v=9bZkp7q19f0 https://yewtu.be/watch?v=9bZkp7q19f0

# custom go links.
rule go ^blog([/?#].*)? https://blog.go.dev$1
rule go ^book([/?#].*)? https://www.gopl.io$1
rule go ^ref([/?#].*)? https://go.dev/ref/spec$1
`

// ruleset is a map of id -> list of replacements.
// replacement is an object containing the {regex, prefix, replacement} fields.
let ruleset = null

// newruleset returns a ruleset object or a string that contains an error message.
function newruleset(cfg) {
  let rs = {}
  let lines = cfg.split('\n')
  for (let lineno in lines) {
    let line = lines[lineno]
    if (line == '' || line.startsWith('#')) continue
    let fields = line.split(' ')
    if (fields[0] != "rule") continue
    if (fields.length != 4) return `rule on line ${parseInt(lineno)+1} has ${fields.length} fields, want 4`
    let repl = {}
    try {
      repl.regex = new RegExp(fields[2])
    } catch (e) {
      return `rule ${fields[1]} on line ${parseInt(lineno)+1}: invalid regex '${fields[2]}': ${e}`
    }
    if (fields[3].includes('$')) {
      repl.replacement = fields[3]
    } else {
      repl.prefix = fields[3]
    }
    let id = fields[1]
    if (!rs[id]) rs[id] = []
    rs[id].push(repl)
  }
  return rs
}

// similar to go's strings.Cut but without the ok result.
function cut(s, sep) {
  let i = s.indexOf(sep)
  if (i == -1) return [s, ""]
  return [s.slice(0, i), s.slice(i + sep.length)]
}

// returns a string, the resulting url.
// if the return value doesn't start with "http", the result is an error message.
function replace(rs, url) {
  if (url.startsWith("?q=")) url = url.slice(3)  // when added via "add a keyword" in ff.
  if (url.startsWith("http://")) url = url.slice(7)
  if (url.startsWith("https://")) url = url.slice(8)
  if (url.startsWith("http%3A%2F%2F")) url = url.slice(13)
  if (url.startsWith("https%3A%2F%2F")) url = url.slice(14)
  let [id, query] = cut(url, '/')
  if (query == '' && url.includes('%2F')) {
    [id, query] = cut(decodeURIComponent(url), '/')
  }
  if (!rs[id]) return `no rule for ${id}`
  for (let r of rs[id]) {
    if (!r.regex.test(query)) continue
    if (r.prefix) return r.prefix + query
    return query.replace(r.regex, r.replacement)
  }
  return `no matching subrule for ${id}`
}

function replaceall(ruleset, text) {
  return text.replaceAll(/[a-z.]*\/\S*\b/g, s => {
    let r = replace(ruleset, s)
    if (!r.startsWith("http")) s
    return `[${s}](${r})`
  })
}

function replaceallraw(ruleset, text) {
  return text.replaceAll(/[a-z.]*\/\S*\b/g, s => {
    let r = replace(ruleset, s)
    if (!r.startsWith("http")) s
    return `<a href="${r}">${s}</a>`
  })
}

// returns an empty string if all successful, otherwise an error message with the errors.
function runtests(rs, cfg) {
  let lines = cfg.split('\n')
  let total = 0
  let failures = 0
  let report = ''
  for (let lineno in lines) {
    let line = lines[lineno]
    if (line == '' || line.startsWith('#')) continue
    let fields = line.split(' ')
    if (fields[0] != "test") continue
    if (fields.length != 3) return `test on line ${parseInt(lineno)+1} has ${fields.length} fields, want 3`
    total++
    let want = fields[2]
    let got = replace(rs, fields[1])
    if (got != want) {
      failures++
      report += `line ${parseInt(lineno)+1}: replace(${fields[1]}) = ${got}, want = ${want}\n`
    }
  }
  if (failures == 0) return ''
  return report + `${failures} / ${total} testcases failed.`
}

function reportError(e) {
  herror.innerText = 'error: ' + e
}

function main() {
  window.onerror = (msg, src, line) => reportError(`${src}:${line} ${msg}`)
  ruleset = newruleset(defaultrules)
  if (typeof ruleset === 'string') {
    herror.innerText = 'data error: ' + ruleset
    return
  }
  let testreport = runtests(ruleset, defaultrules)
  if (testreport != '') {
    reportError(testreport)
    return
  }

  hlinkifysection.children[0].innerHTML = replaceallraw(ruleset, hlinkifysection.children[0].innerHTML)

  htransformdemo.innerText = replaceall(ruleset, htransformdemoinput.children[0].innerText)

  hdemodata.value = defaultrules.slice(1)
  hdemotest.innerText = testreport
  if (testreport == '') {
    hdemotest.innerText = 'all tests passed.'
  }
  hdemodata.onkeyup = () => {
    let rs = newruleset(hdemodata.value)
    if (typeof rs === 'string') {
      hdemotest.innerText = rs
      return
    }
    let testreport = runtests(rs, hdemodata.value)
    hdemotest.innerText = testreport
    if (testreport == '') {
      hdemotest.innerText = 'all tests passed.'
    }
  }

  if (location.hash.length >= 2 && !location.hash.startsWith("comment")) {
    let resolved = replace(ruleset, location.hash.slice(1))
    if (!resolved.startsWith('http')) {
      herror.innerText = 'redirect error: ' + resolved
      return
    }
    window.location.replace(resolved)
  }
}

main()
