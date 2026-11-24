package sema_test

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/phonologus/icontools/internal/builtin"
	"github.com/phonologus/icontools/internal/corpus"
	"github.com/phonologus/icontools/internal/oracle"
	"github.com/phonologus/icontools/internal/sema"
	"github.com/phonologus/icontools/internal/syntax"
)

func program(t *testing.T, srcs ...string) (*sema.Program, []*sema.File) {
	t.Helper()
	var files []*sema.File
	for i, src := range srcs {
		f := syntax.Parse(src)
		for _, e := range f.Errors {
			t.Fatalf("file %d: %s", i, e.Msg)
		}
		files = append(files, &sema.File{Path: fmt.Sprintf("f%d.icn", i), Syntax: f})
	}
	return sema.NewProgram(files, true), files
}

// uses renders the resolution of every identifier in f as
// "name@offset->kind@declOffset" (declOffset -1 for built-ins).
func uses(info *sema.Info) string {
	var out []string
	for t, s := range info.Uses {
		d := -1
		if s.Decl != nil {
			d = s.Decl.Offset
		}
		out = append(out, fmt.Sprintf("%s@%d->%s@%d", t.Text, t.Offset, s.Kind, d))
	}
	sort.Slice(out, func(i, j int) bool {
		var a, b int
		fmt.Sscanf(out[i][strings.Index(out[i], "@")+1:], "%d", &a)
		fmt.Sscanf(out[j][strings.Index(out[j], "@")+1:], "%d", &b)
		return a < b
	})
	return strings.Join(out, "\n")
}

func TestResolve(t *testing.T) {
	src := `global g, write
record point(x, y)
procedure main(args)
   local i
   static n
   every i := 1 to 3 do write(i, g, n, z, point(1, 2).x, find("a"))
   z := 1
end
`
	p, files := program(t, src)
	info := p.Resolve(files[0])
	check := func(name string, nth int, kind sema.Kind, declText string) {
		t.Helper()
		var toks []*syntax.Token
		for tk := range info.Uses {
			if tk.Text == name {
				toks = append(toks, tk)
			}
		}
		sort.Slice(toks, func(i, j int) bool { return toks[i].Offset < toks[j].Offset })
		if nth >= len(toks) {
			t.Errorf("%s #%d: not resolved", name, nth)
			return
		}
		s := info.Uses[toks[nth]]
		if s.Kind != kind {
			t.Errorf("%s #%d: kind %s, want %s", name, nth, s.Kind, kind)
		}
		if declText != "" && (s.Decl == nil || src[s.Decl.Offset:] == "" ||
			!strings.HasPrefix(src[s.Decl.Offset-len(declText)+len(name):], declText)) {
			t.Errorf("%s #%d: declared at %v", name, nth, s.Decl)
		}
	}
	check("g", 1, sema.Global, "")
	check("write", 1, sema.Global, "") // the global hides the built-in
	check("i", 1, sema.Local, "")
	check("n", 1, sema.Static, "")
	check("args", 0, sema.Param, "")
	check("z", 0, sema.Implicit, "")
	check("z", 1, sema.Implicit, "")
	check("point", 1, sema.Record, "")
	check("find", 0, sema.Builtin, "")
	if s1, s2 := info.Uses[tokenNamed(info, "z", 0)], info.Uses[tokenNamed(info, "z", 1)]; s1 != s2 {
		t.Error("the two uses of z are different symbols")
	}
	var fieldX []*sema.Symbol
	for tk, fs := range info.FieldUses {
		if tk.Text == "x" {
			fieldX = fs
		}
	}
	if len(fieldX) != 1 || fieldX[0].Kind != sema.Field || fieldX[0].Name != "x" {
		t.Errorf(".x resolves to %v", fieldX)
	}
	// Exactly one warning: z is undeclared.
	if len(info.Diagnostics) != 1 || !strings.Contains(info.Diagnostics[0].Msg, `"z": undeclared`) {
		t.Errorf("diagnostics: %+v", info.Diagnostics)
	}
}

func tokenNamed(info *sema.Info, name string, nth int) *syntax.Token {
	var toks []*syntax.Token
	for tk := range info.Uses {
		if tk.Text == name {
			toks = append(toks, tk)
		}
	}
	sort.Slice(toks, func(i, j int) bool { return toks[i].Offset < toks[j].Offset })
	if nth < len(toks) {
		return toks[nth]
	}
	return nil
}

func TestCrossFile(t *testing.T) {
	p, files := program(t,
		"global count\nprocedure main()\n   count := helper(1)\nend\n",
		"procedure helper(x)\n   return count + x\nend\nrecord helper(a)\n")
	i0, i1 := p.Resolve(files[0]), p.Resolve(files[1])
	if s := i0.Uses[tokenNamed(i0, "helper", 0)]; s == nil || s.Kind != sema.Procedure || s.File != files[1] {
		t.Errorf("helper in file 0: %+v", s)
	}
	if s := i1.Uses[tokenNamed(i1, "count", 0)]; s == nil || s.Kind != sema.Global || s.File != files[0] {
		t.Errorf("count in file 1: %+v", s)
	}
	if len(i1.Diagnostics) != 1 || !strings.Contains(i1.Diagnostics[0].Msg, "inconsistent redeclaration") {
		t.Errorf("diagnostics: %+v", i1.Diagnostics)
	}
}

func TestIncomplete(t *testing.T) {
	f := syntax.Parse("link strings\nprocedure main()\n   write(replace(s, \"a\", \"b\"))\nend\n")
	sf := &sema.File{Path: "a.icn", Syntax: f}
	info := sema.NewProgram([]*sema.File{sf}, false).Resolve(sf)
	if len(info.Diagnostics) != 0 {
		t.Errorf("undeclared reported for incomplete program: %+v", info.Diagnostics)
	}
}

var (
	transMsg = regexp.MustCompile(`^File .*; Line (\d+) # "([^"]*)": (redeclared identifier|inconsistent redeclaration|invalid keyword)`)
	linkMsg  = regexp.MustCompile(`^.*: "([^"]*)": (undeclared identifier|inconsistent redeclaration)(?:, procedure (\w+))?`)
)

// TestOracle compares semantic diagnostics with icont -u on every corpus
// file that stands alone (no link declarations) and parses.
//
// icont's linker discards procedures not reachable from main (or an
// invocable declaration) before reporting undeclared identifiers, so it
// reports them only in reachable procedures; we report them everywhere,
// and compare only those.
func TestOracle(t *testing.T) {
	o := oracle.New(t)
	out := t.TempDir()
	var compared, skipped int
	for _, file := range corpus.IconFiles(t) {
		rel, _ := filepath.Rel(o.Root, file)
		src, ok := o.Prepare(t, file)
		if !ok {
			skipped++
			continue
		}
		f := syntax.Parse(src.Text)
		if len(f.Errors) > 0 || hasLink(f) {
			skipped++
			continue
		}
		sf := &sema.File{Path: file, Syntax: f}
		prog := sema.NewProgram([]*sema.File{sf}, true)
		info := prog.Resolve(sf)
		p := func(*sema.File) *sema.Program { return prog }

		msgs, _ := o.Icont(filepath.Dir(src.Path), "-s", "-u", "-o", filepath.Join(out, "x"), filepath.Base(src.Path))
		var want []string
		translationErrors := false
		for line := range strings.SplitSeq(msgs, "\n") {
			if m := transMsg.FindStringSubmatch(line); m != nil {
				translationErrors = true
				want = append(want, fmt.Sprintf("%s %s: %s", m[1], m[2], m[3]))
			} else if m := linkMsg.FindStringSubmatch(line); m != nil {
				// The oracle's icont is built without graphics, so to it
				// the graphics functions are undeclared identifiers.
				if f, ok := builtin.LookupFunction(m[1]); ok && f.Graphics {
					continue
				}
				// Which feature symbols (_GRAPHICS, _MS_WINDOWS, ...) icont
				// predefines depends on how it was built; we treat them
				// all as possibly defined.
				if _, ok := builtin.LookupPredefined(m[1]); ok {
					continue
				}
				want = append(want, fmt.Sprintf("%s: %s %s", m[1], m[2], m[3]))
			}
		}
		ls := lineStarts(src.Text)
		live := reachable(p(sf), info)
		var got []string
		for _, d := range info.Diagnostics {
			name, kind, _ := strings.Cut(d.Msg, ": ")
			name = strings.Trim(name, `"&`)
			kind, _, _ = strings.Cut(kind, " (")
			if kind == "undeclared identifier" {
				if translationErrors {
					continue // icont does not link a file with errors
				}
				proc := procAt(f, d.Offset)
				if !live[proc] {
					continue
				}
				got = append(got, fmt.Sprintf("%s: %s %s", name, kind, proc))
			} else if kind == "inconsistent redeclaration" || kind == "redeclared identifier" || kind == "invalid keyword" {
				got = append(got, fmt.Sprintf("%d %s: %s", lineOf(ls, d.Offset), name, kind))
			}
		}
		if src.LineDirs {
			got, want = stripLines(got), stripLines(want)
		}
		sort.Strings(got)
		sort.Strings(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s:\n got %q\nwant %q", rel, got, want)
		}
		compared++
	}
	t.Logf("compared %d files; skipped %d (links, errors or preprocessing failures)", compared, skipped)
}

// reachable returns the names of the procedures icont's linker keeps:
// those reachable from main and from invocable declarations, following
// references to procedures; all of them if there is "invocable all".
func reachable(p *sema.Program, info *sema.Info) map[string]bool {
	procs := map[string]*syntax.ProcDecl{}
	var roots []string
	all := false
	for _, d := range info.File.Syntax.Decls {
		switch d := d.(type) {
		case *syntax.ProcDecl:
			if d.Name != nil {
				procs[d.Name.Text] = d
			}
		case *syntax.InvocableDecl:
			for _, it := range d.Items.Elems {
				if it.Name == nil {
					continue
				}
				if it.Name.Text == "all" {
					all = true
				}
				roots = append(roots, strings.Trim(it.Name.Text, `"`))
			}
		}
	}
	live := map[string]bool{}
	if all {
		for name := range procs {
			live[name] = true
		}
		return live
	}
	var visit func(string)
	visit = func(name string) {
		d := procs[name]
		if d == nil || live[name] {
			return
		}
		live[name] = true
		for _, t := range syntax.Tokens(d) {
			if s := info.Uses[t]; s != nil && s.Kind == sema.Procedure {
				visit(s.Name)
			}
		}
	}
	visit("main")
	for _, r := range roots {
		visit(r)
	}
	return live
}

func hasLink(f *syntax.File) bool {
	for _, d := range f.Decls {
		if _, ok := d.(*syntax.LinkDecl); ok {
			return true
		}
	}
	return false
}

func procAt(f *syntax.File, off int) string {
	for _, d := range f.Decls {
		if p, ok := d.(*syntax.ProcDecl); ok {
			if a, b, ok := syntax.Span(p); ok && a <= off && off < b && p.Name != nil {
				return p.Name.Text
			}
		}
	}
	return "?"
}

func lineStarts(s string) []int {
	ls := []int{0}
	for i := range len(s) {
		if s[i] == '\n' {
			ls = append(ls, i+1)
		}
	}
	return ls
}

func lineOf(ls []int, off int) int {
	return sort.Search(len(ls), func(i int) bool { return ls[i] > off })
}

func stripLines(ss []string) []string {
	var out []string
	for _, s := range ss {
		if i := strings.IndexByte(s, ' '); i > 0 && s[0] >= '0' && s[0] <= '9' {
			s = "_" + s[i:]
		}
		out = append(out, s)
	}
	return out
}
