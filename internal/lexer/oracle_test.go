package lexer_test

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phonologus/icontools/internal/corpus"
	"github.com/phonologus/icontools/internal/lexer"
	"github.com/phonologus/icontools/internal/oracle"
	"github.com/phonologus/icontools/internal/token"
)

// TestOracle compares our significant token stream with icont's own lexer
// on every corpus file. Files needing the preprocessor are compared on
// their "icont -E" expansion; where that contains #line comments, line
// numbers are not compared, as icont reports remapped lines.
func TestOracle(t *testing.T) {
	o := oracle.New(t)
	var compared, preproc, rejected int
	for _, f := range corpus.IconFiles(t) {
		rel, _ := filepath.Rel(o.Root, f)
		src, ok := o.Prepare(t, f)
		if !ok {
			rejected++
			continue
		}
		if src.Expanded {
			preproc++
		}
		toks, errs := lexer.Tokenize(src.Text)
		out, icontOK := o.Tokens(src)
		switch {
		case !icontOK && len(errs) > 0:
			rejected++
			continue
		case !icontOK:
			t.Errorf("%s: icont reports lexical errors, we report none", rel)
			continue
		case len(errs) > 0:
			t.Errorf("%s: we report %q, icont accepts the file", rel, errs[0].Msg)
			continue
		}
		want := parseOracle(out)
		got := ours(toks, lexer.NewLines(src.Text))
		if src.LineDirs {
			dropLines(want)
			dropLines(got)
		}
		if d := firstDiff(got, want); d >= 0 {
			t.Errorf("%s: token %d differs\n got %v\nwant %v", rel, d, at(got, d), at(want, d))
		}
		compared++
	}
	t.Logf("compared %d files (%d after preprocessing); %d rejected by icont", compared, preproc, rejected)
}

type otok struct {
	spell string
	line  int // 0 = don't compare
	image string
}

func (o otok) String() string { return fmt.Sprintf("%q line %d %q", o.spell, o.line, o.image) }

func parseOracle(out []byte) []otok {
	var ts []otok
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		var o otok
		o.spell = f[0]
		fmt.Sscan(f[1], &o.line)
		if len(f) > 2 {
			o.image = f[2]
		}
		if o.spell == ";" || o.spell == "end-of-file" {
			o.line = 0 // icont's positions for these are idiosyncratic
		}
		ts = append(ts, o)
	}
	return ts
}

func ours(toks []lexer.Token, ls *lexer.Lines) []otok {
	var ts []otok
	for _, t := range toks {
		o := otok{spell: t.Kind.String()}
		switch t.Kind {
		case token.SEMICOL, token.EOF:
		default:
			o.line, _ = ls.Position(t.Offset)
		}
		switch t.Kind {
		case token.IDENT, token.INTLIT, token.REALLIT:
			o.image = t.Text
		}
		ts = append(ts, o)
	}
	return ts
}

func firstDiff(a, b []otok) int {
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

func at(ts []otok, i int) any {
	if i < len(ts) {
		return ts[i]
	}
	return "<none>"
}

func dropLines(ts []otok) {
	for i := range ts {
		ts[i].line = 0
	}
}
