package lsp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/phonologus/icontools/internal/builtin"
	"github.com/phonologus/icontools/internal/lexer"
	"github.com/phonologus/icontools/internal/sema"
	"github.com/phonologus/icontools/internal/syntax"
	"github.com/phonologus/icontools/internal/token"
)

// posCtx is a position in a document, with the document's program and
// name resolution.
type posCtx struct {
	d    *document
	off  int
	prog *sema.Program
	docs map[*sema.File]*document
	info *sema.Info
	tok  *syntax.Token // identifier at the position, or nil
}

func withPosition(s *Server, m *message, f func(*posCtx) (any, error)) (any, error) {
	var p TextDocumentPositionParams
	if err := unmarshal(m.Params, &p); err != nil {
		return nil, err
	}
	c, err := s.at(p)
	if err != nil {
		return nil, err
	}
	return f(c)
}

func (s *Server) at(p TextDocumentPositionParams) (*posCtx, error) {
	d, err := s.doc(p.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	off := d.m.offset(p.Position)
	prog, docs := s.ws.program(d)
	return &posCtx{
		d: d, off: off, prog: prog, docs: docs,
		info: prog.Resolve(d.sema),
		tok:  identAt(d.file, off),
	}, nil
}

// identAt returns the identifier (or keyword name) token touching byte
// offset off: one containing it, or ending at it.
func identAt(f *syntax.File, off int) *syntax.Token {
	toks := f.Tokens
	i := sort.Search(len(toks), func(i int) bool { return toks[i].Offset > off })
	for j := i - 1; j >= 0 && j >= i-2; j-- {
		t := &toks[j]
		if t.Implicit {
			continue
		}
		if (t.Kind == token.IDENT || t.Kind == token.FAIL) && t.Offset <= off && off <= t.End() {
			return t
		}
		if t.End() < off {
			break
		}
	}
	return nil
}

// target is what an identifier denotes.
type target struct {
	sym     *sema.Symbol
	fields  []*sema.Symbol // for a field reference x.name
	isField bool
	kw      *builtin.Keyword
	isKw    bool
	link    string // for a link declaration: the linked file's path
	isLink  bool
}

func (s *Server) target(c *posCtx) target {
	var t target
	if c.tok == nil {
		return t
	}
	if sym := c.info.Uses[c.tok]; sym != nil {
		t.sym = sym
		if sym.Kind == sema.Field {
			t.isField = true
			t.fields = c.prog.Fields[sym.Name]
		}
		return t
	}
	if fs, ok := c.info.FieldUses[c.tok]; ok {
		t.isField, t.fields = true, fs
		return t
	}
	if k, ok := c.info.Keywords[c.tok]; ok {
		t.isKw, t.kw = true, k
		return t
	}
	for _, decl := range c.d.file.Decls {
		if l, ok := decl.(*syntax.LinkDecl); ok {
			for _, ft := range l.Files.Elems {
				if ft == c.tok {
					t.isLink = true
					t.link = s.ws.linkTarget(strings.Trim(ft.Text, `"`), c.d)
				}
			}
		}
	}
	return t
}

func (c *posCtx) tokRange() *Range {
	r := c.d.m.rangeOf(c.tok.Offset, c.tok.End())
	return &r
}

func (c *posCtx) location(f *sema.File, t *syntax.Token) (Location, bool) {
	d := c.docs[f]
	if d == nil || t == nil {
		return Location{}, false
	}
	return Location{URI: d.uri, Range: d.m.rangeOf(t.Offset, t.End())}, true
}

// ---------------------------------------------------------------------
// Hover

func (s *Server) hover(c *posCtx) (any, error) {
	if c.tok == nil {
		return nil, nil
	}
	t := s.target(c)
	var md string
	switch {
	case t.isField:
		md = s.fieldDoc(c.tok.Text, t.fields, c)
	case t.sym != nil:
		md = s.symbolDoc(t.sym, c)
	case t.isKw && t.kw != nil:
		md = s.keywordDoc(t.kw)
	case t.isKw:
		md = fmt.Sprintf("`&%s` is not a keyword", c.tok.Text)
	case t.isLink && t.link != "":
		md = fmt.Sprintf("Links `%s`", t.link)
	case t.isLink:
		md = "Linked file (source not found; set `ipath` or `$IPATH`)"
	default:
		return nil, nil
	}
	return Hover{Contents: MarkupContent{Kind: "markdown", Value: md}, Range: c.tokRange()}, nil
}

func codeBlock(code string) string { return "```icon\n" + code + "\n```" }

func (s *Server) symbolDoc(sym *sema.Symbol, c *posCtx) string {
	var parts []string
	switch sym.Kind {
	case sema.Param, sema.Local, sema.Static, sema.Implicit:
		word := map[sema.Kind]string{sema.Param: "parameter", sema.Local: "local",
			sema.Static: "static", sema.Implicit: "local"}[sym.Kind]
		parts = append(parts, codeBlock(word+" "+sym.Name))
		where := ""
		if p := sym.Proc(); p != nil && p.Name != nil {
			where = fmt.Sprintf(" of procedure `%s`", p.Name.Text)
		}
		if sym.Kind == sema.Implicit {
			parts = append(parts, "Undeclared, so a local variable"+where+".")
		} else {
			parts = append(parts, strings.ToUpper(word[:1])+word[1:]+where+".")
		}
	case sema.Procedure:
		p := sym.Node.(*syntax.ProcDecl)
		parts = append(parts, codeBlock("procedure "+procSignature(p)))
		parts = append(parts, s.declDoc(sym, c)...)
	case sema.Record:
		r := sym.Node.(*syntax.RecordDecl)
		parts = append(parts, codeBlock("record "+sym.Name+tokenList(r.Fields.Elems)))
		parts = append(parts, s.declDoc(sym, c)...)
	case sema.Global:
		parts = append(parts, codeBlock("global "+sym.Name))
		parts = append(parts, s.declDoc(sym, c)...)
	case sema.Builtin:
		return s.builtinDoc(sym.Builtin)
	case sema.Macro:
		m := sym.Macro
		if m.Predefined != nil {
			parts = append(parts, codeBlock("$define "+m.Name+" 1"), "Predefined by the Icon preprocessor")
			if f := m.Predefined.Feature; f != "" {
				parts[1] += fmt.Sprintf(" when `&features` includes %q", f)
			}
			parts[1] += "."
		} else {
			parts = append(parts, codeBlock(strings.TrimSpace("$define "+m.Name+" "+m.Value)))
			if m.Path != c.d.path {
				parts = append(parts, fmt.Sprintf("Defined in `%s`.", relPathTo(c.d, m.Path)))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// declDoc returns a declaration's doc comment and, if it is in another
// file, where.
func (s *Server) declDoc(sym *sema.Symbol, c *posCtx) []string {
	var parts []string
	if d := c.docs[sym.File]; d != nil {
		if doc := docComment(d, sym); doc != "" {
			parts = append(parts, doc)
		}
		if d != c.d {
			parts = append(parts, fmt.Sprintf("Declared in `%s`.", relPath(c.d, d)))
		}
	}
	return parts
}

func relPathTo(from *document, path string) string {
	if from.path != "" && path != "" {
		if r, err := filepath.Rel(filepath.Dir(from.path), path); err == nil {
			return r
		}
	}
	return path
}

func relPath(from, to *document) string {
	if from.path != "" && to.path != "" {
		if r, err := filepath.Rel(filepath.Dir(from.path), to.path); err == nil {
			return r
		}
	}
	return to.uri
}

func (s *Server) fieldDoc(name string, fields []*sema.Symbol, c *posCtx) string {
	if len(fields) == 0 {
		return fmt.Sprintf("No record has a field `%s`.", name)
	}
	var recs []string
	for _, f := range fields {
		r := f.Node.(*syntax.RecordDecl)
		if r.Name != nil {
			recs = append(recs, "record "+r.Name.Text+tokenList(r.Fields.Elems))
		}
	}
	return fmt.Sprintf("Field `%s` of\n\n%s", name, codeBlock(strings.Join(recs, "\n")))
}

func (s *Server) builtinDoc(f *builtin.Function) string {
	var parts []string
	if md := docFunction(s.opts.Docs, f.Name); md != "" {
		parts = append(parts, md)
	} else {
		parts = append(parts, codeBlock(f.Signature), capitalize(f.Summary))
	}
	kind := "Built-in function"
	if f.Graphics {
		kind = "Built-in graphics function"
	}
	parts = append(parts, fmt.Sprintf("%s · [Icon reference](%s)", kind, f.URL()))
	return strings.Join(parts, "\n\n")
}

func (s *Server) keywordDoc(k *builtin.Keyword) string {
	var parts []string
	if md := docKeyword(s.opts.Docs, k.Name); md != "" {
		parts = append(parts, md)
	} else {
		parts = append(parts, codeBlock("&"+k.Name), capitalize(k.Summary))
	}
	parts = append(parts, fmt.Sprintf("Keyword · [Icon reference](%s)", k.URL()))
	return strings.Join(parts, "\n\n")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// docComment returns the comment documenting a declaration: the comment
// lines immediately above it, and a trailing comment on its first line
// (the IPL's "#:" convention).
func docComment(d *document, sym *sema.Symbol) string {
	toks := syntax.Tokens(sym.Node)
	if len(toks) == 0 {
		return ""
	}
	var lines []string
	// Comment lines directly above the declaration.
	lead := toks[0].Leading
	newlines := 0
	for i := len(lead) - 1; i >= 0; i-- {
		tr := lead[i]
		if tr.Kind == lexer.Space {
			continue
		}
		if tr.Kind == lexer.Newline {
			if newlines++; newlines > 1 {
				break
			}
			continue
		}
		if tr.Kind != lexer.Comment {
			break
		}
		newlines = 0
		lines = append([]string{tr.Text}, lines...)
	}
	// A trailing comment on the declaration's first line.
	line := func(off int) int { return d.m.position(off).Line }
	first := line(toks[0].Offset)
	for _, t := range d.file.Tokens[indexOf(d.file, toks[0]):] {
		if t.Implicit || line(t.Offset) == first {
			continue
		}
		for _, tr := range t.Leading {
			if tr.Kind == lexer.Newline {
				break
			}
			if tr.Kind == lexer.Comment && line(tr.Offset) == first {
				lines = append(lines, tr.Text)
			}
		}
		break
	}
	var out []string
	for _, l := range lines {
		l = strings.TrimLeft(l, "#")
		l = strings.TrimPrefix(l, ":")
		if strings.Trim(l, "#=-*_ \t") == "" {
			continue // a rule, not text
		}
		out = append(out, strings.TrimSpace(l))
	}
	return strings.Join(out, "  \n")
}

func indexOf(f *syntax.File, t *syntax.Token) int {
	i := sort.Search(len(f.Tokens), func(i int) bool { return f.Tokens[i].Offset >= t.Offset })
	for ; i < len(f.Tokens); i++ {
		if &f.Tokens[i] == t {
			return i
		}
	}
	return 0
}

func procSignature(p *syntax.ProcDecl) string {
	name := "?"
	if p.Name != nil {
		name = p.Name.Text
	}
	sig := name + tokenList(p.Params.Elems)
	if p.Lbrack != nil {
		sig = strings.TrimSuffix(sig, ")") + "[])"
	}
	return sig
}

func tokenList(ts []*syntax.Token) string {
	var names []string
	for _, t := range ts {
		if t != nil {
			names = append(names, t.Text)
		}
	}
	return "(" + strings.Join(names, ", ") + ")"
}

// docFunction and docKeyword return reference documentation, if loaded.
func docFunction(d *builtin.Docs, name string) string {
	if d == nil {
		return ""
	}
	return d.Functions[name]
}

func docKeyword(d *builtin.Docs, name string) string {
	if d == nil {
		return ""
	}
	return d.Keywords[name]
}

// ---------------------------------------------------------------------
// Definition, references, highlights

func (s *Server) definition(c *posCtx) (any, error) {
	t := s.target(c)
	var locs []Location
	add := func(f *sema.File, tok *syntax.Token) {
		if l, ok := c.location(f, tok); ok {
			locs = append(locs, l)
		}
	}
	switch {
	case t.isField:
		for _, f := range t.fields {
			add(f.File, f.Decl)
		}
	case t.sym != nil && t.sym.Kind == sema.Macro:
		if m := t.sym.Macro; m.Path != "" {
			if d := s.ws.load(m.Path); d != nil {
				locs = append(locs, Location{URI: d.uri, Range: d.m.rangeOf(m.Offset, m.End)})
			}
		}
	case t.sym != nil && t.sym.Kind != sema.Builtin:
		if t.sym.Kind == sema.Global {
			// A global declared in several places: offer them all.
			for _, g := range c.prog.Globals[t.sym.Name] {
				add(g.File, g.Decl)
			}
		} else {
			add(t.sym.File, t.sym.Decl)
		}
	case t.isLink && t.link != "":
		locs = append(locs, Location{URI: pathToURI(t.link)})
	}
	if len(locs) == 0 {
		return nil, nil
	}
	return locs, nil
}

// ref is an occurrence of a symbol.
type ref struct {
	d      *document
	tok    *syntax.Token
	isDecl bool
}

func isGlobalKind(k sema.Kind) bool {
	return k == sema.Global || k == sema.Procedure || k == sema.Record
}

// refs returns the occurrences of what the identifier at c denotes, in
// source order within each file, or nil if it denotes nothing.
func (s *Server) refs(c *posCtx) []ref {
	t := s.target(c)
	var out []ref
	switch {
	case t.sym != nil && t.sym.Kind.IsLocal():
		for tok, sym := range c.info.Uses {
			if sym == t.sym {
				out = append(out, ref{c.d, tok, tok == sym.Decl && sym.Kind != sema.Implicit})
			}
		}
	case t.sym != nil && t.sym.Kind == sema.Macro:
		s.eachFile(c, func(d *document, info *sema.Info) {
			for tok, u := range info.Uses {
				if u.Kind == sema.Macro && u.Macro == t.sym.Macro {
					out = append(out, ref{d, tok, false})
				}
			}
		})
	case t.isField:
		name := c.tok.Text
		s.eachFile(c, func(d *document, info *sema.Info) {
			for tok := range info.FieldUses {
				if tok.Text == name {
					out = append(out, ref{d, tok, false})
				}
			}
			for tok, sym := range info.Uses {
				if sym.Kind == sema.Field && sym.Name == name {
					out = append(out, ref{d, tok, true})
				}
			}
		})
	case t.sym != nil:
		sym := t.sym
		s.eachFile(c, func(d *document, info *sema.Info) {
			for tok, u := range info.Uses {
				same := u == sym || (isGlobalKind(sym.Kind) && isGlobalKind(u.Kind) && u.Name == sym.Name)
				if same {
					out = append(out, ref{d, tok, u.Decl == tok})
				}
			}
		})
	case t.isKw:
		name := c.tok.Text
		s.eachFile(c, func(d *document, info *sema.Info) {
			for tok := range info.Keywords {
				if tok.Text == name {
					out = append(out, ref{d, tok, false})
				}
			}
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].d.uri != out[j].d.uri {
			return out[i].d.uri < out[j].d.uri
		}
		return out[i].tok.Offset < out[j].tok.Offset
	})
	return out
}

func (s *Server) eachFile(c *posCtx, f func(*document, *sema.Info)) {
	for _, sf := range c.prog.Files {
		d := c.docs[sf]
		info := c.info
		if d != c.d {
			info = c.prog.Resolve(sf)
		}
		f(d, info)
	}
}

func (s *Server) references(c *posCtx, includeDecl bool) []Location {
	locs := []Location{}
	if c.tok == nil {
		return locs
	}
	for _, r := range s.refs(c) {
		if r.isDecl && !includeDecl {
			continue
		}
		locs = append(locs, Location{URI: r.d.uri, Range: r.d.m.rangeOf(r.tok.Offset, r.tok.End())})
	}
	return locs
}

func (s *Server) highlight(c *posCtx) (any, error) {
	hs := []DocumentHighlight{}
	if c.tok == nil {
		return hs, nil
	}
	for _, r := range s.refs(c) {
		if r.d == c.d {
			hs = append(hs, DocumentHighlight{Range: c.d.m.rangeOf(r.tok.Offset, r.tok.End()), Kind: 1})
		}
	}
	return hs, nil
}

// ---------------------------------------------------------------------
// Completion

var reservedWords = []string{"break", "by", "case", "create", "default", "do",
	"else", "end", "every", "fail", "global", "if", "initial", "invocable",
	"link", "local", "next", "not", "of", "procedure", "record", "repeat",
	"return", "static", "suspend", "then", "to", "until", "while"}

func (s *Server) completion(c *posCtx) (any, error) {
	start := c.off
	if c.tok != nil && c.tok.Offset < c.off {
		start = c.tok.Offset // completing a partly typed word
	}
	prev, before := prevTokens(c.d.file, start)
	list := CompletionList{Items: []CompletionItem{}}
	switch {
	case prev != nil && prev.Kind == token.AND && prev.End() == start:
		for _, k := range builtin.Keywords() {
			list.Items = append(list.Items, CompletionItem{
				Label: k.Name, Kind: CompletionConstant, Detail: "&" + k.Name,
				Documentation: markdown(capitalize(k.Summary)),
			})
		}
	case prev != nil && prev.Kind == token.DOT && prev.End() == start && before != nil && before.Kind.IsEnder():
		names := make([]string, 0, len(c.prog.Fields))
		for name := range c.prog.Fields {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			var recs []string
			for _, f := range c.prog.Fields[name] {
				if r := f.Node.(*syntax.RecordDecl); r.Name != nil {
					recs = append(recs, r.Name.Text)
				}
			}
			list.Items = append(list.Items, CompletionItem{
				Label: name, Kind: CompletionField, Detail: "field of " + strings.Join(recs, ", "),
			})
		}
	default:
		list.Items = s.identCompletions(c)
	}
	return list, nil
}

// prevTokens returns the last two non-implicit tokens ending at or before
// off.
func prevTokens(f *syntax.File, off int) (prev, before *syntax.Token) {
	toks := f.Tokens
	i := sort.Search(len(toks), func(i int) bool { return toks[i].End() > off || toks[i].Kind == token.EOF })
	for j := i - 1; j >= 0; j-- {
		t := &toks[j]
		if t.Implicit || t.Kind == token.EOF {
			continue
		}
		if prev == nil {
			prev = t
		} else {
			return prev, t
		}
	}
	return prev, nil
}

func markdown(s string) *MarkupContent {
	if s == "" {
		return nil
	}
	return &MarkupContent{Kind: "markdown", Value: s}
}

func (s *Server) identCompletions(c *posCtx) []CompletionItem {
	var items []CompletionItem
	seen := map[string]bool{}
	add := func(it CompletionItem) {
		if !seen[it.Label] {
			seen[it.Label] = true
			items = append(items, it)
		}
	}
	for _, m := range c.d.sema.Macros {
		if m.From <= c.off && c.off < m.Until {
			add(CompletionItem{Label: m.Def.Name, Kind: CompletionConstant,
				Detail: strings.TrimSpace("$define " + m.Def.Name + " " + m.Def.Value), SortText: "0" + m.Def.Name})
		}
	}
	if p := enclosingProc(c.d.file, c.off); p != nil {
		for name, sym := range c.info.Locals[p] {
			if sym.Kind == sema.Implicit && sym.Decl == c.tok {
				continue // the word being typed
			}
			add(CompletionItem{Label: name, Kind: CompletionVariable, Detail: sym.Kind.String(), SortText: "0" + name})
		}
	}
	for name := range c.prog.Globals {
		sym := c.prog.Global(name)
		it := CompletionItem{Label: name, SortText: "1" + name}
		switch sym.Kind {
		case sema.Procedure:
			it.Kind, it.Detail = CompletionFunction, "procedure "+procSignature(sym.Node.(*syntax.ProcDecl))
		case sema.Record:
			it.Kind, it.Detail = CompletionStruct, "record "+name+tokenList(sym.Node.(*syntax.RecordDecl).Fields.Elems)
		default:
			it.Kind, it.Detail = CompletionVariable, "global"
		}
		add(it)
	}
	for _, f := range builtin.Functions() {
		add(CompletionItem{Label: f.Name, Kind: CompletionFunction, Detail: f.Signature,
			Documentation: markdown(capitalize(f.Summary)), SortText: "2" + f.Name})
	}
	for _, w := range reservedWords {
		add(CompletionItem{Label: w, Kind: CompletionKeyword, SortText: "3" + w})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SortText < items[j].SortText })
	return items
}

func enclosingProc(f *syntax.File, off int) *syntax.ProcDecl {
	for _, d := range f.Decls {
		if p, ok := d.(*syntax.ProcDecl); ok {
			if a, b, ok := syntax.Span(p); ok && a <= off && (off <= b || p.End == nil) {
				return p
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// Signature help

func (s *Server) signatureHelp(c *posCtx) (any, error) {
	call := innermostCall(c.d.file, c.off)
	if call == nil {
		return nil, nil
	}
	id, ok := call.Fun.(*syntax.Ident)
	if !ok {
		return nil, nil
	}
	sym := c.info.Uses[id.Name]
	if sym == nil {
		return nil, nil
	}
	var name string
	var params []string
	var doc string
	switch sym.Kind {
	case sema.Procedure:
		p := sym.Node.(*syntax.ProcDecl)
		name = sym.Name
		for _, t := range p.Params.Elems {
			if t != nil {
				params = append(params, t.Text)
			}
		}
		if p.Lbrack != nil && len(params) > 0 {
			params[len(params)-1] += "[]"
		}
		if d := c.docs[sym.File]; d != nil {
			doc = docComment(d, sym)
		}
	case sema.Record:
		name = sym.Name
		for _, t := range sym.Node.(*syntax.RecordDecl).Fields.Elems {
			if t != nil {
				params = append(params, t.Text)
			}
		}
		doc = "Record constructor"
	case sema.Builtin:
		name = sym.Name
		params = sym.Builtin.Params()
		doc = capitalize(sym.Builtin.Summary)
	default:
		return nil, nil
	}
	active := 0
	for _, sep := range call.Args.Seps {
		if sep != nil && sep.Offset < c.off {
			active++
		}
	}
	if n := len(params); n > 0 && active >= n {
		active = n - 1 // extra arguments of a variadic function
	}
	info := SignatureInformation{
		Label:         name + "(" + strings.Join(params, ", ") + ")",
		Documentation: markdown(doc),
		Parameters:    []ParameterInformation{},
	}
	for _, p := range params {
		info.Parameters = append(info.Parameters, ParameterInformation{Label: p})
	}
	return SignatureHelp{Signatures: []SignatureInformation{info}, ActiveParameter: active}, nil
}

// innermostCall returns the innermost call whose argument list contains
// off.
func innermostCall(f *syntax.File, off int) *syntax.Call {
	var best *syntax.Call
	var walk func(n syntax.Node)
	walk = func(n syntax.Node) {
		if call, ok := n.(*syntax.Call); ok && call.Lparen != nil &&
			call.Lparen.End() <= off && (call.Rparen == nil || off <= call.Rparen.Offset) {
			best = call
		}
		for _, ch := range syntax.Children(n) {
			if cn, ok := ch.(syntax.Node); ok {
				walk(cn)
			}
		}
	}
	walk(f)
	return best
}

// ---------------------------------------------------------------------
// Rename

func (s *Server) renameable(c *posCtx) (target, error) {
	t := s.target(c)
	if c.tok == nil || t.sym == nil || t.sym.Kind == sema.Builtin || t.sym.Kind == sema.Macro || t.isField {
		return t, &rpcError{codeRequestFailed, "nothing here that can be renamed"}
	}
	// Don't edit files outside the document's directory (libraries).
	dir := filepath.Dir(c.d.path)
	for _, r := range s.refs(c) {
		if r.d.path == "" || filepath.Dir(r.d.path) != dir {
			return t, &rpcError{codeRequestFailed,
				fmt.Sprintf("%q is also used in %s, outside this directory", c.tok.Text, relPath(c.d, r.d))}
		}
	}
	return t, nil
}

func (s *Server) prepareRename(c *posCtx) (any, error) {
	if _, err := s.renameable(c); err != nil {
		return nil, err
	}
	return PrepareRenameResult{Range: *c.tokRange(), Placeholder: c.tok.Text}, nil
}

func (s *Server) rename(c *posCtx, newName string) (any, error) {
	t, err := s.renameable(c)
	if err != nil {
		return nil, err
	}
	if toks, _ := lexer.Tokenize(newName); len(toks) != 2 || toks[0].Kind != token.IDENT || toks[0].Text != newName {
		return nil, &rpcError{codeInvalidParams, fmt.Sprintf("%q is not a valid identifier", newName)}
	}
	sym := t.sym
	if newName == sym.Name {
		return WorkspaceEdit{Changes: map[string][]TextEdit{}}, nil
	}
	refs := s.refs(c)
	if err := s.renameConflict(c, sym, refs, newName); err != nil {
		return nil, err
	}
	edits := map[string][]TextEdit{}
	for _, r := range refs {
		edits[r.d.uri] = append(edits[r.d.uri], TextEdit{
			Range: r.d.m.rangeOf(r.tok.Offset, r.tok.End()), NewText: newName,
		})
	}
	return WorkspaceEdit{Changes: edits}, nil
}

// renameConflict reports an error if renaming sym to newName would change
// the meaning of the program: a clash with an existing declaration, or an
// occurrence that would then resolve to something else (or vice versa).
func (s *Server) renameConflict(c *posCtx, sym *sema.Symbol, refs []ref, newName string) error {
	conflict := func(format string, args ...any) error {
		return &rpcError{codeRequestFailed, fmt.Sprintf(format, args...)}
	}
	if sym.Kind.IsLocal() {
		p := sym.Proc()
		if other := c.info.Locals[p][newName]; other != nil {
			return conflict("%q is already a %s of this procedure", newName, other.Kind)
		}
		// Would capture a use of a global or built-in of that name.
		for _, t := range syntax.Tokens(p) {
			if u := c.info.Uses[t]; u != nil && u.Name == newName {
				return conflict("%q is used in this procedure as a %s", newName, u.Kind)
			}
		}
		return nil
	}
	if g := c.prog.Globals[newName]; len(g) > 0 {
		return conflict("%q is already a %s (%s)", newName, g[0].Kind, relPath(c.d, c.docs[g[0].File]))
	}
	// Each procedure using the global must not have a local of the new
	// name, nor use a built-in of that name.
	var problem error
	s.eachFile(c, func(d *document, info *sema.Info) {
		for _, decl := range d.file.Decls {
			p, ok := decl.(*syntax.ProcDecl)
			if !ok || problem != nil {
				continue
			}
			usesSym, usesNew := false, false
			for _, t := range syntax.Tokens(p) {
				if u := info.Uses[t]; u != nil {
					usesSym = usesSym || u.Name == sym.Name && isGlobalKind(u.Kind)
					usesNew = usesNew || u.Name == newName
				}
			}
			if usesSym && usesNew && p.Name != nil {
				problem = conflict("%q is already used in procedure %s", newName, p.Name.Text)
			}
		}
	})
	if problem == nil {
		if _, ok := builtin.LookupFunction(newName); ok {
			// A new global of this name would hide the built-in wherever
			// it is used.
			s.eachFile(c, func(d *document, info *sema.Info) {
				for _, u := range info.Uses {
					if u.Kind == sema.Builtin && u.Name == newName && problem == nil {
						problem = conflict("%q would hide the built-in function used in %s", newName, relPath(c.d, d))
					}
				}
			})
		}
	}
	return problem
}

// ---------------------------------------------------------------------
// Workspace symbols

// workspaceSymbols searches the procedures, records and globals of the
// programs of the open documents.
func (s *Server) workspaceSymbols(query string) []SymbolInformation {
	q := strings.ToLower(query)
	out := []SymbolInformation{}
	seen := map[string]bool{}
	uris := make([]string, 0, len(s.docs))
	for uri := range s.docs {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		prog, docs := s.ws.program(s.docs[uri])
		for _, f := range prog.Files {
			d := docs[f]
			if seen[d.uri] {
				continue
			}
			seen[d.uri] = true
			for _, sym := range documentSymbols(d) {
				if strings.Contains(strings.ToLower(sym.Name), q) && sym.Kind != SymbolModule {
					out = append(out, SymbolInformation{
						Name: sym.Name, Kind: sym.Kind,
						Location:      Location{URI: d.uri, Range: sym.SelectionRange},
						ContainerName: filepath.Base(d.path),
					})
				}
			}
		}
	}
	return out
}
