// Package sema resolves the identifiers of Icon programs to their
// declarations, following the rules of icont.
//
// A Program is a set of files linked together. Within a procedure, an
// identifier refers to (in order): a parameter, local or static of the
// procedure; a global (declared by global, procedure or record in any
// file of the program); a built-in function; otherwise it is an implicit
// local of the procedure ("undeclared identifier", which icont -u warns
// about). A field name in x.name may belong to any record declaring it.
package sema

import (
	"fmt"
	"sort"

	"github.com/phonologus/icontools/internal/builtin"
	"github.com/phonologus/icontools/internal/syntax"
	"github.com/phonologus/icontools/internal/token"
)

// Kind classifies symbols.
type Kind int

const (
	Param     Kind = iota // procedure parameter
	Local                 // local declaration
	Static                // static declaration
	Implicit              // undeclared, hence local to its procedure
	Global                // global declaration
	Procedure             // procedure declaration
	Record                // record declaration
	Field                 // field of a record declaration
	Builtin               // built-in function
	Macro                 // preprocessor symbol ($define, or predefined)
)

var kindNames = [...]string{"parameter", "local", "static", "undeclared local",
	"global", "procedure", "record", "field", "built-in function",
	"preprocessor symbol"}

func (k Kind) String() string { return kindNames[k] }

// IsLocal reports whether symbols of kind k belong to a procedure.
func (k Kind) IsLocal() bool { return k <= Implicit }

// Symbol is a declared (or implicitly declared) name.
type Symbol struct {
	Name string
	Kind Kind
	File *File         // declaring file (nil for built-ins)
	Decl *syntax.Token // declaring token (for Implicit, the first use)

	// Node is the declaration: *syntax.ProcDecl for procedures and for
	// local kinds (the enclosing procedure), *syntax.RecordDecl for
	// records and fields, *syntax.GlobalDecl for globals.
	Node syntax.Node

	Builtin *builtin.Function // for Builtin
	Macro   *MacroDef         // for Macro
}

// MacroDef is a preprocessor symbol definition.
type MacroDef struct {
	Name, Value string
	Path        string // file holding the $define ("" if predefined)
	Offset, End int    // byte span of the name in that file
	Predefined  *builtin.Predefined
}

// MacroScope is a macro definition in force in part of a file: from byte
// offset From (the $define, or the $include that brought it in) to Until
// (a $undef or redefinition, or the end of the file).
type MacroScope struct {
	Def         *MacroDef
	From, Until int
}

// Proc returns the procedure a local symbol belongs to.
func (s *Symbol) Proc() *syntax.ProcDecl {
	p, _ := s.Node.(*syntax.ProcDecl)
	return p
}

// File is a parsed source file.
type File struct {
	Path   string
	Syntax *syntax.File
	Macros []MacroScope // preprocessor symbols defined in or included by the file
}

// Program is a set of files linked together.
type Program struct {
	Files   []*File
	Globals map[string][]*Symbol // procedures, records and globals, by name
	Fields  map[string][]*Symbol // record fields, by name

	// Complete is false if some linked files could not be found, so that
	// unresolved names may be declared there; undeclared identifiers are
	// then not reported.
	Complete bool
}

// NewProgram collects the global declarations of files.
func NewProgram(files []*File, complete bool) *Program {
	p := &Program{
		Files:    files,
		Globals:  map[string][]*Symbol{},
		Fields:   map[string][]*Symbol{},
		Complete: complete,
	}
	for _, f := range files {
		for _, d := range f.Syntax.Decls {
			switch d := d.(type) {
			case *syntax.GlobalDecl:
				for _, t := range d.Names.Elems {
					p.addGlobal(t, Global, f, d)
				}
			case *syntax.ProcDecl:
				p.addGlobal(d.Name, Procedure, f, d)
			case *syntax.RecordDecl:
				p.addGlobal(d.Name, Record, f, d)
				for _, t := range d.Fields.Elems {
					if t != nil {
						p.Fields[t.Text] = append(p.Fields[t.Text],
							&Symbol{Name: t.Text, Kind: Field, File: f, Decl: t, Node: d})
					}
				}
			}
		}
	}
	return p
}

func (p *Program) addGlobal(t *syntax.Token, k Kind, f *File, n syntax.Node) {
	if t == nil || t.Kind != token.IDENT {
		return
	}
	p.Globals[t.Text] = append(p.Globals[t.Text], &Symbol{Name: t.Text, Kind: k, File: f, Decl: t, Node: n})
}

// Global returns the symbol a global name refers to: preferably a
// procedure or record (whose declarations make the global), else the
// first global declaration, else a built-in function.
func (p *Program) Global(name string) *Symbol {
	syms := p.Globals[name]
	for _, s := range syms {
		if s.Kind != Global {
			return s
		}
	}
	if len(syms) > 0 {
		return syms[0]
	}
	if f, ok := builtin.LookupFunction(name); ok {
		return builtinSymbol(f)
	}
	return nil
}

var builtinSyms = map[string]*Symbol{}

func builtinSymbol(f *builtin.Function) *Symbol {
	s := builtinSyms[f.Name]
	if s == nil {
		s = &Symbol{Name: f.Name, Kind: Builtin, Builtin: f}
		builtinSyms[f.Name] = s
	}
	return s
}

// Severity of a diagnostic.
type Severity int

const (
	Error Severity = iota
	Warning
)

// Diagnostic is a semantic problem in a file.
type Diagnostic struct {
	Offset, End int
	Severity    Severity
	Msg         string
}

// Info is the resolution of one file of a program.
type Info struct {
	File *File

	// Uses maps each identifier token (in expressions, and the names in
	// declarations) to its symbol.
	Uses map[*syntax.Token]*Symbol

	// FieldUses maps the name token of each field reference x.name to
	// the fields it may denote (empty if no record has such a field).
	FieldUses map[*syntax.Token][]*Symbol

	// Keywords maps the name token of each &keyword to the keyword (nil
	// if there is no such keyword).
	Keywords map[*syntax.Token]*builtin.Keyword

	// Locals holds each procedure's local symbols by name.
	Locals map[*syntax.ProcDecl]map[string]*Symbol

	Diagnostics []Diagnostic
}

// Resolve resolves the identifiers of f, which must be one of p's files.
func (p *Program) Resolve(f *File) *Info {
	r := &resolver{p: p, info: &Info{
		File:      f,
		Uses:      map[*syntax.Token]*Symbol{},
		FieldUses: map[*syntax.Token][]*Symbol{},
		Keywords:  map[*syntax.Token]*builtin.Keyword{},
		Locals:    map[*syntax.ProcDecl]map[string]*Symbol{},
	}}
	for _, d := range f.Syntax.Decls {
		r.decl(d)
	}
	sort.SliceStable(r.info.Diagnostics, func(i, j int) bool {
		return r.info.Diagnostics[i].Offset < r.info.Diagnostics[j].Offset
	})
	return r.info
}

type resolver struct {
	p      *Program
	info   *Info
	proc   *syntax.ProcDecl
	locals map[string]*Symbol
	macros map[*MacroDef]*Symbol
}

// macro returns the symbol for a preprocessor symbol in force at t, if
// any: one defined in (or included into) the file, or predefined.
func (r *resolver) macro(t *syntax.Token) *Symbol {
	var def *MacroDef
	for i := range r.info.File.Macros {
		m := &r.info.File.Macros[i]
		if m.Def.Name == t.Text && m.From <= t.Offset && t.Offset < m.Until {
			def = m.Def
		}
	}
	if def == nil {
		p, ok := builtin.LookupPredefined(t.Text)
		if !ok {
			return nil
		}
		def = predefinedDef(p)
	}
	if r.macros == nil {
		r.macros = map[*MacroDef]*Symbol{}
	}
	s := r.macros[def]
	if s == nil {
		s = &Symbol{Name: def.Name, Kind: Macro, Macro: def}
		r.macros[def] = s
	}
	return s
}

var predefinedDefs = map[string]*MacroDef{}

func predefinedDef(p *builtin.Predefined) *MacroDef {
	d := predefinedDefs[p.Name]
	if d == nil {
		d = &MacroDef{Name: p.Name, Value: "1", Predefined: p}
		predefinedDefs[p.Name] = d
	}
	return d
}

func (r *resolver) errorf(t *syntax.Token, sev Severity, format string, args ...any) {
	r.info.Diagnostics = append(r.info.Diagnostics, Diagnostic{
		Offset: t.Offset, End: t.End(), Severity: sev, Msg: fmt.Sprintf(format, args...),
	})
}

func (r *resolver) decl(d syntax.Decl) {
	switch d := d.(type) {
	case *syntax.GlobalDecl:
		for _, t := range d.Names.Elems {
			r.globalDeclName(t, Global)
		}
	case *syntax.RecordDecl:
		r.globalDeclName(d.Name, Record)
		seen := map[string]bool{}
		for _, t := range d.Fields.Elems {
			if t == nil {
				continue
			}
			for _, s := range r.p.Fields[t.Text] {
				if s.Decl == t {
					r.info.Uses[t] = s
				}
			}
			if seen[t.Text] {
				r.errorf(t, Error, "%q: duplicate field name", t.Text)
			}
			seen[t.Text] = true
		}
	case *syntax.InvocableDecl:
		for _, it := range d.Items.Elems {
			if it.Name != nil && it.Name.Kind == token.IDENT {
				if s := r.p.Global(it.Name.Text); s != nil {
					r.info.Uses[it.Name] = s
				}
			}
		}
	case *syntax.ProcDecl:
		r.globalDeclName(d.Name, Procedure)
		r.procDecl(d)
	}
}

// globalDeclName records the symbol declared by t and reports
// inconsistent redeclarations: two procedures or records of the same name,
// in any files of the program, as icont does. A global declaration may
// share its name with anything (it is the procedure's or record's global).
func (r *resolver) globalDeclName(t *syntax.Token, k Kind) {
	if t == nil || t.Kind != token.IDENT {
		return
	}
	syms := r.p.Globals[t.Text]
	var mine *Symbol
	for _, s := range syms {
		if s.Decl == t {
			mine = s
		}
	}
	if mine == nil {
		return
	}
	r.info.Uses[t] = mine
	// Report at each conflicting declaration after the first.
	for _, s := range syms {
		if s == mine || s.Kind == Global || k == Global {
			continue
		}
		if r.p.before(s, mine) {
			r.errorf(t, Error, "%q: inconsistent redeclaration (also a %s in %s)",
				t.Text, s.Kind, s.File.Path)
			return
		}
	}
}

// before reports whether a's declaration precedes b's, in the order of the
// program's files and then of position.
func (p *Program) before(a, b *Symbol) bool {
	if a.File != b.File {
		for _, f := range p.Files {
			if f == a.File {
				return true
			}
			if f == b.File {
				return false
			}
		}
	}
	return a.Decl.Offset < b.Decl.Offset
}

func (r *resolver) procDecl(d *syntax.ProcDecl) {
	r.proc = d
	r.locals = map[string]*Symbol{}
	r.info.Locals[d] = r.locals
	declare := func(t *syntax.Token, k Kind) {
		if t == nil || t.Kind != token.IDENT {
			return
		}
		if _, dup := r.locals[t.Text]; dup {
			r.errorf(t, Error, "%q: redeclared identifier", t.Text)
			r.info.Uses[t] = r.locals[t.Text]
			return
		}
		s := &Symbol{Name: t.Text, Kind: k, File: r.info.File, Decl: t, Node: d}
		r.locals[t.Text] = s
		r.info.Uses[t] = s
	}
	for _, t := range d.Params.Elems {
		declare(t, Param)
	}
	for _, l := range d.Locals {
		k := Local
		if l.Keyword != nil && l.Keyword.Kind == token.STATIC {
			k = Static
		}
		for _, t := range l.Names.Elems {
			declare(t, k)
		}
	}
	if d.Initial != nil {
		r.expr(d.Initial.X)
	}
	for _, x := range d.Body.Elems {
		r.expr(x)
	}
	r.proc, r.locals = nil, nil
}

// ident resolves an identifier used in an expression. A preprocessor
// symbol takes precedence over everything: icont substitutes it before
// parsing.
func (r *resolver) ident(t *syntax.Token) {
	if s := r.macro(t); s != nil {
		r.info.Uses[t] = s
		return
	}
	if s := r.locals[t.Text]; s != nil {
		r.info.Uses[t] = s
		return
	}
	if s := r.p.Global(t.Text); s != nil {
		r.info.Uses[t] = s
		return
	}
	s := &Symbol{Name: t.Text, Kind: Implicit, File: r.info.File, Decl: t, Node: r.proc}
	r.locals[t.Text] = s
	r.info.Uses[t] = s
	if r.p.Complete {
		r.errorf(t, Warning, "%q: undeclared identifier", t.Text)
	}
}

// isAssignOp reports whether k assigns to its left operand.
func isAssignOp(k token.Kind) bool {
	s := k.String()
	return s == ":=:" || s == "<-" || s == "<->" || len(s) >= 2 && s[len(s)-2:] == ":="
}

func (r *resolver) expr(x syntax.Expr) {
	switch x := x.(type) {
	case nil:
	case *syntax.Ident:
		if x.Name != nil {
			r.ident(x.Name)
		}
	case *syntax.Keyword:
		if x.Name == nil {
			return
		}
		k, ok := builtin.LookupKeyword(x.Name.Text)
		if !ok {
			r.errorf(x.Name, Error, "&%s: invalid keyword", x.Name.Text)
		}
		r.info.Keywords[x.Name] = k
	case *syntax.Binary:
		r.expr(x.X)
		r.expr(x.Y)
		// Assigning to a procedure, record constructor or built-in
		// function is legal, but replaces it throughout the program:
		// usually a mistake (e.g. "left := f" for "f.left := f").
		if id, ok := x.X.(*syntax.Ident); ok && id.Name != nil && x.Op != nil && isAssignOp(x.Op.Kind) {
			if s := r.info.Uses[id.Name]; s != nil {
				switch s.Kind {
				case Procedure, Record, Builtin:
					what := map[Kind]string{Procedure: "procedure", Record: "record constructor", Builtin: "built-in function"}[s.Kind]
					r.errorf(id.Name, Warning, "assignment to %s %q replaces it throughout the program", what, s.Name)
				}
			}
		}
	case *syntax.Field:
		r.expr(x.X)
		if x.Name != nil {
			r.info.FieldUses[x.Name] = r.p.Fields[x.Name.Text]
		}
	default:
		// Visit sub-expressions in source order.
		for _, c := range syntax.Children(x) {
			switch c := c.(type) {
			case syntax.Expr:
				r.expr(c)
			case *syntax.CaseClause:
				r.expr(c.Label)
				r.expr(c.Body)
			}
		}
	}
}
