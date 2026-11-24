package lsp

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A small multi-file program: main.icn uses a procedure, record and
// global declared in shapes.icn (same directory, so linked by being in
// the same program).
const mainSrc = `global count

procedure main(args)
   local p, total
   count := 0
   p := point(1, 2)
   total := area(p, 3)
   write(p.x, &subject, total, left("a", 3))
   undeclared := 1
end
`

const shapesSrc = `record point(x, y)

# area computes the area of a square
# of side p.x.
procedure area(p, scale)   #: square area
   count +:= 1
   return p.x * p.x * scale
end
`

type project struct {
	dir   string
	files map[string]string // name -> text
}

func newProject(t *testing.T, files map[string]string) *project {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &project{dir: dir, files: files}
}

func (p *project) uri(name string) string { return pathToURI(filepath.Join(p.dir, name)) }

// pos returns the position of the nth (0-based) occurrence of marker in
// file name, offset by delta bytes. Text is ASCII, so UTF-16 = bytes.
func (p *project) pos(t *testing.T, name, marker string, nth, delta int) TextDocumentPositionParams {
	t.Helper()
	text := p.files[name]
	off := -1
	for i, from := 0, 0; i <= nth; i++ {
		j := strings.Index(text[from:], marker)
		if j < 0 {
			t.Fatalf("%q #%d not in %s", marker, nth, name)
		}
		off = from + j
		from = off + 1
	}
	m := newMapper(text, false)
	return TextDocumentPositionParams{TextDocument: TextDocumentIdentifier{URI: p.uri(name)}, Position: m.position(off + delta)}
}

func (p *project) open(cl *client, name string) PublishDiagnosticsParams {
	cl.notify("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: p.uri(name), LanguageID: "icon", Version: 1, Text: p.files[name]},
	})
	for {
		d := cl.diagnostics()
		if d.URI == p.uri(name) {
			return d
		}
	}
}

func locStrings(p *project, locs []Location) []string {
	var out []string
	for _, l := range locs {
		out = append(out, strings.TrimPrefix(l.URI, p.uri("")+"/")+":"+itoa(l.Range.Start.Line)+":"+itoa(l.Range.Start.Character))
	}
	sort.Strings(out)
	return out
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestFeatures(t *testing.T) {
	p := newProject(t, map[string]string{"main.icn": mainSrc, "shapes.icn": shapesSrc})
	cl := newClient(t)
	cl.initialize()

	// Diagnostics: only the undeclared identifier, as a warning.
	d := p.open(cl, "main.icn")
	if len(d.Diagnostics) != 1 || d.Diagnostics[0].Severity != SeverityWarning ||
		!strings.Contains(d.Diagnostics[0].Message, `"undeclared": undeclared identifier`) {
		t.Errorf("diagnostics: %+v", d.Diagnostics)
	}

	hover := func(pp TextDocumentPositionParams) string {
		t.Helper()
		var h *Hover
		if err := cl.call("textDocument/hover", pp, &h); err != nil {
			t.Fatal(err)
		}
		if h == nil {
			return ""
		}
		return h.Contents.Value
	}
	for _, tc := range []struct {
		marker string
		nth    int
		want   []string
	}{
		{"total", 1, []string{"local total", "Local of procedure `main`."}},
		{"args", 0, []string{"parameter args"}},
		{"area", 0, []string{"procedure area(p, scale)", "area computes the area of a square  \nof side p.x.  \nsquare area", "Declared in `shapes.icn`."}},
		{"point", 0, []string{"record point(x, y)"}},
		{"count", 1, []string{"global count"}},
		{"x,", 0, []string{"Field `x` of", "record point(x, y)"}},
		{"subject", 0, []string{"&subject", "Keyword · [Icon reference](https://www2.cs.arizona.edu/icon/refernce/keywlist.htm)"}},
		{"left", 0, []string{"left(s1,i,s2)", "Pad s1 on right", "funcko.htm#left"}},
		{"undeclared", 0, []string{"Undeclared, so a local variable of procedure `main`."}},
	} {
		got := hover(p.pos(t, "main.icn", tc.marker, tc.nth, 0))
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("hover %s#%d: %q lacks %q", tc.marker, tc.nth, got, w)
			}
		}
	}

	// Definition, across files.
	var locs []Location
	if err := cl.call("textDocument/definition", p.pos(t, "main.icn", "area", 0, 2), &locs); err != nil {
		t.Fatal(err)
	}
	if got := locStrings(p, locs); len(got) != 1 || got[0] != "shapes.icn:4:10" {
		t.Errorf("definition of area: %v", got)
	}
	if err := cl.call("textDocument/definition", p.pos(t, "main.icn", "x,", 0, 0), &locs); err != nil {
		t.Fatal(err)
	}
	if got := locStrings(p, locs); len(got) != 1 || got[0] != "shapes.icn:0:13" {
		t.Errorf("definition of .x: %v", got)
	}

	// References to the global count, in both files.
	refs := func(name, marker string, nth int, decl bool) []string {
		t.Helper()
		var rp ReferenceParams
		rp.TextDocumentPositionParams = p.pos(t, name, marker, nth, 0)
		rp.Context.IncludeDeclaration = decl
		var locs []Location
		if err := cl.call("textDocument/references", rp, &locs); err != nil {
			t.Fatal(err)
		}
		return locStrings(p, locs)
	}
	if got := strings.Join(refs("main.icn", "count", 1, true), " "); got != "main.icn:0:7 main.icn:4:3 shapes.icn:5:3" {
		t.Errorf("references to count: %s", got)
	}
	if got := strings.Join(refs("main.icn", "count", 1, false), " "); got != "main.icn:4:3 shapes.icn:5:3" {
		t.Errorf("references to count without declaration: %s", got)
	}
	if got := strings.Join(refs("main.icn", "p :=", 0, true), " "); got != "main.icn:3:9 main.icn:5:3 main.icn:6:17 main.icn:7:9" {
		t.Errorf("references to local p: %s", got)
	}

	// Completion.
	complete := func(text string, off int) []CompletionItem {
		t.Helper()
		p.files["main.icn"] = text
		cl.notify("textDocument/didChange", DidChangeTextDocumentParams{
			TextDocument:   VersionedTextDocumentIdentifier{URI: p.uri("main.icn"), Version: 2},
			ContentChanges: []TextDocumentContentChangeEvent{{Text: text}},
		})
		var list CompletionList
		pp := TextDocumentPositionParams{TextDocument: TextDocumentIdentifier{URI: p.uri("main.icn")},
			Position: newMapper(text, false).position(off)}
		if err := cl.call("textDocument/completion", pp, &list); err != nil {
			t.Fatal(err)
		}
		return list.Items
	}
	has := func(items []CompletionItem, labels ...string) bool {
		set := map[string]bool{}
		for _, it := range items {
			set[it.Label] = true
		}
		for _, l := range labels {
			if !set[l] {
				return false
			}
		}
		return true
	}
	text := strings.Replace(mainSrc, "   undeclared := 1\n", "   write(&su)\n   q := p.\n   to\n", 1)
	if items := complete(text, strings.Index(text, "&su")+3); !has(items, "subject", "pos") || len(items) < 50 {
		t.Errorf("keyword completion: %d items", len(items))
	}
	if items := complete(text, strings.Index(text, "p.\n")+2); !has(items, "x", "y") || len(items) != 2 {
		t.Errorf("field completion: %v", items)
	}
	items := complete(text, strings.Index(text, "   to\n")+5)
	if !has(items, "total", "args", "count", "area", "point", "write", "find", "every") {
		t.Errorf("identifier completion lacks expected names")
	}
	if items[0].SortText[0] != '0' {
		t.Errorf("locals should sort first: %+v", items[0])
	}

	// Signature help inside area(p, 3), on the second argument.
	text = mainSrc
	complete(text, 0) // restore the text
	var sh *SignatureHelp
	if err := cl.call("textDocument/signatureHelp", p.pos(t, "main.icn", "3)", 0, 0), &sh); err != nil {
		t.Fatal(err)
	}
	if sh == nil || sh.Signatures[0].Label != "area(p, scale)" || sh.ActiveParameter != 1 {
		t.Errorf("signature help: %+v", sh)
	}
	if err := cl.call("textDocument/signatureHelp", p.pos(t, "main.icn", `"a", 3`, 0, 5), &sh); err != nil {
		t.Fatal(err)
	}
	if sh == nil || sh.Signatures[0].Label != "left(s1, i, s2)" || sh.ActiveParameter != 1 {
		t.Errorf("signature help for left: %+v", sh)
	}

	// Rename a global across files.
	var we WorkspaceEdit
	rp := RenameParams{TextDocumentPositionParams: p.pos(t, "main.icn", "count", 1, 0), NewName: "tally"}
	if err := cl.call("textDocument/rename", rp, &we); err != nil {
		t.Fatal(err)
	}
	if len(we.Changes[p.uri("main.icn")]) != 2 || len(we.Changes[p.uri("shapes.icn")]) != 1 {
		t.Errorf("rename edits: %+v", we.Changes)
	}
	// Conflicts are refused.
	rp.NewName = "area"
	if err := cl.call("textDocument/rename", rp, &we); err == nil {
		t.Error("renaming count to area should conflict")
	}
	rp = RenameParams{TextDocumentPositionParams: p.pos(t, "main.icn", "total", 1, 0), NewName: "p"}
	if err := cl.call("textDocument/rename", rp, &we); err == nil {
		t.Error("renaming local total to p should conflict")
	}
	rp.NewName = "write" // the procedure uses the built-in write
	if err := cl.call("textDocument/rename", rp, &we); err == nil {
		t.Error("renaming local total to write should conflict")
	}
	rp.NewName = "1x"
	if err := cl.call("textDocument/rename", rp, &we); err == nil {
		t.Error("renaming to 1x should be refused")
	}

	// Workspace symbols.
	var syms []SymbolInformation
	if err := cl.call("workspace/symbol", WorkspaceSymbolParams{Query: "ar"}, &syms); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range syms {
		names = append(names, s.Name)
	}
	if strings.Join(names, " ") != "area" {
		t.Errorf("workspace symbols: %v", names)
	}
	cl.shutdown()
}

func TestLinks(t *testing.T) {
	p := newProject(t, map[string]string{
		"prog.icn": "link mylib, missing\nprocedure main()\n   helper(nowhere)\nend\n",
	})
	lib := t.TempDir()
	os.WriteFile(filepath.Join(lib, "mylib.icn"), []byte("procedure helper(x)\nend\n"), 0o644)

	cl := newClient(t)
	var ip InitializeParams
	ip.InitializationOptions.IPath = []string{lib}
	var res InitializeResult
	if err := cl.call("initialize", ip, &res); err != nil {
		t.Fatal(err)
	}
	// "missing" cannot be found, so undeclared identifiers are not
	// reported (they might be declared there).
	if d := p.open(cl, "prog.icn"); len(d.Diagnostics) != 0 {
		t.Errorf("diagnostics: %+v", d.Diagnostics)
	}
	var locs []Location
	if err := cl.call("textDocument/definition", p.pos(t, "prog.icn", "helper", 0, 0), &locs); err != nil {
		t.Fatal(err)
	}
	if len(locs) != 1 || locs[0].URI != pathToURI(filepath.Join(lib, "mylib.icn")) {
		t.Errorf("definition of helper: %+v", locs)
	}
	if err := cl.call("textDocument/definition", p.pos(t, "prog.icn", "mylib", 0, 0), &locs); err != nil {
		t.Fatal(err)
	}
	if len(locs) != 1 || locs[0].URI != pathToURI(filepath.Join(lib, "mylib.icn")) {
		t.Errorf("definition of link: %+v", locs)
	}
	// Renaming helper would edit the library: refused.
	var we WorkspaceEdit
	if err := cl.call("textDocument/rename", RenameParams{TextDocumentPositionParams: p.pos(t, "prog.icn", "helper", 0, 0), NewName: "h"}, &we); err == nil {
		t.Error("renaming a library procedure should be refused")
	}
	cl.shutdown()
}

func TestMacros(t *testing.T) {
	p := newProject(t, map[string]string{
		"defs.icn": "$define WIDTH 80   # columns\n",
		"prog.icn": `$include "defs.icn"
$define GREETING "hello"
procedure main()
   write(GREETING, WIDTH, _UNIX)
$undef GREETING
   write(GREETING)
end
`,
	})
	cl := newClient(t)
	cl.initialize()
	d := p.open(cl, "prog.icn")
	// Only the use after $undef is an (undeclared) variable.
	if len(d.Diagnostics) != 1 || d.Diagnostics[0].Range.Start.Line != 5 {
		t.Errorf("diagnostics: %+v", d.Diagnostics)
	}
	hover := func(marker string, nth int) string {
		t.Helper()
		var h *Hover
		if err := cl.call("textDocument/hover", p.pos(t, "prog.icn", marker, nth, 0), &h); err != nil {
			t.Fatal(err)
		}
		if h == nil {
			return ""
		}
		return h.Contents.Value
	}
	if got := hover("GREETING", 1); !strings.Contains(got, `$define GREETING "hello"`) {
		t.Errorf("hover GREETING: %q", got)
	}
	if got := hover("WIDTH", 0); !strings.Contains(got, "$define WIDTH 80") || !strings.Contains(got, "Defined in `defs.icn`") {
		t.Errorf("hover WIDTH: %q", got)
	}
	if got := hover("_UNIX", 0); !strings.Contains(got, `Predefined by the Icon preprocessor when `+"`&features`"+` includes "UNIX"`) {
		t.Errorf("hover _UNIX: %q", got)
	}
	var locs []Location
	if err := cl.call("textDocument/definition", p.pos(t, "prog.icn", "WIDTH", 0, 0), &locs); err != nil {
		t.Fatal(err)
	}
	if got := locStrings(p, locs); len(got) != 1 || got[0] != "defs.icn:0:8" {
		t.Errorf("definition of WIDTH: %v", got)
	}
	cl.shutdown()
}
