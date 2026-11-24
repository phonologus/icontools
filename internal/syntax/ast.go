package syntax

import (
	"reflect"

	"github.com/phonologus/icontools/internal/lexer"
)

// Token is a lexical token with its leading trivia.
type Token = lexer.Token

// Node is any syntax tree node. Every token of the source belongs to
// exactly one node; see Children and Tokens.
type Node interface {
	// children calls f for each child, a Node or a *Token, in source
	// order. Absent (nil) children are skipped.
	children(f func(any))
}

// Children returns the child nodes and tokens of n in source order.
func Children(n Node) []any {
	var out []any
	n.children(func(c any) { out = append(out, c) })
	return out
}

// Tokens returns every token under n, in source order.
func Tokens(n Node) []*Token {
	var out []*Token
	var walk func(Node)
	walk = func(n Node) {
		n.children(func(c any) {
			switch c := c.(type) {
			case *Token:
				out = append(out, c)
			case Node:
				walk(c)
			}
		})
	}
	walk(n)
	return out
}

// Span returns the byte range covered by n's tokens (excluding the first
// token's leading trivia). ok is false if n has no tokens.
func Span(n Node) (start, end int, ok bool) {
	toks := Tokens(n)
	if len(toks) == 0 {
		return 0, 0, false
	}
	return toks[0].Offset, toks[len(toks)-1].End(), true
}

// emit calls f(c) for each c that is neither nil nor a nil pointer.
func emit(f func(any), cs ...any) {
	for _, c := range cs {
		if c == nil {
			continue
		}
		if v := reflect.ValueOf(c); v.Kind() == reflect.Pointer && v.IsNil() {
			continue
		}
		f(c)
	}
}

// List is a sequence of elements with separators (commas or semicolons).
// Seps[i] follows Elems[i]; there is either one separator fewer than
// elements or, for semicolon-terminated sequences, the same number.
//
// Elements may be nil where the grammar allows an empty expression, as in
// f(a,,b). A list consisting of a single empty element and no separators,
// such as the arguments of f(), has no elements at all.
type List[T any] struct {
	Elems []T
	Seps  []*Token
}

func (l *List[T]) each(f func(any)) {
	for i, e := range l.Elems {
		emit(f, any(e))
		if i < len(l.Seps) {
			emit(f, l.Seps[i])
		}
	}
}

// ---------------------------------------------------------------------
// File and declarations

// File is a parsed source file.
type File struct {
	Src    string
	Tokens []Token // all tokens, including the final EOF
	Decls  []Decl
	EOF    *Token // carries trailing trivia
	Errors []Error
}

func (n *File) children(f func(any)) {
	for _, d := range n.Decls {
		f(d)
	}
	emit(f, n.EOF)
}

// Error is a lexical or syntax error.
type Error struct {
	Offset int
	Msg    string
}

func (e Error) Error() string { return e.Msg }

// Decl is a top-level declaration.
type Decl interface {
	Node
	decl()
}

// LinkDecl: link name, "file", ...
type LinkDecl struct {
	Link  *Token
	Files List[*Token] // IDENT or STRINGLIT
}

// GlobalDecl: global a, b, ...
type GlobalDecl struct {
	Global *Token
	Names  List[*Token]
}

// RecordDecl: record name(field, ...)
type RecordDecl struct {
	Record, Name, Lparen *Token
	Fields               List[*Token]
	Rparen               *Token
}

// InvocableDecl: invocable name, "op":arity, ...
type InvocableDecl struct {
	Invocable *Token
	Items     List[*InvocableItem]
}

// InvocableItem: name | "string" | "string":arity
type InvocableItem struct {
	Name         *Token // IDENT or STRINGLIT
	Colon, Arity *Token // optional
}

// ProcDecl is a procedure declaration.
//
//	procedure name(params[])
//	   local ...; static ...
//	   initial expr
//	   body
//	end
type ProcDecl struct {
	Procedure, Name, Lparen *Token
	Params                  List[*Token]
	Lbrack, Rbrack          *Token // "[]" marking a variadic final parameter
	Rparen                  *Token
	Semi                    *Token // usually implicit
	Locals                  []*LocalDecl
	Initial                 *InitialClause
	Body                    List[Expr] // Seps are semicolons, one per element
	End                     *Token
}

// LocalDecl: local a, b;  or  static a, b;
type LocalDecl struct {
	Keyword *Token // LOCAL or STATIC
	Names   List[*Token]
	Semi    *Token
}

// InitialClause: initial expr;
type InitialClause struct {
	Initial *Token
	X       Expr
	Semi    *Token
}

// BadDecl holds tokens that could not be parsed as a declaration.
type BadDecl struct {
	Elems []any // *Token or Node
}

func (*LinkDecl) decl()      {}
func (*GlobalDecl) decl()    {}
func (*RecordDecl) decl()    {}
func (*InvocableDecl) decl() {}
func (*ProcDecl) decl()      {}
func (*BadDecl) decl()       {}

func (n *LinkDecl) children(f func(any)) { emit(f, n.Link); n.Files.each(f) }
func (n *GlobalDecl) children(f func(any)) {
	emit(f, n.Global)
	n.Names.each(f)
}
func (n *RecordDecl) children(f func(any)) {
	emit(f, n.Record, n.Name, n.Lparen)
	n.Fields.each(f)
	emit(f, n.Rparen)
}
func (n *InvocableDecl) children(f func(any)) {
	emit(f, n.Invocable)
	n.Items.each(f)
}
func (n *InvocableItem) children(f func(any)) { emit(f, n.Name, n.Colon, n.Arity) }
func (n *ProcDecl) children(f func(any)) {
	emit(f, n.Procedure, n.Name, n.Lparen)
	n.Params.each(f)
	emit(f, n.Lbrack, n.Rbrack, n.Rparen, n.Semi)
	for _, l := range n.Locals {
		f(l)
	}
	if n.Initial != nil {
		f(n.Initial)
	}
	n.Body.each(f)
	emit(f, n.End)
}
func (n *LocalDecl) children(f func(any)) {
	emit(f, n.Keyword)
	n.Names.each(f)
	emit(f, n.Semi)
}
func (n *InitialClause) children(f func(any)) { emit(f, n.Initial, n.X, n.Semi) }
func (n *BadDecl) children(f func(any))       { emit(f, n.Elems...) }

// ---------------------------------------------------------------------
// Expressions

// Expr is an expression.
type Expr interface {
	Node
	expr()
}

type (
	// Ident is an identifier.
	Ident struct{ Name *Token }

	// Literal is an integer, real, string or cset literal.
	Literal struct{ Tok *Token }

	// Keyword is &name (Name may be the reserved word fail).
	Keyword struct{ Amp, Name *Token }

	// Unary is a prefix operator, including "not".
	Unary struct {
		Op *Token
		X  Expr
	}

	// Binary is an infix operator, including &, ?, |, :=, augmented
	// assignments, limitation (\), activation (@) and application (!).
	Binary struct {
		X  Expr
		Op *Token
		Y  Expr
	}

	// ToBy is "X to Y" or "X to Y by Z".
	ToBy struct {
		X  Expr
		To *Token
		Y  Expr
		By *Token // optional
		Z  Expr
	}

	// Paren is (e) or the mutual evaluation (e1, e2, ...).
	Paren struct {
		Lparen *Token
		List   List[Expr]
		Rparen *Token
	}

	// Block is { e1; e2; ... }.
	Block struct {
		Lbrace *Token
		List   List[Expr]
		Rbrace *Token
	}

	// ListLit is the list constructor [e1, e2, ...].
	ListLit struct {
		Lbrack *Token
		List   List[Expr]
		Rbrack *Token
	}

	// Index is X[i, j, ...].
	Index struct {
		X      Expr
		Lbrack *Token
		List   List[Expr]
		Rbrack *Token
	}

	// Section is X[lo:hi], X[lo+:n] or X[lo-:n].
	Section struct {
		X      Expr
		Lbrack *Token
		Lo     Expr
		Op     *Token // COLON, PCOLON or MCOLON
		Hi     Expr
		Rbrack *Token
	}

	// Call is F(args).
	Call struct {
		Fun    Expr
		Lparen *Token
		Args   List[Expr]
		Rparen *Token
	}

	// PDCO is the programmer-defined control operation F{args}.
	PDCO struct {
		Fun    Expr
		Lbrace *Token
		Args   List[Expr]
		Rbrace *Token
	}

	// Field is X.name.
	Field struct {
		X    Expr
		Dot  *Token
		Name *Token
	}

	// Create is "create X".
	Create struct {
		Create *Token
		X      Expr
	}

	// If is "if Cond then Then [else Else]".
	If struct {
		If   *Token
		Cond Expr
		Then *Token
		Body Expr
		Else *Token // optional
		Alt  Expr
	}

	// Loop is "while/until/every Cond [do Body]" or "repeat Body"
	// (Cond is nil for repeat).
	Loop struct {
		Keyword *Token
		Cond    Expr
		Do      *Token
		Body    Expr
	}

	// Return is "fail", "return [X]" or "suspend [X] [do Body]".
	Return struct {
		Keyword *Token
		X       Expr
		Do      *Token
		Body    Expr
	}

	// Break is "break [X]".
	Break struct {
		Break *Token
		X     Expr
	}

	// Next is "next".
	Next struct{ Next *Token }

	// Case is "case X of { clauses }".
	Case struct {
		Case, Of, Lbrace *Token
		X                Expr
		Clauses          List[*CaseClause] // Seps are semicolons
		Rbrace           *Token
	}

	// BadExpr holds tokens (and any partial expressions) that could not
	// be parsed.
	BadExpr struct {
		Elems []any // *Token or Node
	}
)

// CaseClause is "Label : Body" or "default : Body".
type CaseClause struct {
	Default *Token // set for the default clause, else Label is set
	Label   Expr
	Colon   *Token
	Body    Expr
}

func (n *CaseClause) children(f func(any)) { emit(f, n.Default, n.Label, n.Colon, n.Body) }

func (*Ident) expr()   {}
func (*Literal) expr() {}
func (*Keyword) expr() {}
func (*Unary) expr()   {}
func (*Binary) expr()  {}
func (*ToBy) expr()    {}
func (*Paren) expr()   {}
func (*Block) expr()   {}
func (*ListLit) expr() {}
func (*Index) expr()   {}
func (*Section) expr() {}
func (*Call) expr()    {}
func (*PDCO) expr()    {}
func (*Field) expr()   {}
func (*Create) expr()  {}
func (*If) expr()      {}
func (*Loop) expr()    {}
func (*Return) expr()  {}
func (*Break) expr()   {}
func (*Next) expr()    {}
func (*Case) expr()    {}
func (*BadExpr) expr() {}

func (n *Ident) children(f func(any))   { emit(f, n.Name) }
func (n *Literal) children(f func(any)) { emit(f, n.Tok) }
func (n *Keyword) children(f func(any)) { emit(f, n.Amp, n.Name) }
func (n *Unary) children(f func(any))   { emit(f, n.Op, n.X) }
func (n *Binary) children(f func(any))  { emit(f, n.X, n.Op, n.Y) }
func (n *ToBy) children(f func(any))    { emit(f, n.X, n.To, n.Y, n.By, n.Z) }
func (n *Paren) children(f func(any)) {
	emit(f, n.Lparen)
	n.List.each(f)
	emit(f, n.Rparen)
}
func (n *Block) children(f func(any)) {
	emit(f, n.Lbrace)
	n.List.each(f)
	emit(f, n.Rbrace)
}
func (n *ListLit) children(f func(any)) {
	emit(f, n.Lbrack)
	n.List.each(f)
	emit(f, n.Rbrack)
}
func (n *Index) children(f func(any)) {
	emit(f, n.X, n.Lbrack)
	n.List.each(f)
	emit(f, n.Rbrack)
}
func (n *Section) children(f func(any)) { emit(f, n.X, n.Lbrack, n.Lo, n.Op, n.Hi, n.Rbrack) }
func (n *Call) children(f func(any)) {
	emit(f, n.Fun, n.Lparen)
	n.Args.each(f)
	emit(f, n.Rparen)
}
func (n *PDCO) children(f func(any)) {
	emit(f, n.Fun, n.Lbrace)
	n.Args.each(f)
	emit(f, n.Rbrace)
}
func (n *Field) children(f func(any))  { emit(f, n.X, n.Dot, n.Name) }
func (n *Create) children(f func(any)) { emit(f, n.Create, n.X) }
func (n *If) children(f func(any)) {
	emit(f, n.If, n.Cond, n.Then, n.Body, n.Else, n.Alt)
}
func (n *Loop) children(f func(any))   { emit(f, n.Keyword, n.Cond, n.Do, n.Body) }
func (n *Return) children(f func(any)) { emit(f, n.Keyword, n.X, n.Do, n.Body) }
func (n *Break) children(f func(any))  { emit(f, n.Break, n.X) }
func (n *Next) children(f func(any))   { emit(f, n.Next) }
func (n *Case) children(f func(any)) {
	emit(f, n.Case, n.X, n.Of, n.Lbrace)
	n.Clauses.each(f)
	emit(f, n.Rbrace)
}
func (n *BadExpr) children(f func(any)) { emit(f, n.Elems...) }
