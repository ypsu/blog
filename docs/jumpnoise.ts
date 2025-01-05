/*! Note for the generated js: original typescript source in condjump.ts. */

declare var hAdvancedSwitcher: HTMLInputElement
declare var hCode: HTMLElement
declare var hCodeSelector: HTMLSelectElement
declare var hCondensedCode: HTMLElement
declare var hCustomDetails: HTMLDetailsElement
declare var hDemo: HTMLElement
declare var hDemoSwitcher: HTMLInputElement
declare var hError: HTMLElement
declare var hJSNote: HTMLElement
declare var hPermalink: HTMLAnchorElement
declare var hStyleSelector: HTMLSelectElement

declare var hEO: HTMLInputElement
declare var hNV: HTMLInputElement
declare var hWV: HTMLInputElement

function seterror(msg: string) {
  hError.innerText = `Error: ${msg}.\nReload the page to try again.`
  hError.hidden = false
  document.body.classList.add("cbgNeutral")
}

let condjumpRE = /\bif ([^;\n]*?)[\t ]*\{[\t ]*\n[\t ]*(break|continue|goto|return)\b[\t ]*([^;\n]*?)[\t ]*\n[\t ]*\}[\t ]*$/gm
let defaultExprRE = /^\s*(0|nil|""|[\w_.]*\{\})\s*$/
let errFn = /^\s*[\w.&]*Err/

function condense(_: string, ...args: string[]) {
  let [cond, keyword, values] = [args[0], args[1], args[2]]
  let hasParams = values != ""

  // Check if the non-err args are empty or not.
  // This is not perfect but enough for a barebones demo.
  let returnArgs = values.split(",")
  while (returnArgs.length >= 1 && returnArgs[0].match(defaultExprRE)) returnArgs.shift()
  let propagate = (returnArgs.length == 1 && returnArgs[0].trim() == "err") || (returnArgs.length >= 1 && returnArgs[0].match(errFn) != null)
  let err = returnArgs.join(",").trim()

  let replace = (m: string) => {
    if (m == "K") return keyword
    if (m == "C") return cond
    if (m == "V") return values
    if (m == "E") return err
    return "???"
  }

  if (!hasParams) return hNV.value.replaceAll(/[KCVE]/g, replace)
  if (hEO.value != "" && propagate) return hEO.value.replaceAll(/[KCVE]/g, replace)
  return hWV.value.replaceAll(/[KCVE]/g, replace)
}

function renderDemo() {
  if (hDemoSwitcher.checked) {
    hDemo.innerText = samples["containsStack"].replaceAll(condjumpRE, condense)
  } else {
    hDemo.innerText = samples["containsStack"]
  }
}

function renderAdvancedDemo() {
  hCode.hidden = false
  hCondensedCode.hidden = false
  if (hAdvancedSwitcher.checked) {
    hCondensedCode.innerText = hCode.innerText.replaceAll(condjumpRE, condense)
    hCode.hidden = true
    return
  }
  hCondensedCode.hidden = true
}

function pickSample(name: string) {
  let c = samples[name]
  if (c != null) hCode.innerText = c
  renderAdvancedDemo()
}

function applyRules() {
  let [a, b, c] = [hNV.value, hWV.value, hEO.value]
  a = a.replace("S", "s")
  b = b.replace("S", "s")
  c = c.replace("S", "s")
  location.hash = "#" + encodeURI(`${a}S${b}S${c}`)
}

function applyHash() {
  let parts = decodeURIComponent(location.hash.slice(1)).split("S")
  if (parts.length != 3) return
  hNV.value = parts[0]
  hWV.value = parts[1]
  hEO.value = parts[2]

  let i = 0
  for (i = 0; i < styles.length; i++) {
    let style = styles[i]
    if (parts[0] == style[1] && parts[1] == style[2] && parts[2] == style[3]) break
  }
  hStyleSelector.selectedIndex = i

  let url = location.origin + location.pathname + location.hash
  hPermalink.href = url
  hPermalink.innerText = url
  renderAdvancedDemo()
}

function pickStyle(idx: number) {
  if (idx == styles.length) hCustomDetails.open = true
  if (idx < 0 || idx >= styles.length) return
  hNV.value = styles[idx][1]
  hWV.value = styles[idx][2]
  hEO.value = styles[idx][3]
  applyRules()
}

function main() {
  window.onerror = (msg, src, line) => seterror(`${src}:${line} ${msg}`)
  window.onunhandledrejection = (e) => seterror(e.reason)
  window.onhashchange = applyHash
  hCode.oninput = () => (hCodeSelector.selectedIndex = Object.keys(samples).length)
  hJSNote.hidden = true
  hDemo.innerText = samples["containsStack"]

  let h = ""
  for (let sample in samples) {
    h += `<option value=${sample}>${sample}\n`
  }
  h += "<option value=custom>custom\n"
  hCodeSelector.innerHTML = h

  h = ""
  for (let i in styles) {
    h += `<option value=${i}>${styles[i][0]}\n`
  }
  h += `<option value=${styles.length}>custom\n`
  hStyleSelector.innerHTML = h

  pickSample("readnote")
  if (location.hash.length <= 1) pickStyle(0)
  applyHash()
}

let samples: { [k: string]: string } = {
  readnote: `func readnote(f *elf.File, name []byte, typ int32) ([]byte, error) {
	for _, sect := range f.Sections {
		if sect.Type != elf.SHT_NOTE {
			continue
		}
		r := sect.Open()
		for {
			var namesize, descsize, noteType int32
			err := binary.Read(r, f.ByteOrder, &namesize)
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("read namesize failed: %v", err)
			}
			err = binary.Read(r, f.ByteOrder, &descsize)
			if err != nil {
				return nil, fmt.Errorf("read descsize failed: %v", err)
			}
			err = binary.Read(r, f.ByteOrder, &noteType)
			if err != nil {
				return nil, fmt.Errorf("read type failed: %v", err)
			}
			noteName, err := readwithpad(r, namesize)
			if err != nil {
				return nil, fmt.Errorf("read name failed: %v", err)
			}
			desc, err := readwithpad(r, descsize)
			if err != nil {
				return nil, fmt.Errorf("read desc failed: %v", err)
			}
			if string(name) == string(noteName) && typ == noteType {
				return desc, nil
			}
		}
	}
	return nil, nil
}`,

  CopyFile: `// this is a variant of https://go.googlesource.com/proposal/+/master/design/go2draft-error-handling-overview.md#draft-design.
// it uses a little trick from https://github.com/golang/go/issues/48855 to make it cleaner.
// might not be the least amount of code but the condensed code is quite straight and thus easy to read.

func CopyFile(src, dst string) error {
	r, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy %s %s: %v", src, dst, err)
	}
	defer r.Close()

	w, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("copy %s %s: %v", src, dst, err)
	}

	cleanupAndReturn := func(err error) error {
		w.Close()
		os.Remove(dst)
		return err
	}

	_, err = io.Copy(w, r)
	if err != nil {
		return cleanupAndReturn(fmt.Errorf("copy %s %s: %v", src, dst, err))
	}
	err = w.Close()
	if err != nil {
		return cleanupAndReturn(fmt.Errorf("copy %s %s: %v", src, dst, err))
	}
	return nil
}`,

  containsStack: `func containsStack(got [][]string, want []string) bool {
	for _, stk := range got {
		if len(stk) < len(want) {
			continue
		}
		for i, f := range want {
			if f != stk[i] {
				break
			}
			if i == len(want)-1 {
				return true
			}
		}
	}
	return false
}`,

  parse: `func (p *Parser) Parse() (*obj.Prog, bool) {
	scratch := make([][]lex.Token, 0, 3)
	for {
		word, cond, operands, ok := p.line(scratch)
		if !ok {
			break
		}
		scratch = operands

		if p.pseudo(word, operands) {
			continue
		}
		i, present := p.arch.Instructions[word]
		if present {
			p.instruction(i, word, cond, operands)
			continue
		}
		p.errorf("unrecognized instruction %q", word)
	}
	if p.errorCount > 0 {
		return nil, false
	}
	p.patch()
	return p.firstProg, true
}`,

  netpollBreak: `// netpollBreak interrupts an epollwait.
func netpollBreak() {
	// Failing to cas indicates there is an in-flight wakeup, so we're done here.
	if !netpollWakeSig.CompareAndSwap(0, 1) {
		return
	}

	var one uint64 = 1
	oneSize := int32(unsafe.Sizeof(one))
	for {
		n := write(netpollEventFd, noescape(unsafe.Pointer(&one)), oneSize)
		if n == oneSize {
			break
		}
		if n == -_EINTR {
			continue
		}
		if n == -_EAGAIN {
			return
		}
		println("runtime: netpollBreak write failed with", -n)
		throw("runtime: netpollBreak write failed")
	}
}`,

  refineNonZeroes: `// refineNonZeroes refines non-zero entries of b in zig-zag order. If nz >= 0,
// the first nz zero entries are skipped over.
func (d *decoder) refineNonZeroes(b *block, zig, zigEnd, nz, delta int32) (int32, error) {
	for ; zig <= zigEnd; zig++ {
		u := unzig[zig]
		if b[u] == 0 {
			if nz == 0 {
				break
			}
			nz--
			continue
		}
		bit, err := d.decodeBit()
		if err != nil {
			return 0, err
		}
		if !bit {
			continue
		}
		if b[u] >= 0 {
			b[u] += delta
		} else {
			b[u] -= delta
		}
	}
	return zig, nil
}`,

  readlink: `func readlink(name string) (string, error) {
	for len := 128; ; len *= 2 {
		b := make([]byte, len)
		var (
			n int
			e error
		)
		for {
			n, e = fixCount(syscall.Readlink(name, b))
			if e != syscall.EINTR {
				break
			}
		}
		// buffer too small
		if (runtime.GOOS == "aix" || runtime.GOOS == "wasip1") && e == syscall.ERANGE {
			continue
		}
		if e != nil {
			return "", &PathError{Op: "readlink", Path: name, Err: e}
		}
		if n < len {
			return string(b[0:n]), nil
		}
	}
}`,

  parseThreadSample: `// parseThreadSample parses a symbolized or unsymbolized stack trace.
// Returns the first line after the traceback, the sample (or nil if
// it hits a 'same-as-previous' marker) and an error.
func parseThreadSample(s *bufio.Scanner) (nextl string, addrs []uint64, err error) {
	var line string
	sameAsPrevious := false
	for s.Scan() {
		line = strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "---") {
			break
		}
		if strings.Contains(line, "same as previous thread") {
			sameAsPrevious = true
			continue
		}

		curAddrs, err := parseHexAddresses(line)
		if err != nil {
			return "", nil, fmt.Errorf("malformed sample: %s: %v", line, err)
		}
		addrs = append(addrs, curAddrs...)
	}
	if err := s.Err(); err != nil {
		return "", nil, err
	}
	if sameAsPrevious {
		return line, nil, nil
	}
	return line, addrs, nil
}`,

  parseAddressList: `func (p *addrParser) parseAddressList() ([]*Address, error) {
	var list []*Address
	for {
		p.skipSpace()

		// allow skipping empty entries (RFC5322 obs-addr-list)
		if p.consume(',') {
			continue
		}

		addrs, err := p.parseAddress(true)
		if err != nil {
			return nil, err
		}
		list = append(list, addrs...)

		if !p.skipCFWS() {
			return nil, errors.New("mail: misformatted parenthetical comment")
		}
		if p.empty() {
			break
		}
		if p.peek() != ',' {
			return nil, errors.New("mail: expected comma")
		}

		// Skip empty entries for obs-addr-list.
		for p.consume(',') {
			p.skipSpace()
		}
		if p.empty() {
			break
		}
	}
	return list, nil
}`,
}

let styles = [
  ["above demo", "ifK C", "ifK C, V", ""],
  ["above demo with zero-values omitted", "ifK C", "ifK C, V", "ifK C, E"],
  ["above demo with parens", "ifK(C)", "ifK(C, V)", ""],
  ["on keyword", "on C, K", "on C, K V", ""],
  ["single line if", "if C { K }", "if C { K V }", ""],
  ["single line if with keyword on left", "K if C", "K if C { V }", ""],
]

main()
