// package markdown renders raw markdown text into html.
// only a subset of the official markdown is supported.
package markdown

import (
	"fmt"
	"html"
	"log"
	"regexp"
	"strings"
)

type mode int

var linkRe = regexp.MustCompile(`\bhttps?://[-.a-z0-9]+(\S*)?\b[/;]?`)
var postRe = regexp.MustCompile(`@/\S+\b[/;]?`)
var anchorRe = regexp.MustCompile(`@#\S+\b[/;]?`)

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

// Render renders a markdown string into html and returns that.
func Render(input string, restricted bool) string {
	out := &strings.Builder{}
	for _, rawblock := range strings.Split(input, "\n\n") {
		rawblock = strings.TrimLeft(rawblock, "\n")

		// escape html characters.
		safeblock := html.EscapeString(rawblock)

		// linkify.
		if !restricted && !strings.HasPrefix(safeblock, " ") {
			safeblock = linkRe.ReplaceAllStringFunc(safeblock, func(link string) string {
				link, suffix := stripLinkSuffix(link)
				return fmt.Sprintf("<a href='%s'>%s</a>%s", link, link, suffix)
			})
			safeblock = postRe.ReplaceAllStringFunc(safeblock, func(link string) string {
				link, suffix := stripLinkSuffix(link[1:])
				return fmt.Sprintf("<a href='%s'>@%s</a>%s", link, link, suffix)
			})
			safeblock = anchorRe.ReplaceAllStringFunc(safeblock, func(link string) string {
				link, suffix := stripLinkSuffix(link[1:])
				return fmt.Sprintf("<a href='%s'>@%s</a>%s", link, link, suffix)
			})
		}

		block := strings.TrimSpace(safeblock)
		if len(block) == 0 {
			continue
		}

		if !restricted && rawblock[0] == '!' {
			for _, li := range strings.Split(strings.TrimSpace(rawblock), "\n") {
				if len(li) == 0 || li[0] != '!' {
					log.Printf("invalid markdown directive %q, all lines must have !", li)
					continue
				}
				fields := strings.Fields(li)
				if fields[0] == "!html" {
					if len(li) >= 6 {
						out.WriteString(li[6:])
					}
					out.WriteByte('\n')
				} else if fields[0] == "!pubdate" {
					fmt.Fprintf(out, "<p><i>published on %s", fields[1])
					if len(fields) >= 3 {
						fmt.Fprintf(out, ", last modified on %s", fields[2])
					}
					out.WriteString("</i></p>")
				} else if fields[0] == "!tags" {
					// just ignore.
				} else {
					log.Printf("unrecognized markdown directive %q", li)
				}
			}
		} else if safeblock[0] == ' ' {
			out.WriteString("<pre>")
			out.WriteString(safeblock)
			out.WriteString("</pre>")
		} else if block[0] == '-' {
			out.WriteString("<ul><li>")
			for i, li := range strings.Split(block[1:], "\n-") {
				if i != 0 {
					out.WriteString("</li>\n<li>")
				}
				out.WriteString(li)
			}
			out.WriteString("</li></ul>")
		} else if strings.HasPrefix(block, "&gt;") {
			out.WriteString("<blockquote><p>")
			q := strings.ReplaceAll(block[4:], "\n&gt;", "\n")
			q = strings.ReplaceAll(q, "\n\n", "</p>\n\n<p>")
			out.WriteString(q)
			out.WriteString("</p></blockquote>")
		} else if block[0] == '#' {
			out.WriteString("<p style=font-weight:bold>")
			out.WriteString(block)
			out.WriteString("</p>")
		} else {
			out.WriteString("<p>")
			out.WriteString(block)
			out.WriteString("</p>")
		}
		out.WriteString("\n\n")
	}
	return strings.ReplaceAll(out.String(), "</pre>\n\n<pre>", "\n\n")
}
