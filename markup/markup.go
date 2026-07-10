// Package markup provides basic markdown-like features for the blog.
package markup

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
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
			w.WriteString("<blockquote><p>\n")
			for i < n && strings.HasPrefix(lines[i], ">") {
				if strings.TrimSpace(lines[i]) == ">" {
					w.WriteString("</p><p>")
				} else {
					writeline(w, strings.TrimPrefix(lines[i], "> "), restricted)
				}
				i++
			}
			w.WriteString("</p></blockquote>\n")

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

		case !restricted && trimmedLine == "!hr":
			w.WriteString("<hr>\n")
			i++

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

// Note that this is coloring only within ``` blocks.
// If you need uncolored pre tag, then use the two-space version.
func writeColoredLine(w *strings.Builder, lineno int, line string) {
	line = html.EscapeString(line)
	if len(line) == utf8.RuneCountInString(line) {
		// Optimization for the common case.
		if lineno > 0 {
			fmt.Fprintf(w, "<span class=cLineNumber>%2d| </span>", lineno)
		}
		w.WriteString(line)
		return
	}

	i, runes := 0, []rune(line)
	n := len(runes)
	if n == 0 {
		return
	}
	filled := false
	if runes[0] == '¹' && strings.Count(line, "¹") == 1 {
		w.WriteString("<div class=cbgNegative>")
		i, filled = 1, true
	}
	if runes[0] == '²' && strings.Count(line, "²") == 1 {
		w.WriteString("<div class=cbgPositive>")
		i, filled = 1, true
	}
	if runes[0] == '³' && strings.Count(line, "³") == 1 {
		w.WriteString("<div class=cbgNotice>")
		i, filled = 1, true
	}
	if lineno > 0 {
		fmt.Fprintf(w, "<span class=cLineNumber>%2d| </span>", lineno)
	}
	for i < n {
		ch := runes[i]
		switch ch {
		case '⁰', '¹', '²', '³':
			if ch == '⁰' {
				w.WriteString("<span class=cfgNeutral>")
			} else if ch == '¹' {
				w.WriteString("<span class=cbgNegative>")
			} else if ch == '²' {
				w.WriteString("<span class=cbgPositive>")
			} else if ch == '³' {
				w.WriteString("<span class=cbgNotice>")
			}
			i++
			for i < n && runes[i] != ch {
				w.WriteRune(runes[i])
				i++
			}
			w.WriteString("</span>")

		default:
			w.WriteRune(ch)
		}
		i++
	}
	if filled {
		w.WriteString("</div>")
	}
}

func writeblock(w *strings.Builder, tag string, lines []string) {
	switch {
	case tag == "numbered":
		w.WriteString("<pre class=cSource>")
		for i, line := range lines {
			writeColoredLine(w, i+1, line)
		}
		w.WriteString("</pre>\n")

	default:
		w.WriteString("<pre class=cSource>")
		if tag != "" {
			fmt.Fprintf(w, "[UnrecognizedTag:%s]\n", html.EscapeString(tag))
		}
		for _, line := range lines {
			writeColoredLine(w, 0, line)
		}
		w.WriteString("</pre>\n")
	}
}

var linkRe = regexp.MustCompile(`\bhttps?://[-.a-z0-9]+(\S*)\b[/;]?`)
var postRe = regexp.MustCompile(`@/\S+\b[/;]?`)
var anchorRe = regexp.MustCompile(`@#\S+\b[/;]?`)
var rawAnchorRe = regexp.MustCompile(`(?m)(?:^| )(#c[0-9-]+|#[A-Z][A-Za-z0-9-]+)`)

func writeline(w *strings.Builder, line string, restricted bool) {
	if !strings.Contains(line, "`") {
		// Optimization for the common case.
		w.WriteString(linkify(line, restricted))
		return
	}

	i, n := 0, len(line)
	for i < n {
		c := line[i]

		switch {
		case c == '`' && (i+1 == n || line[i+1] != '`'):
			j := i + 1
			for j < n && line[j] != '`' {
				j++
			}
			w.WriteString("<code class=cInlineSource>")
			w.WriteString(html.EscapeString(line[i+1 : j]))
			w.WriteString("</code>")
			i = j + 1

		case c == '`':
			open := 2
			for i+open < n && line[i+open] == '`' {
				open++
			}
			ts, te := i+open, i+open
			for te < n && line[te] != '|' {
				te++
			}
			if te == n {
				w.WriteString("[MissingBacktickTagSeparator]\n")
				return
			}
			tag, run := line[ts:te], 0
			i = te + 1
			for i < n && (run < open || line[i] == '`') {
				if line[i] == '`' {
					run++
				} else {
					run = 0
				}
				i++
			}
			if run < open {
				w.WriteString("[MissingCloseBackticks]\n")
				return
			}
			content := line[te+1 : i-open]
			switch {
			case tag == "":
				fmt.Fprintf(w, "<code class=cInlineSource>%s</code>", html.EscapeString(content))
			case tag == "comment":
				// Write nothing, this is secret content.
			case tag == "b":
				fmt.Fprintf(w, "<b>%s</b>", html.EscapeString(content))
			case tag == "em":
				fmt.Fprintf(w, "<em>%s</em>", html.EscapeString(content))
			case !restricted && (strings.HasPrefix(tag, "http://") || strings.HasPrefix(tag, "https://")):
				fmt.Fprintf(w, "<a href='%s'>%s</a>", tag, html.EscapeString(content))
			case !restricted && tag == "raw":
				w.WriteString(content)
			case !restricted && strings.HasPrefix(tag, "."):
				fmt.Fprintf(w, "<span class='%s'>%s</span>", tag[1:], html.EscapeString(content))
			default:
				fmt.Fprintf(w, "[UnrecognizedTag:%s:%s]", html.EscapeString(tag), html.EscapeString(content))
			}

		default:
			j := i + 1
			for j < n && line[j] != '`' {
				j++
			}
			w.WriteString(linkify(line[i:j], restricted))
			i = j
		}
	}
}

func linkify(s string, restricted bool) string {
	s = html.EscapeString(s)

	if restricted {
		// Always linkify comment references.
		return rawAnchorRe.ReplaceAllStringFunc(s, func(link string) string {
			var prefix string
			if link[0] == ' ' {
				prefix, link = " ", link[1:]
			}
			return fmt.Sprintf("%s<a href='%s'>%s</a>", prefix, link, link)
		})
	}

	s = linkRe.ReplaceAllStringFunc(s, func(link string) string {
		link, suffix := stripLinkSuffix(link)
		return fmt.Sprintf("<a href='%s'>%s</a>%s", link, link, suffix)
	})
	s = postRe.ReplaceAllStringFunc(s, func(link string) string {
		link, suffix := stripLinkSuffix(link[1:])
		return fmt.Sprintf("<a href='%s'>@%s</a>%s", link, link, suffix)
	})
	s = anchorRe.ReplaceAllStringFunc(s, func(link string) string {
		link, suffix := stripLinkSuffix(link[1:])
		return fmt.Sprintf("<a href='%s'>@%s</a>%s", link, link, suffix)
	})
	s = rawAnchorRe.ReplaceAllStringFunc(s, func(link string) string {
		var prefix string
		if link[0] == ' ' {
			prefix, link = " ", link[1:]
		}
		return fmt.Sprintf("%s<a href='%s'>%s</a>", prefix, link, link)
	})
	return s
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
			if i == 0 || w.Len() == 0 {
				if w.Len() != 0 || unicode.IsLetter(c) {
					w.WriteRune(unicode.ToUpper(c))
				}
			} else {
				w.WriteRune(c)
			}
		}
		if strings.Contains(word, ":") {
			break
		}
	}
	if w.Len() == 0 {
		return "ExtractIDError"
	}
	return w.String()
}
