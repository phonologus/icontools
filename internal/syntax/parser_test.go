package syntax_test

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/phonologus/icontools/internal/corpus"
	"github.com/phonologus/icontools/internal/oracle"
	"github.com/phonologus/icontools/internal/syntax"
)

// body parses "procedure p()\n<src>\nend" and renders the body in
// icont's tree form, with leaves shown by text.
func body(t *testing.T, src string) string {
	t.Helper()
	f := syntax.Parse("procedure p()\n" + src + "\nend\n")
	for _, e := range f.Errors {
		t.Errorf("%q: error: %s", src, e.Msg)
	}
	d := f.Decls[0].(*syntax.ProcDecl)
	s := icontTree(d, nil)
	// Strip "(Proc (Id p) () (Slist " and ") ()) end)".
	s = strings.TrimPrefix(s, "(Proc (Id p) () (Slist ")
	s = strings.TrimSuffix(s, " ()) end)")
	return s
}

func TestPrecedence(t *testing.T) {
	tests := []struct{ src, want string }{
		{"a + b * c", "(Binop + (Id a) (Binop * (Id b) (Id c)))"},
		{"a - b - c", "(Binop - (Binop - (Id a) (Id b)) (Id c))"},
		{"a ^ b ^ c", "(Binop ^ (Id a) (Binop ^ (Id b) (Id c)))"},
		{"a := b := c", "(Binop := (Id a) (Binop := (Id b) (Id c)))"},
		{"a +:= b", "(Augop +:= (Id a) (Id b))"},
		{"a & b ? c", "(Conj & (Id a) (Scan ? (Id b) (Id c)))"},
		{"a | b | c", "(Alt (Id a) (Alt (Id b) (Id c)))"},
		{"1 to 10 by 2", "(ToBy (Int 1) (Int 10) (Int 2))"},
		{"x := 1 to 2 | 3", "(Binop := (Id x) (To (Int 1) (Alt (Int 2) (Int 3))))"},
		{"-a.b", "(Unop - (Field (Id a) (Id b)))"},
		{"not a = b", "(Binop = (Not (Id a)) (Id b))"},
		{"a < b || c", "(Binop < (Id a) (Binop || (Id b) (Id c)))"},
		{"||x", "(Bar (Id x))"},
		{"f ! L", "(Apply (Id f) (Id L))"},
		{"e \\ 3", "(Limit (Id e) (Int 3))"},
		{"x @ c", "(Activat @ (Id c) (Id x))"},
		{"@c", "(Activat @ (Id c) ())"},
		{"&subject ? move(1)", "(Scan ? (Key subject) (Invok (Id move) (Int 1)))"},
		{"&fail", "(Key fail)"},
		// Control structures extend as far right as possible.
		{"if a then b else c & d", "(If (Id a) (Id b) (Conj & (Id c) (Id d)))"},
		{"x := if a then b else c + 1", "(Binop := (Id x) (If (Id a) (Id b) (Binop + (Id c) (Int 1))))"},
		{"every i := 1 to 3 do write(i)", "(Loop every (Binop := (Id i) (To (Int 1) (Int 3))) (Invok (Id write) (Id i)))"},
		{"while a", "(Loop while (Id a) ())"},
		{"repeat x", "(Loop repeat (Id x) ())"},
		{"return a | b", "(Ret return (Alt (Id a) (Id b)))"},
		{"return", "(Ret return ())"},
		{"fail", "(Ret fail ())"},
		{"suspend a do b", "(Loop suspend (Id a) (Id b))"},
		{"break next", "(Break (Next))"},
		{"create a", "(Create (Id a))"},
		// Postfix forms.
		{"a[i, j]", "(Binop [ (Binop [ (Id a) (Id i)) (Id j))"},
		{"a[i:j]", "(Sect : (Id a) (Id i) (Id j))"},
		{"a[i+:2]", "(Sect +: (Id a) (Id i) (Int 2))"},
		{"f()", "(Invok (Id f) ())"},
		{"f(a, , b)", "(Invok (Id f) (Elist (Elist (Id a) ()) (Id b)))"},
		{"f{a, b}", "(Invok (Id f) (List (Elist (Create (Id a)) (Create (Id b)))))"},
		{"f{}", "(Invok (Id f) (List ()))"},
		{"(a, b)", "(Invok () (Elist (Id a) (Id b)))"},
		{"(a)", "(Id a)"},
		{"[a, b]", "(List (Elist (Id a) (Id b)))"},
		{"{a; b; c}", "(Slist (Id a) (Slist (Id b) (Id c)))"},
		{"x.y.z", "(Field (Field (Id x) (Id y)) (Id z))"},
		{"$<1$>", "(List (Int 1))"},
		{"case x of {\n1: a\ndefault: b\n}", "(Case (Id x) (Clist (Ccls (Int 1) (Id a)) (Ccls default (Id b))))"},
		// Newlines.
		{"x := a +\n b", "(Binop := (Id x) (Binop + (Id a) (Id b)))"},
	}
	for _, tt := range tests {
		if got := body(t, tt.src); got != tt.want {
			t.Errorf("%q\n got %s\nwant %s", tt.src, got, tt.want)
		}
	}
}

func TestDecls(t *testing.T) {
	src := `link strings, "graphics"
global a, b
record point(x, y)
invocable all, "+":2
procedure main(args[])
   local i, j
   static k
   initial k := 0
   write(i)
end
`
	f := syntax.Parse(src)
	for _, e := range f.Errors {
		t.Errorf("error at %d: %s", e.Offset, e.Msg)
	}
	var kinds []string
	for _, d := range f.Decls {
		kinds = append(kinds, fmt.Sprintf("%T", d))
	}
	want := "*syntax.LinkDecl *syntax.GlobalDecl *syntax.RecordDecl *syntax.InvocableDecl *syntax.ProcDecl"
	if got := strings.Join(kinds, " "); got != want {
		t.Errorf("decls: %s", got)
	}
	p := f.Decls[4].(*syntax.ProcDecl)
	if p.Lbrack == nil || len(p.Locals) != 2 || p.Initial == nil || len(p.Body.Elems) != 1 {
		t.Errorf("procedure parsed wrongly: %+v", p)
	}
	checkLossless(t, "decls", f)
}

// TestErrors checks that malformed input yields errors, a lossless tree,
// and no panics or hangs.
func TestErrors(t *testing.T) {
	tests := []string{
		"procedure p()\n x := \nend\n",
		"procedure p()\n f(a, b\nend\n",
		"procedure p()\n x := 1 )\n y := 2\nend\n",
		"procedure p()\n }\nend\n",
		"procedure p()\n if x then\nend\n",
		"procedure p()\n return 1 end\n",
		"procedure p()\n x := 1\n",
		"procedure p()\n x := 1\nprocedure q()\nend\n",
		"x := 1\nprocedure q()\nend\n",
		"procedure p()\n case x of { 1: a; }\nend\n",
		"procedure p(\nend\n",
		"record r(a,\nprocedure q()\nend\n",
		"procedure p()\n x := `\nend\n",
		"procedure p()\n a[1:]\nend\n",
		"procedure p()\n {a; b\nend\n",
		"procedure p()\n & \nend\n",
		"global\n",
		"end\n",
		"",
	}
	for _, src := range tests {
		f := syntax.Parse(src)
		if len(f.Errors) == 0 && src != "" {
			t.Errorf("%q: no errors reported", src)
		}
		checkLossless(t, src, f)
	}
}

// checkLossless verifies that the tree contains every token exactly once,
// in source order.
func checkLossless(t *testing.T, name string, f *syntax.File) {
	t.Helper()
	got := syntax.Tokens(f)
	if len(got) != len(f.Tokens) {
		t.Errorf("%s: tree has %d tokens, source has %d", name, len(got), len(f.Tokens))
		return
	}
	for i := range got {
		if got[i] != &f.Tokens[i] {
			t.Errorf("%s: tree token %d is %q, want %q", name, i, got[i].Text, f.Tokens[i].Text)
			return
		}
	}
}

func TestLosslessCorpus(t *testing.T) {
	files := corpus.IconFiles(t)
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		checkLossless(t, file, syntax.Parse(string(src)))
	}
}

// icontPos formats a token's position as icont does: line:column, with
// 1-based columns and tabs expanded to multiples of 8.
func icontPos(src string, lines []int) func(*syntax.Token) string {
	return func(t *syntax.Token) string {
		ln := sortSearch(lines, t.Offset)
		col := 0
		for _, c := range []byte(src[lines[ln]:t.Offset]) {
			switch c {
			case '\t':
				col = (col | 7) + 1
			case '\b':
				if col > 0 {
					col--
				}
			default:
				col++
			}
		}
		return fmt.Sprintf("%d:%d", ln+1, col+1)
	}
}

func lineStarts(src string) []int {
	ls := []int{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			ls = append(ls, i+1)
		}
	}
	return ls
}

func sortSearch(lines []int, off int) int {
	lo, hi := 0, len(lines)
	for hi-lo > 1 {
		m := (lo + hi) / 2
		if lines[m] <= off {
			lo = m
		} else {
			hi = m
		}
	}
	return lo
}

// TestOracle compares, for every corpus file, whether icont and we accept
// it, and if so the tree of every procedure.
func TestOracle(t *testing.T) {
	o := oracle.New(t)
	var compared, procs, rejected, semantic, prepFail int
	for _, file := range corpus.IconFiles(t) {
		rel, _ := filepath.Rel(o.Root, file)
		src, ok := o.Prepare(t, file)
		if !ok {
			prepFail++
			continue
		}
		f := syntax.Parse(src.Text)
		out, msgs, icontOK := o.Trees(src)
		switch {
		case !icontOK && len(f.Errors) > 0:
			rejected++
			continue
		case !icontOK && allSemantic(msgs):
			semantic++
			continue
		case !icontOK:
			t.Errorf("%s: icont reports %q, we report nothing", rel, msgs[0])
			continue
		case len(f.Errors) > 0:
			e := f.Errors[0]
			line := strings.Count(src.Text[:e.Offset], "\n") + 1
			t.Errorf("%s:%d: we report %q, icont accepts the file", rel, line, e.Msg)
			continue
		}
		var want []string
		sc := bufio.NewScanner(bytes.NewReader(out))
		sc.Buffer(nil, 64<<20)
		for sc.Scan() {
			want = append(want, sc.Text())
		}
		pos := icontPos(src.Text, lineStarts(src.Text))
		var got []string
		for _, d := range f.Decls {
			if p, ok := d.(*syntax.ProcDecl); ok {
				got = append(got, icontTree(p, pos))
			}
		}
		if src.LineDirs {
			for i := range want {
				want[i] = stripPositions(want[i])
			}
			for i := range got {
				got[i] = stripPositions(got[i])
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s: %d procedures, icont has %d", rel, len(got), len(want))
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: procedure %d differs\n%s", rel, i, diffAt(got[i], want[i]))
				break
			}
		}
		compared++
		procs += len(got)
	}
	t.Logf("compared %d files (%d procedures); %d rejected by both; %d with only non-syntax errors; %d not preprocessable",
		compared, procs, rejected, semantic, prepFail)
}

// semanticErrors are icont diagnostics that do not come from the grammar;
// the parser does not detect them.
var semanticErrors = []string{
	"inconsistent redeclaration",
	"redeclared identifier",
	"invalid keyword",
}

func allSemantic(msgs []string) bool {
	if len(msgs) == 0 {
		return false
	}
	for _, m := range msgs {
		found := false
		for _, s := range semanticErrors {
			if strings.Contains(m, s) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func stripPositions(s string) string {
	var b strings.Builder
	for _, f := range strings.Fields(s) {
		if i := strings.IndexByte(f, ':'); i > 0 && f[0] >= '0' && f[0] <= '9' {
			f = "_" + f[strings.IndexByte(f, ')'):]
			if !strings.Contains(f, ")") {
				f = "_"
			}
		}
		b.WriteString(f + " ")
	}
	return b.String()
}

func diffAt(got, want string) string {
	i := 0
	for i < len(got) && i < len(want) && got[i] == want[i] {
		i++
	}
	lo := max(0, i-80)
	return fmt.Sprintf(" got ...%s\nwant ...%s", clip(got[lo:], 200), clip(want[lo:], 200))
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// TestMutations parses randomly damaged corpus files (a token deleted,
// duplicated or swapped with its neighbour) and checks that icont and we
// agree on whether each is syntactically valid, and on the trees when it
// is. Mutants with only non-syntax errors in icont are ignored. Set
// MUTATIONS to change the number of mutants (default 300).
func TestMutations(t *testing.T) {
	o := oracle.New(t)
	n := 300
	if s := os.Getenv("MUTATIONS"); s != "" {
		fmt.Sscan(s, &n)
	}
	files := corpus.IconFiles(t)
	rng := rand.New(rand.NewPCG(1, 2))
	dir := t.TempDir()
	var agreeOK, agreeBad, disagree int
	for i := 0; i < n; i++ {
		file := files[rng.IntN(len(files))]
		src, ok := o.Prepare(t, file)
		if !ok {
			continue
		}
		f := syntax.Parse(src.Text)
		if len(f.Errors) > 0 || len(f.Tokens) < 3 {
			continue
		}
		text, what := mutate(rng, src.Text, f.Tokens)
		m := oracle.Source{File: file, Text: text, Path: filepath.Join(dir, "m.icn"), LineDirs: src.LineDirs}
		if err := os.WriteFile(m.Path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		mf := syntax.Parse(text)
		checkLossless(t, what, mf)
		out, msgs, icontOK := o.Trees(m)
		if !icontOK && allSemantic(msgs) {
			continue
		}
		rel, _ := filepath.Rel(o.Root, file)
		switch {
		case !icontOK && len(mf.Errors) > 0:
			agreeBad++
		case icontOK && len(mf.Errors) == 0:
			agreeOK++
			var got []string
			pos := icontPos(text, lineStarts(text))
			for _, d := range mf.Decls {
				if p, ok := d.(*syntax.ProcDecl); ok {
					got = append(got, icontTree(p, pos))
				}
			}
			want := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
			if len(out) == 0 {
				want = nil
			}
			if m.LineDirs {
				for i := range got {
					got[i] = stripPositions(got[i])
				}
				for i := range want {
					want[i] = stripPositions(want[i])
				}
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				disagree++
				t.Errorf("%s, %s: trees differ", rel, what)
			}
		case !icontOK:
			disagree++
			t.Errorf("%s, %s: icont reports %q, we accept it", rel, what, msgs[0])
		default:
			disagree++
			e := mf.Errors[0]
			t.Errorf("%s, %s: we report %q at %q, icont accepts it", rel, what, e.Msg, near(text, e.Offset))
		}
	}
	t.Logf("%d mutants valid for both, %d invalid for both, %d disagreements", agreeOK, agreeBad, disagree)
}

// mutate applies one random token-level change to src.
func mutate(rng *rand.Rand, src string, toks []syntax.Token) (string, string) {
	// Choose among real (non-implicit, non-EOF) tokens.
	var idx []int
	for i, t := range toks {
		if !t.Implicit && t.Text != "" {
			idx = append(idx, i)
		}
	}
	i := idx[rng.IntN(len(idx))]
	t := toks[i]
	switch rng.IntN(3) {
	case 0:
		return src[:t.Offset] + src[t.End():], fmt.Sprintf("deleted %q at %d", t.Text, t.Offset)
	case 1:
		return src[:t.End()] + " " + t.Text + src[t.End():], fmt.Sprintf("duplicated %q at %d", t.Text, t.Offset)
	default:
		j := slices.Index(idx, i)
		if j+1 >= len(idx) {
			return src[:t.Offset] + src[t.End():], fmt.Sprintf("deleted %q at %d", t.Text, t.Offset)
		}
		u := toks[idx[j+1]]
		mid := src[t.End():u.Offset]
		return src[:t.Offset] + u.Text + mid + t.Text + src[u.End():],
			fmt.Sprintf("swapped %q and %q at %d", t.Text, u.Text, t.Offset)
	}
}

func near(s string, off int) string {
	lo, hi := max(0, off-30), min(len(s), off+30)
	return s[lo:off] + "⟦here⟧" + s[off:hi]
}

// TestErrorMessages checks the first error reported, and where, for some
// common mistakes an editor user will make.
func TestErrorMessages(t *testing.T) {
	tests := []struct {
		src, at, msg string // at: text the error should point to
	}{
		{"procedure p()\n x := \nend\n", "end\n", `expected expression, found "end"`},
		{"procedure p()\n f(a, b\n y := 2\nend\n", "\n y", `expected "," or ")", found newline`},
		{"procedure p()\n x := 1 )\nend\n", ")", `unexpected ")"`},
		{"procedure p()\n return 1 end\n", "end", `expected ";" or newline, found "end"`},
		{"procedure p()\n x := 1\nprocedure q()\nend\n", "procedure q", `missing "end" of procedure`},
		{"procedure p()\n x := 1\n", "", `missing "end" of procedure`},
		{"procedure p()\n {a; b\nend\n", "end", `expected "}", found "end"`},
		{"procedure p()\n x := &\nend\n", "end", `expected keyword name after "&", found "end"`},
	}
	for _, tt := range tests {
		f := syntax.Parse(tt.src)
		if len(f.Errors) == 0 {
			t.Errorf("%q: no error", tt.src)
			continue
		}
		e := f.Errors[0]
		if e.Msg != tt.msg || !strings.HasPrefix(tt.src[e.Offset:], tt.at) {
			t.Errorf("%q: got %q at %q\nwant %q at %q", tt.src, e.Msg, near(tt.src, e.Offset), tt.msg, tt.at)
		}
	}
}
