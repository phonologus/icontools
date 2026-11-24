package lexer

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phonologus/icontools/internal/corpus"
	"github.com/phonologus/icontools/internal/token"
)

// render shows significant tokens compactly; implicit semicolons are "⁏".
func render(toks []Token) string {
	var b strings.Builder
	for _, t := range toks {
		if t.Kind == token.EOF {
			break
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		switch {
		case t.Implicit:
			b.WriteString("⁏")
		case t.Kind == token.ILLEGAL:
			fmt.Fprintf(&b, "ILLEGAL(%s)", t.Text)
		default:
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

func TestTokens(t *testing.T) {
	tests := []struct{ src, want string }{
		// Semicolon insertion: Ender at end of line, Beginner next.
		{"x\ny", "x ⁏ y"},
		{"x :=\ny", "x := y"},
		{"f(x)\n-y", "f ( x ) ⁏ - y"},
		{"s\n|| t", "s ⁏ || t"}, // binary-only operators are Beginners
		{"s ||\nt", "s || t"},
		{"return\nx", "return ⁏ x"},
		{"x # comment\ny", "x ⁏ y"},
		{"x\n\n\n  y", "x ⁏ y"},
		{"if x\nthen y", "if x then y"}, // then is not a Beginner
		{"x\nelse", "x else"},
		{"x\nend", "x ⁏ end"},
		// Operators: longest match.
		{"a<<=:=b", "a <<=:= b"},
		{"a~===b", "a ~=== b"},
		{"x<-1", "x <- 1"},
		{"x[1+:2]", "x [ 1 +: 2 ]"},
		{"$(x$)", "$( x $)"},
		// Numbers, and '.' as operator or real literal.
		{"1 1.5 .5 1e10 1.5E-3 16rFF 2r101", "1 1.5 .5 1e10 1.5E-3 16rFF 2r101"},
		{"r.f", "r . f"},
		{"3.foo", "3. foo"},
		{"10x", "10 x"},
		// Keywords are & + identifier.
		{"&subject", "& subject"},
		// Strings and csets.
		{`"a\"b" 'xyz'`, `"a\"b" 'xyz'`},
		{`"\^"" x`, `"\^"" x`}, // \^ consumes the following quote
		{"\"abc_  \n   def\"", "\"abc_  \n   def\""},
		{"\"abc\\\ndef\"", "\"abc\\\ndef\""},
		// Directives are trivia; $( is not a directive.
		{"$define X 1\nX", "X"},
		{"  $include \"f\"\nx", "x"},
		{"$(\n$)", "$( $)"},
		// Invalid characters don't disturb semicolon insertion; the
		// semicolon goes immediately before the Beginner.
		{"x\n` y", "x ILLEGAL(`) ⁏ y"},
	}
	for _, tt := range tests {
		toks, _ := Tokenize(tt.src)
		if got := render(toks); got != tt.want {
			t.Errorf("Tokenize(%q)\n got %s\nwant %s", tt.src, got, tt.want)
		}
	}
}

func TestErrors(t *testing.T) {
	tests := []struct{ src, want string }{
		{`"abc`, "unclosed quote"},
		{"\"abc\ndef\"", "unclosed quote"},
		{"1r0", "invalid radix for integer literal"},
		{"2r2", "invalid integer literal"},
		{"8r19", "invalid digit in integer literal"},
		{"1e", "invalid real literal"},
		{"`", "invalid character"},
		{"%:x", "invalid character"},
	}
	for _, tt := range tests {
		_, errs := Tokenize(tt.src)
		if len(errs) == 0 || errs[0].Msg != tt.want {
			t.Errorf("Tokenize(%q): errors %v, want %q", tt.src, errs, tt.want)
		}
	}
}

func reassemble(toks []Token) string {
	var b strings.Builder
	for _, t := range toks {
		for _, tr := range t.Leading {
			b.WriteString(tr.Text)
		}
		b.WriteString(t.Text)
	}
	return b.String()
}

// TestRoundTrip checks losslessness and offset consistency on every .icn
// file in the reference tree.
func TestRoundTrip(t *testing.T) {
	files := corpus.IconFiles(t)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		toks, _ := Tokenize(string(src))
		if got := reassemble(toks); got != string(src) {
			t.Errorf("%s: round trip differs", f)
			continue
		}
		off := 0
		for _, tk := range toks {
			for _, tr := range tk.Leading {
				if tr.Offset != off {
					t.Fatalf("%s: trivia %v at offset %d, want %d", f, tr.Kind, tr.Offset, off)
				}
				off += len(tr.Text)
			}
			if tk.Offset != off {
				t.Fatalf("%s: token %q at offset %d, want %d", f, tk.Text, tk.Offset, off)
			}
			off += len(tk.Text)
		}
	}
	t.Logf("%d files", len(files))
}

// TestOracle compares our significant token stream with icont's own lexer,
// via the icontok tool (tools/icontok). Set ICONTOK to its path to enable.
//
// Files containing preprocessor directives, or using the symbols ipp
// predefines (src/h/features.h), are compared on their expansion by
// "icont -E" instead. In files containing "#line" comments line numbers
// are not compared, as icont reports the remapped lines.
func TestOracle(t *testing.T) {
	tool := os.Getenv("ICONTOK")
	if tool == "" {
		t.Skip("ICONTOK not set; see tools/icontok/build.sh")
	}
	root := corpus.Require(t)
	predef := predefined(t, root)
	// icont itself, built alongside icontok by build.sh.
	icont := filepath.Join(filepath.Dir(tool), "..", "..", "bin", "icont")
	var compared, preproc, bothReject int
	for _, f := range corpus.IconFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, f)
		toks, errs := Tokenize(string(src))
		lexFile := f
		if hasDirective(toks) || usesAny(toks, predef) {
			// Compare on icont's preprocessed text instead.
			expanded, ok := preprocess(root, icont, f)
			if !ok {
				bothReject++
				continue
			}
			src = expanded
			toks, errs = Tokenize(string(src))
			lexFile = filepath.Join(t.TempDir(), "expanded.icn")
			if err := os.WriteFile(lexFile, src, 0o644); err != nil {
				t.Fatal(err)
			}
			preproc++
		}
		cmd := exec.Command(tool, lexFile)
		cmd.Dir = filepath.Dir(f)
		out, err := cmd.Output()
		icontOK := err == nil
		switch {
		case !icontOK && len(errs) > 0:
			bothReject++
			continue
		case !icontOK:
			t.Errorf("%s: icont reports lexical errors, we report none", rel)
			continue
		case len(errs) > 0:
			t.Errorf("%s: we report %q, icont accepts the file", rel, errs[0].Msg)
			continue
		}
		want := parseOracle(out)
		got := ours(toks, NewLines(string(src)))
		if hasLineComment(toks) {
			dropLines(want)
			dropLines(got)
		}
		if d := firstDiff(got, want); d >= 0 {
			t.Errorf("%s: token %d differs\n got %v\nwant %v", rel, d, at(got, d), at(want, d))
		}
		compared++
	}
	t.Logf("compared %d files (%d after preprocessing); %d rejected by icont -E", compared, preproc, bothReject)
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

func ours(toks []Token, ls *Lines) []otok {
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

func hasDirective(toks []Token) bool {
	for _, t := range toks {
		for _, tr := range t.Leading {
			if tr.Kind == Directive {
				return true
			}
		}
	}
	return false
}

func dropLines(ts []otok) {
	for i := range ts {
		ts[i].line = 0
	}
}

func hasLineComment(toks []Token) bool {
	for _, t := range toks {
		for _, tr := range t.Leading {
			if tr.Kind == Comment && strings.HasPrefix(tr.Text, "#line") {
				return true
			}
		}
	}
	return false
}

// predefined returns the symbols ipp defines automatically.
func predefined(t *testing.T, root string) map[string]bool {
	src, err := os.ReadFile(filepath.Join(root, "src", "h", "features.h"))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]bool{}
	for line := range strings.SplitSeq(string(src), "\n") {
		if _, rest, ok := strings.Cut(line, "Feature("); ok {
			if f := strings.Split(rest, `"`); len(f) > 1 {
				m[f[1]] = true
			}
		}
	}
	return m
}

func usesAny(toks []Token, names map[string]bool) bool {
	for _, t := range toks {
		if t.Kind == token.IDENT && names[t.Text] {
			return true
		}
	}
	return false
}

// preprocess returns the output of "icont -E file", with the IPL include
// directories on the search path.
func preprocess(root, icont, file string) ([]byte, bool) {
	cmd := exec.Command(icont, "-E", filepath.Base(file))
	cmd.Dir = filepath.Dir(file)
	cmd.Env = append(os.Environ(), "LPATH="+
		filepath.Join(root, "ipl", "incl")+" "+filepath.Join(root, "ipl", "gincl"))
	out, err := cmd.Output()
	return out, err == nil
}
