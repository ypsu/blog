// package markdown renders raw markdown text into html.
// only a subset of the official markdown is supported.
package markdown

import (
	"fmt"
	"log"
	"regexp"
	"strings"
)

type mode int

var linkRe = regexp.MustCompile(`\bhttps?://[-.a-z0-9]+(/\S*)?\b`)
var gopherRe = regexp.MustCompile(`\bgopher://[-.a-z0-9]+(/\S*)?\b`)
var postRe = regexp.MustCompile(`@(/\S+)\b`)
var anchorRe = regexp.MustCompile(`@(#\S+)\b`)

// Render renders a markdown string into html and returns that.
func Render(input string) string {
	out := &strings.Builder{}
	for _, rawblock := range strings.Split(input, "\n\n") {
		rawblock = strings.TrimLeft(rawblock, "\n")

		// escape html characters.
		safeblock := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#039;").Replace(rawblock)

		// linkify.
		safeblock = linkRe.ReplaceAllString(safeblock, "<a href='$0'>$0</a>")
		safeblock = gopherRe.ReplaceAllString(safeblock, "<a href='https://gopher.floodgap.com/gopher/gw?a=$0'>$0</a>")
		safeblock = anchorRe.ReplaceAllString(safeblock, "<a href='$1'>$0</a>")
		safeblock = postRe.ReplaceAllString(safeblock, "<a href='$1'>$0</a>")

		block := strings.TrimSpace(safeblock)
		if len(block) == 0 {
			continue
		}

		if rawblock[0] == '!' {
			for _, li := range strings.Split(strings.TrimSpace(rawblock), "\n") {
				if len(li) == 0 || li[0] != '!' {
					log.Printf("invalid markdown directive %q, all lines must have !", li)
					continue
				}
				fields := strings.Fields(li)
				if fields[0] == "!html" && len(li) >= 7 {
					out.WriteString(li[6:])
				} else if fields[0] == "!pubdate" {
					fmt.Fprintf(out, "<p><i>published on %s", fields[1])
					if len(fields) >= 3 {
						fmt.Fprintf(out, ", last modified on %s", fields[2])
					}
					out.WriteString("</i></p>")
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
			out.WriteString("<blockquote style='border-left:solid 1px;padding:0 0.5em'>")
			out.WriteString(strings.ReplaceAll(block[4:], "\n&gt;", "\n"))
			out.WriteString("</blockquote>")
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
	return out.String()
}
