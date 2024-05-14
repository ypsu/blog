// The redirgo tool runs the tests from redir.js's default rules.
package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type rule struct {
	pattern     *regexp.Regexp
	prefix      string
	replacement string
}

type ruleset struct {
	m map[string][]rule
}

var errIDNotFound = errors.New("no rule for given id")
var errQueryNotMatched = errors.New("no rule matched the given query")

func newRuleset() *ruleset {
	return &ruleset{m: make(map[string][]rule)}
}

func (rs *ruleset) replace(u string) (string, error) {
	id, query, _ := strings.Cut(u, "/")
	rules, ok := rs.m[id]
	if !ok {
		return "", errIDNotFound
	}
	for _, rule := range rules {
		if !rule.pattern.MatchString(query) {
			continue
		}
		if rule.prefix != "" {
			return rule.prefix + query, nil
		}
		return rule.pattern.ReplaceAllString(query, rule.replacement), nil
	}
	return "", errQueryNotMatched
}

func runtests(cfg string) error {
	lines := strings.Split(cfg, "\n")

	// parse rules.
	rs := newRuleset()
	for i, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if fields[0] != "rule" {
			continue
		}
		if len(fields) != 4 {
			return fmt.Errorf("parse rule line %d: got %d tokens, want 4: %q", i+1, len(fields), line)
		}
		id, pattern, replacement := fields[1], fields[2], fields[3]
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("parse %s regex pattern on line %d: %s", id, i+1, err)
		}
		rule := rule{pattern: re}
		if strings.IndexByte(replacement, '$') == -1 {
			rule.prefix = replacement
		} else {
			rule.replacement = replacement
		}
		rs.m[id] = append(rs.m[id], rule)
	}

	// run the tests.
	failures, total := 0, 0
	for i, line := range lines {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if fields[0] != "test" {
			continue
		}
		if len(fields) != 3 {
			return fmt.Errorf("parse test line %d: got %d tokens, want 3: %q", i+1, len(fields), line)
		}
		total++
		tc, want := fields[1], fields[2]
		got, err := rs.replace(tc)
		if err != nil {
			failures++
			fmt.Printf("redir.js:%d: replace(%q) error: %s\n", i+1, tc, err)
		} else if got != want {
			failures++
			fmt.Printf("redir.js:%d: replace(%q) = %q, want %q\n", i+1, tc, got, want)
		}
	}
	if failures != 0 {
		return fmt.Errorf("%d/%d tests failed", failures, total)
	}
	fmt.Println("all tests passed.")

	return nil
}

func run() error {
	bytes, err := os.ReadFile("redir.js")
	if err != nil {
		return err
	}
	s := string(bytes)

	// extract the first ` quoted string: that's the config to test.
	start := strings.IndexByte(s, '`')
	if start == -1 {
		return errors.New("redir data not found in redir.js")
	}
	end := strings.IndexByte(s[start+1:], '`')
	if end == -1 {
		return errors.New("redir data not closed in redir.js")
	}
	end += start + 1

	return runtests(s[start:end])
}

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
