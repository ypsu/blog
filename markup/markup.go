// Package markup provides basic markdown-like features for the blog.
package markup

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
)

func Render(input string, restricted bool) string {
	w := &strings.Builder{}
	lines := strings.SplitAfter(input, "\n")
	n, i := len(lines), 0
	for i < n {
		line, trimmedLine := lines[i], strings.TrimSpace(lines[i])
		switch {
		case trimmedLine == "":
			w.WriteString(line)
			i++

		case !restricted && strings.HasPrefix(line, "# "):
			w.WriteString("<h1>")
			writeline(w, strings.TrimPrefix(trimmedLine, "# "), false)
			w.WriteString("</h1>\n")
			i++

		case !restricted && strings.HasPrefix(line, "## "):
			header := strings.TrimPrefix(trimmedLine, "## ")
			id := identify(header)
			if title, note, ok := strings.Cut(header, ":"); ok {
				fmt.Fprintf(w, "<h2 id=%s><a href=#%s>%s</a>:", id, id, html.EscapeString(title))
				writeline(w, note, false)
				w.WriteString("</h2>\n")
			} else {
				fmt.Fprintf(w, "<h2 id=%s><a href=#%s>%s</a></h2>\n", id, id, html.EscapeString(header))
			}
			i++

		case strings.HasPrefix(line, "```") && !strings.Contains(strings.TrimLeft(line, "`"), "`"):
			pf := line[:3]
			for j := 3; j < len(line) && line[j] == '`'; j++ {
				pf = line[:j+1]
			}
			j := i + 1
			for j < n && !strings.HasPrefix(lines[j], pf) {
				j++
			}
			writeblock(w, strings.TrimSpace(line[len(pf):]), lines[i+1:j])
			i = j + 1

		case strings.HasPrefix(line, "  "):
			w.WriteString("<pre class=cSource>")
			for i < n && (strings.HasPrefix(lines[i], "  ") || i+1 < n && strings.HasPrefix(lines[i+1], "  ")) {
				w.WriteString(html.EscapeString(strings.TrimPrefix(lines[i], "  ")))
				i++
			}
			w.WriteString("</pre>\n")

		case strings.HasPrefix(line, "> "):
			w.WriteString("<blockquote>\n")
			for i < n && strings.HasPrefix(lines[i], "> ") {
				writeline(w, strings.TrimPrefix(lines[i], "> "), restricted)
				i++
			}
			w.WriteString("</blockquote>\n")

		case strings.HasPrefix(line, "- "):
			w.WriteString("<ul>\n")
			for i < n && strings.TrimSpace(lines[i]) != "" {
				line := lines[i]
				if strings.HasPrefix(line, "- ") {
					line = line[2:]
					w.WriteString("<li>")
				}
				writeline(w, line, restricted)
				i++
			}
			w.WriteString("</ul>\n")

		case !restricted && strings.HasPrefix(line, "!html "):
			for i < n && strings.TrimSpace(lines[i]) != "" {
				line = strings.TrimPrefix(lines[i], "!html")
				line = strings.TrimPrefix(line, " ")
				w.WriteString(line)
				i++
			}

		case !restricted && strings.HasPrefix(line, "!pubdate"):
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				fmt.Fprintf(w, "<p class=cPublishDate><i>Published on %s, last modified on %s.</i></p>", fields[1], fields[2])
			} else if len(fields) >= 2 {
				fmt.Fprintf(w, "<p class=cPublishDate><i>Published on %s.</i></p>", fields[1])
			}
			i++

		case !restricted && strings.HasPrefix(line, "!tags"):
			// Simply ignore.
			i++

		default:
			w.WriteString("<p>\n")
			for i < n && strings.TrimSpace(lines[i]) != "" {
				writeline(w, lines[i], restricted)
				i++
			}
			w.WriteString("</p>\n")
		}
	}
	return w.String()
}

func writeblock(w *strings.Builder, tag string, lines []string) {
	switch {
	case tag == "numbered":
		w.WriteString("<pre class=cSource>")
		for i, line := range lines {
			fmt.Fprintf(w, "%2d| %s", i+1, html.EscapeString(line))
		}
		w.WriteString("</pre>\n")

	default:
		w.WriteString("<pre class=cSource>")
		if tag != "" {
			fmt.Fprintf(w, "[UnrecognizedTag:%s]\n", html.EscapeString(tag))
		}
		for _, line := range lines {
			w.WriteString(html.EscapeString(line))
		}
		w.WriteString("</pre>\n")
	}
}

var linkRe = regexp.MustCompile(`\bhttps?://[-.a-z0-9]+(\S*)?\b[/;]?`)
var postRe = regexp.MustCompile(`@/\S+\b[/;]?`)
var anchorRe = regexp.MustCompile(`@#\S+\b[/;]?`)
var rawAnchorRe = regexp.MustCompile(`(?m)(?:^| )(#c[0-9-]+|#[A-Z][A-Za-z0-9-]+)`)

func writeline(w *strings.Builder, line string, restricted bool) {
	if strings.Contains(line, "`") {
		nw, i, n := &strings.Builder{}, 0, len(line)
		for i < n {
			c := line[i]

			switch {
			case c == '`' && (i+1 == n || line[i+1] != '`'):
				j := i + 1
				for j < n && line[j] != '`' {
					j++
				}
				nw.WriteString("<tt class=cInlineSource>")
				nw.WriteString(html.EscapeString(line[i+1 : j]))
				nw.WriteString("</tt>")
				i = j + 1

			case c == '`':
				open := 2
				for i+open < n && line[i+open] == '`' {
					open++
				}
				ts, te := i+open, i+open
				for te < n && line[te] != '(' {
					te++
				}
				tag, run := line[ts:te], 0
				i = te + 1
				for i < n && run < open {
					if line[i] == '`' {
						run++
					} else {
						run = 0
					}
					i++
				}
				content := line[te+1 : max(te+1, i-open-1)]
				switch {
				case tag == "comment":
					// Write nothing, this is secret content.
				case tag == "tt":
					fmt.Fprintf(nw, "<span class=cInlineSource>%s</span>", html.EscapeString(content))
				case !restricted && tag == "raw":
					nw.WriteString(content)
				case !restricted && strings.HasPrefix(tag, "."):
					fmt.Fprintf(nw, "<span class=%s>%s</span>", tag[1:], html.EscapeString(content))
				default:
					fmt.Fprintf(nw, "[UnrecognizedTag:%s:%s]", html.EscapeString(tag), html.EscapeString(content))
				}

			default:
				nw.WriteString(html.EscapeString(line[i : i+1]))
				i++
			}
		}
		line = nw.String()
	} else {
		line = html.EscapeString(line)
	}

	if restricted {
		// Always linkify comment references.
		line = rawAnchorRe.ReplaceAllStringFunc(line, func(link string) string {
			var prefix string
			if link[0] == ' ' {
				prefix, link = " ", link[1:]
			}
			return fmt.Sprintf("%s<a href='%s'>%s</a>", prefix, link, link)
		})
		w.WriteString(line)
		return
	}

	// Linkify
	line = linkRe.ReplaceAllStringFunc(line, func(link string) string {
		link, suffix := stripLinkSuffix(link)
		return fmt.Sprintf("<a href='%s'>%s</a>%s", link, link, suffix)
	})
	line = postRe.ReplaceAllStringFunc(line, func(link string) string {
		link, suffix := stripLinkSuffix(link[1:])
		return fmt.Sprintf("<a href='%s'>@%s</a>%s", link, link, suffix)
	})
	line = anchorRe.ReplaceAllStringFunc(line, func(link string) string {
		link, suffix := stripLinkSuffix(link[1:])
		return fmt.Sprintf("<a href='%s'>@%s</a>%s", link, link, suffix)
	})
	line = rawAnchorRe.ReplaceAllStringFunc(line, func(link string) string {
		var prefix string
		if link[0] == ' ' {
			prefix, link = " ", link[1:]
		}
		return fmt.Sprintf("%s<a href='%s'>%s</a>", prefix, link, link)
	})

	w.WriteString(line)
}

// stripLinkSuffix strips sentence-ending chars from the link.
// returns the stripped link and the part that got stripped.
// needed to handle some ugly edge cases that \S+\b doesn't handle well, example:
// "http://a.b" -> &quot;http://a.b&quot; -> the &quot; part comes from the escape, strip that.
// more edge cases in the tests.
func stripLinkSuffix(link string) (string, string) {
	origLink := link
	if strings.HasSuffix(link, "&apos;") {
		link = link[:len(link)-6]
	}
	if strings.HasSuffix(link, "&#39;") {
		link = link[:len(link)-5]
	}
	if strings.HasSuffix(link, "&quot;") {
		link = link[:len(link)-6]
	}
	if strings.HasSuffix(link, "&#34;") {
		link = link[:len(link)-5]
	}
	if strings.HasSuffix(link, ".") {
		link = link[:len(link)-1]
	}
	if strings.HasSuffix(link, ";") {
		link = link[:len(link)-1]
	}
	if strings.HasSuffix(link, ")") {
		link = link[:len(link)-1]
	}
	return link, origLink[len(link):]
}

// identify creates a PascalCased identifier from the passed in header fragment.
func identify(header string) string {
	w := &strings.Builder{}
	for word := range strings.SplitSeq(header, " ") {
		for i, c := range word {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
				continue
			}
			if i == 0 {
				w.WriteString(string(unicode.ToUpper(c)))
			} else {
				w.WriteString(string(c))
			}
		}
		if strings.Contains(word, ":") {
			break
		}
	}
	return w.String()
}
