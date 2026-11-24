package lexer

import (
	"fmt"
	"os"
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
