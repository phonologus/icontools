package format_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phonologus/icontools/internal/corpus"
	"github.com/phonologus/icontools/internal/format"
	"github.com/phonologus/icontools/internal/lexer"
)

func TestGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/*.input")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata")
	}
	for _, in := range files {
		src, err := os.ReadFile(in)
		if err != nil {
			t.Fatal(err)
		}
		got, err := format.Source(src)
		if err != nil {
			t.Errorf("%s: %v", in, err)
			continue
		}
		golden := strings.TrimSuffix(in, ".input") + ".golden"
		if os.Getenv("UPDATE_GOLDEN") != "" {
			os.WriteFile(golden, got, 0o644)
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s: output differs from %s:\n%s", in, golden, got)
		}
		checkSafe(t, in, src, got)
	}
}

// TestCorpus formats every corpus file that parses and checks that the
// result is safe and idempotent. It logs how much of the corpus changes.
func TestCorpus(t *testing.T) {
	var files, formatted, unchanged, lines, changedLines int
	for _, file := range corpus.IconFiles(t) {
		files++
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		got, err := format.Source(src)
		if err != nil {
			continue
		}
		formatted++
		if !checkSafe(t, file, src, got) {
			continue
		}
		again, err := format.Source(got)
		if err != nil {
			t.Errorf("%s: output does not parse: %v", file, err)
			continue
		}
		if string(again) != string(got) {
			t.Errorf("%s: not idempotent:\n%s", file, firstLineDiff(string(got), string(again)))
		}
		if string(got) == string(src) {
			unchanged++
		}
		l, c := countChanged(string(src), string(got))
		lines += l
		changedLines += c
	}
	t.Logf("%d files, %d formatted (%d unchanged); %d of %d lines changed (%.1f%%)",
		files, formatted, unchanged, changedLines, lines, 100*float64(changedLines)/float64(lines))
}

// checkSafe verifies that formatting src as out preserved the significant
// token stream (including implicit semicolons) and every comment and
// directive.
func checkSafe(t *testing.T, name string, src, out []byte) bool {
	t.Helper()
	a, b := sig(string(src)), sig(string(out))
	if d := firstDiff(a, b); d >= 0 {
		t.Errorf("%s: token stream changed at token %d: %s -> %s", name, d, at(a, d), at(b, d))
		return false
	}
	if a, b := vibSection(string(src)), vibSection(string(out)); a != b {
		t.Errorf("%s: VIB section changed", name)
		return false
	}
	ca, cb := comments(string(src)), comments(string(out))
	if d := firstDiff(ca, cb); d >= 0 {
		t.Errorf("%s: comment %d changed: %s -> %s", name, d, at(ca, d), at(cb, d))
		return false
	}
	return true
}

// sig returns the significant tokens, marking implicit semicolons.
func sig(src string) []string {
	toks, _ := lexer.Tokenize(src)
	var out []string
	for _, t := range toks {
		if t.Implicit {
			out = append(out, "⁏")
		} else {
			out = append(out, t.Text)
		}
	}
	return out
}

func comments(src string) []string {
	toks, _ := lexer.Tokenize(src)
	var out []string
	for _, t := range toks {
		for _, tr := range t.Leading {
			if tr.Kind == lexer.Comment || tr.Kind == lexer.Directive {
				out = append(out, strings.TrimRight(tr.Text, " \t\r"))
			}
		}
	}
	return out
}

func firstDiff(a, b []string) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return min(len(a), len(b))
	}
	return -1
}

func at(s []string, i int) string {
	if i < len(s) {
		return fmt.Sprintf("%q", s[i])
	}
	return "<none>"
}

func firstLineDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range min(len(al), len(bl)) {
		if al[i] != bl[i] {
			return fmt.Sprintf("line %d:\n  first:  %q\n  second: %q", i+1, al[i], bl[i])
		}
	}
	return "lengths differ"
}

// countChanged returns the number of lines in a and how many of them do
// not appear unchanged in b (a rough measure, by multiset of lines).
func countChanged(a, b string) (total, changed int) {
	have := map[string]int{}
	for _, l := range strings.Split(b, "\n") {
		have[l]++
	}
	for _, l := range strings.Split(a, "\n") {
		total++
		if have[l] > 0 {
			have[l]--
		} else {
			changed++
		}
	}
	return total, changed
}

// vibSection returns the text from the first VIB begin marker through the
// end of the following end-marker line, or "".
func vibSection(s string) string {
	i := strings.Index(s, "\n#===<<vib:begin>>===")
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i:], "\n#===<<vib:end>>===")
	if j < 0 {
		return ""
	}
	k := strings.IndexByte(s[i+j+1:], '\n')
	if k < 0 {
		return s[i:]
	}
	return s[i : i+j+1+k]
}
