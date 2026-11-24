package token

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"icontools/internal/corpus"
)

// entry matches lines of both toktab[] and optab[] in lextab.h, e.g.
//
//	{ "break",           BREAK,         Beginner+Ender },
//	{{"||",     CONCAT,     Beginner}, Binary, NULL, NULL},
var entry = regexp.MustCompile(`\{\s*\{?\s*"((?:[^"\\]|\\.)*)",\s*(\w+),\s*([\w+]+)\s*\}`)

// TestAgainstLextab checks every keyword and operator in the reference
// lextab.h against our table: spelling, kind name and flags.
func TestAgainstLextab(t *testing.T) {
	root := corpus.Require(t)
	src, err := os.ReadFile(filepath.Join(root, "src", "common", "lextab.h"))
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, m := range entry.FindAllStringSubmatch(string(src), -1) {
		spell, cname, cflags := strings.ReplaceAll(m[1], `\\`, `\`), m[2], m[3]
		var want Flags
		if strings.Contains(cflags, "Beginner") {
			want |= Beginner
		}
		if strings.Contains(cflags, "Ender") {
			want |= Ender
		}
		var got Kind
		switch {
		case Digraphs[spell] != 0:
			got = Digraphs[spell]
		case Operators[spell] != 0:
			got = Operators[spell]
		case Lookup(spell) != IDENT:
			got = Lookup(spell)
		default:
			continue // primitives such as "identifier", "end-of-file"
		}
		seen++
		if goName(got) != cname {
			t.Errorf("%q: kind %s, lextab.h says %s", spell, goName(got), cname)
		}
		if got.Flags() != want {
			t.Errorf("%q: flags %b, lextab.h says %s", spell, got.Flags(), cflags)
		}
	}
	if want := (keywordEnd - keywordBeg - 1) + (operatorEnd - operatorBeg - 1) + Kind(len(Digraphs)); seen != int(want) {
		t.Errorf("matched %d lextab.h entries, want %d", seen, want)
	}
}

// goName returns the Go constant name for k (which matches the C token
// name in lextab.h).
func goName(k Kind) string { return kindNames[k] }

var kindNames = func() map[Kind]string {
	src, err := os.ReadFile("token.go")
	if err != nil {
		panic(err)
	}
	m := map[Kind]string{}
	body := string(src)
	body = body[strings.Index(body, "ILLEGAL Kind = iota"):]
	body = body[:strings.Index(body, "\n)")]
	k := Kind(0)
	for line := range strings.SplitSeq(body, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "//") {
			continue
		}
		m[k] = f[0]
		k++
	}
	return m
}()

func TestSemicolonFlags(t *testing.T) {
	// Spot checks of the non-obvious cases.
	for _, k := range []Kind{CONCAT, INTER, SEQ, NMNE, BAR, MINUS} {
		if !k.IsBeginner() {
			t.Errorf("%s should be a Beginner", k)
		}
	}
	for _, k := range []Kind{RETURN, FAIL, SUSPEND, BREAK, NEXT, RPAREN, RBRACK, RBRACE} {
		if !k.IsEnder() {
			t.Errorf("%s should be an Ender", k)
		}
	}
	if ASSIGN.Flags() != 0 || ELSE.Flags() != 0 {
		t.Error(":= and else must have no flags")
	}
}
