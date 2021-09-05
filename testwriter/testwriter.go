// package testwriter helps generate and validate golden test data.
//
// the test data is in a file called test.data in the test's local directory.
// its structure looks like this:
//
//   input [name1]
//     arbitrary content
//   output [name1]
//     arbitrary content
//   input [name2]
//     arbitrary content
//   output [name2]
//     arbitrary content
//   ...
//
// both input and output is optional in the file.
// missing the input or the output part is represented as an empty string in the maps returned by Data.
// the content is indented by two spaces.
package testwriter

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

var writeFlag = flag.Bool("testwrite", false, "write golden test output to test.data.")
var fileFlag = flag.String("testfile", "test.data", "the golden test file")

func indent(x string) string {
	if len(x) == 0 {
		return x
	}
	return "  " + strings.ReplaceAll(x, "\n", "\n  ")
}

func firstDiff(lhs, rhs string) string {
	lhsLines := append(strings.Split(lhs, "\n"), "<past end>")
	rhsLines := append(strings.Split(rhs, "\n"), "<past end>")
	for i := range lhsLines {
		if lhsLines[i] != rhsLines[i] {
			return fmt.Sprintf("-%s\n+%s", lhsLines[i], rhsLines[i])
		}
	}
	return "???"
}

func validate(t *testing.T, oldin, oldout, newin, newout map[string]string) {
	if !*writeFlag {
		// check that the new data is a superset of the old one.
		for k := range oldin {
			if _, ok := newin[k]; !ok {
				t.Errorf("input %s missing", k)
			}
		}
		for k := range oldout {
			if _, ok := newout[k]; !ok {
				t.Errorf("output %s missing", k)
			}
		}

		// compare old and new data.
		for k, v := range newin {
			if oldin[k] != v {
				t.Errorf("input %s mismatching, first diff:\n%s", k, firstDiff(oldin[k], v))
			}
		}
		for k, v := range newout {
			if oldout[k] != v {
				t.Errorf("output %s mismatching, first diff:\n%s", k, firstDiff(oldout[k], v))
			}
		}
	}

	// sort the keys.
	allKeys := []string{}
	for k := range newin {
		allKeys = append(allKeys, k)
	}
	for k := range newout {
		if _, ok := newin[k]; !ok {
			allKeys = append(allKeys, k)
		}
	}
	sort.Strings(allKeys)

	// pretty print the data.
	out := &bytes.Buffer{}
	for _, k := range allKeys {
		if len(newin[k]) != 0 {
			fmt.Fprintf(out, "input %s\n%s\n", k, indent(newin[k]))
		}
		if len(newout[k]) != 0 {
			fmt.Fprintf(out, "output %s\n%s\n", k, indent(newout[k]))
		}
	}

	// write output if needed.
	if *writeFlag {
		err := os.WriteFile(*fileFlag, out.Bytes(), 0666)
		if err != nil {
			t.Fatal(err)
		}
	} else if t.Failed() {
		t.Log("use --testwrite to commit the changes.")
	}
}

func Data(t *testing.T) (inputs, outputs map[string]string) {
	oldin, oldout := map[string]string{}, map[string]string{}
	newin, newout := map[string]string{}, map[string]string{}

	in, err := os.ReadFile(*fileFlag)
	if os.IsNotExist(err) {
		if !*writeFlag {
			t.Fatal("test.data doesn't exist. wrong dir? use --testwrite to create it.")
		}
	} else if err != nil {
		t.Fatal(err)
	}

	var k, v []byte
	var inputType bool
	for _, line := range bytes.Split(append(in, '\n'), []byte("\n")) {
		if bytes.HasPrefix(line, []byte("  ")) {
			v = append(append(v, line[2:]...), '\n')
			continue
		}
		if len(v) > 0 {
			if len(k) == 0 {
				t.Fatal("missing test.data header")
			}
			v = v[0 : len(v)-1] // eat the last newline because it's superfluous.
			if inputType {
				oldin[string(k)] = string(v)
				newin[string(k)] = string(v)
			} else {
				oldout[string(k)] = string(v)
				newout[string(k)] = string(v)
			}
			k = nil
			v = nil
		}
		if len(line) == 0 {
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid test.data line %q", line)
		}
		if bytes.Compare(fields[0], []byte("input")) == 0 {
			inputType = true
		} else if bytes.Compare(fields[0], []byte("output")) == 0 {
			inputType = false
		} else {
			t.Fatalf("invalid test.data line %q", line)
		}
		k = fields[1]
	}

	t.Cleanup(func() {
		validate(t, oldin, oldout, newin, newout)
	})
	return newin, newout
}
