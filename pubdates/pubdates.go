// pubdates generates the pubdates cache.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

var postPath = flag.String("postpath", ".", "path to the posts")
var createdRE = regexp.MustCompile(`\n!pubdate ....-..-..\b`)
var tagsRE = regexp.MustCompile(`\n!tags ([^\n]*)\n`)
var titleRE = regexp.MustCompile(`(?:^#|\n!title) (\w+):? ([^\n]*)`)

func run() error {
	flag.Parse()
	dirents, err := os.ReadDir(*postPath)
	if err != nil {
		log.Fatal(err)
	}

	var pubdates []string
	for _, ent := range dirents {
		var pubdate, name, subtitle, tags string
		name = ent.Name()
		contents, err := os.ReadFile(path.Join(*postPath, name))
		if err != nil {
			return err
		}

		pubdate = "9999-01-01"
		if created := createdRE.Find(contents); len(created) != 0 {
			pubdate = string(created[10:])
		}

		if header := titleRE.FindSubmatch(contents); len(header) == 3 {
			if name != string(header[1]) {
				log.Printf("wrong title in %s: %s", name, header[1])
			}
			subtitle = string(header[2])
		}

		if tagDirective := tagsRE.FindSubmatch(contents); len(tagDirective) == 2 {
			tags = string(tagDirective[1])
		}

		pubdates = append(pubdates, fmt.Sprintf("%s %s %q %q", pubdate, name, subtitle, tags))
	}

	sort.Strings(pubdates)
	fmt.Println(`# pubdate filename "title" "tags"`)
	fmt.Println(strings.Join(pubdates, "\n"))
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
