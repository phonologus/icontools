package format

import (
	"errors"
	"fmt"
	"strings"

	"github.com/phonologus/icontools/internal/lexer"
	"github.com/phonologus/icontools/internal/syntax"
	"github.com/phonologus/icontools/internal/token"
)

// Indent is one level of indentation.
const Indent = "   "

// Source formats Icon source code. It returns an error, and no output, if
// the source has syntax errors.
func Source(src []byte) ([]byte, error) {
	f := syntax.Parse(string(src))
	if len(f.Errors) > 0 {
		ls := lexer.NewLines(f.Src)
		var msgs []string
		for _, e := range f.Errors {
			l, c := ls.Position(e.Offset)
			msgs = append(msgs, fmt.Sprintf("%d:%d: %s", l, c+1, e.Msg))
		}
		return nil, errors.New(strings.Join(msgs, "\n"))
	}
	return []byte(File(f)), nil
}

// File formats a parsed file, which must be free of errors.
func File(f *syntax.File) string {
	p := &printer{toks: f.Tokens, index: map[*syntax.Token]int{}}
	for i := range f.Tokens {
		p.index[&f.Tokens[i]] = i
	}
	for _, d := range f.Decls {
		p.decl(d)
	}
	p.emit(f.EOF, tok{indent: 0})
	out := strings.TrimRight(p.b.String(), " \t\n")
	if out == "" {
		return ""
	}
	return out + "\n"
}

// printer writes tokens, deciding for each whether it starts a new line
// (only where the source has one), its indentation if so, and the spacing
// before it if not.
type printer struct {
	toks  []syntax.Token
	index map[*syntax.Token]int // position of each token in toks

	b          strings.Builder
	prev       *syntax.Token // last token written
	lineIndent int           // indentation of the current output line
	lineFirst  *syntax.Token // first token on the current output line

	// Frame of the innermost statement: continuation lines are indented
	// to cont. setCont is set until the statement's first token has been
	// written, which fixes cont relative to that token's line.
	cont    int
	setCont bool

	space *bool // if set, overrides the spacing of the next token written
	start *int  // if set, indentation for the next token if it starts a line
}

// tok describes how to place one token.
type tok struct {
	indent  int  // indentation if the token starts a line
	space   bool // separated from the previous token on the same line
	join    bool // pull onto the previous line if only blank lines intervene
	comment int  // indentation for comment lines before it (-1: use indent)
}

// put writes t with the default placement: a continuation of the current
// statement, separated by a space unless sp is false.
func (p *printer) put(t *syntax.Token, sp bool) {
	if t == nil {
		return
	}
	ind := p.cont
	if p.start != nil {
		ind = *p.start
		p.start = nil
	}
	if p.space != nil {
		sp = *p.space
		p.space = nil
	}
	p.emit(t, tok{indent: ind, space: sp, comment: -1})
}

// next sets the spacing for the next token written (the first token of a
// child node), overriding that token's default.
func (p *printer) next(sp bool) { p.space = &sp }

// at sets the indentation for the next token, if it starts a line.
func (p *printer) at(ind int) { p.start = &ind }

func (p *printer) emit(t *syntax.Token, o tok) {
	if t.Implicit {
		return
	}
	if o.comment < 0 {
		o.comment = o.indent
	}
	nl := 0   // newlines pending
	gap := "" // horizontal space before a trailing comment
	commented := false
	col0 := p.prev == nil // at column 0 of a source line
	for _, tr := range t.Leading {
		switch tr.Kind {
		case lexer.Space:
			gap = tr.Text
			col0 = false
		case lexer.Newline:
			nl++
			gap = ""
			col0 = true
		case lexer.Comment, lexer.Directive:
			commented = true
			if nl == 0 && p.prev != nil {
				// A trailing comment, on the line of the previous token.
				if gap == "" {
					gap = " "
				}
				p.b.WriteString(gap + strings.TrimRight(tr.Text, " \t\r"))
			} else {
				p.newlines(nl)
				// Comments are re-indented, except those starting in
				// column 0, which are kept there (directives always are).
				if tr.Kind == lexer.Comment && !col0 {
					p.b.WriteString(strings.Repeat(Indent, o.comment))
				}
				p.b.WriteString(strings.TrimRight(tr.Text, " \t\r"))
			}
			nl = 0
			col0 = false
		}
	}
	if o.join && !commented {
		nl = 0
	}
	if t.Kind == token.EOF {
		return
	}
	switch {
	case nl > 0 || (commented && p.prev != nil):
		p.newlines(max(nl, 1))
		p.b.WriteString(strings.Repeat(Indent, o.indent))
		p.lineIndent = o.indent
		p.lineFirst = t
	case p.prev == nil:
		p.lineIndent = 0
		p.lineFirst = t
	case o.space || wouldMerge(p.prev, t):
		p.b.WriteByte(' ')
	}
	p.b.WriteString(t.Text)
	p.prev = t
	if p.setCont {
		p.cont = p.lineIndent + 1
		p.setCont = false
	}
}

// newlines ends the current line and writes a blank line if n > 1. At the
// start of the file it writes nothing.
func (p *printer) newlines(n int) {
	if p.b.Len() == 0 {
		return
	}
	// Trim trailing blanks on the line being ended.
	s := p.b.String()
	if t := strings.TrimRight(s, " \t"); len(t) != len(s) {
		p.b.Reset()
		p.b.WriteString(t)
	}
	p.b.WriteByte('\n')
	if n > 1 {
		p.b.WriteByte('\n')
	}
}

// wouldMerge reports whether writing b directly after a would lex
// differently, e.g. "-" "-x" becoming "--x", or "1" ".x" becoming "1." "x".
func wouldMerge(a, b *syntax.Token) bool {
	toks, _ := lexer.Tokenize(a.Text + b.Text)
	return len(toks) != 3 || toks[0].Text != a.Text || toks[1].Text != b.Text
}

// ---------------------------------------------------------------------
// Statements

// stmt formats x as a statement (or control-structure body) whose first
// token, if it starts a line, is indented to ind. Continuation lines are
// indented one level beyond the line on which x starts.
func (p *printer) stmt(x syntax.Expr, ind int, sp bool) {
	if x == nil {
		return
	}
	saveCont, saveSet := p.cont, p.setCont
	p.at(ind)
	p.next(sp)
	p.setCont = true
	p.expr(x)
	p.cont, p.setCont = saveCont, saveSet
}

// seq formats a semicolon-separated sequence of statements at ind.
func (p *printer) seq(l syntax.List[syntax.Expr], ind int, sp bool) {
	for i, x := range l.Elems {
		p.stmt(x, ind, sp || i > 0)
		if i < len(l.Seps) && l.Seps[i] != nil {
			p.at(ind)
			p.put(l.Seps[i], false)
		}
	}
}

// ---------------------------------------------------------------------
// Declarations

func (p *printer) decl(d syntax.Decl) {
	p.cont = 1
	switch d := d.(type) {
	case *syntax.LinkDecl:
		p.at(0)
		p.put(d.Link, false)
		p.tokList(d.Files)
	case *syntax.GlobalDecl:
		p.at(0)
		p.put(d.Global, false)
		p.tokList(d.Names)
	case *syntax.RecordDecl:
		p.at(0)
		p.put(d.Record, false)
		p.put(d.Name, true)
		p.put(d.Lparen, false)
		p.next(false)
		p.tokList(d.Fields)
		p.put(d.Rparen, false)
	case *syntax.InvocableDecl:
		p.at(0)
		p.put(d.Invocable, false)
		for i, it := range d.Items.Elems {
			p.put(it.Name, true)
			p.put(it.Colon, false)
			p.put(it.Arity, false)
			if i < len(d.Items.Seps) {
				p.put(d.Items.Seps[i], false)
			}
		}
	case *syntax.ProcDecl:
		p.proc(d)
	}
}

// tokList writes a comma-separated list of single tokens, the first
// preceded by a space unless next(false) was called.
func (p *printer) tokList(l syntax.List[*syntax.Token]) {
	for i, t := range l.Elems {
		p.put(t, true)
		if i < len(l.Seps) {
			p.put(l.Seps[i], false)
		}
	}
}

func (p *printer) proc(d *syntax.ProcDecl) {
	p.at(0)
	p.put(d.Procedure, false)
	p.put(d.Name, true)
	p.put(d.Lparen, false)
	p.next(false)
	p.tokList(d.Params)
	p.put(d.Lbrack, false)
	p.put(d.Rbrack, false)
	p.put(d.Rparen, false)
	p.put(d.Semi, false)
	for _, l := range d.Locals {
		p.cont = 2
		p.at(1)
		p.put(l.Keyword, true)
		p.tokList(l.Names)
		p.put(l.Semi, false)
	}
	if in := d.Initial; in != nil {
		p.cont = 2
		p.at(1)
		p.put(in.Initial, true)
		p.stmt(in.X, 2, true)
		p.put(in.Semi, false)
	}
	p.seq(d.Body, 1, true)
	p.emit(d.End, tok{indent: 0, space: true, comment: 1})
}

// ---------------------------------------------------------------------
// Expressions

func (p *printer) exprs(l syntax.List[syntax.Expr]) {
	for i, x := range l.Elems {
		if x != nil {
			p.next(i > 0)
			p.expr(x)
		}
		if i < len(l.Seps) {
			p.put(l.Seps[i], false)
		}
	}
}

// joinable reports whether an opening brace t may be pulled up onto the
// previous line: only if it does not begin a statement, i.e. the source
// token before it is not a (possibly implicit) semicolon. Since an
// implicit semicolon is the only thing a newline before "{" can produce,
// joining then cannot change the token stream.
func (p *printer) joinable(t *syntax.Token) bool {
	i := p.index[t]
	return i > 0 && p.toks[i-1].Kind != token.SEMICOL
}

// block writes "{ seq }" with IPL layout: contents and the closing brace
// one level beyond the line holding the opening brace.
func (p *printer) block(lb *syntax.Token, l syntax.List[syntax.Expr], rb *syntax.Token) {
	ind := p.cont
	if p.start != nil {
		ind = *p.start
		p.start = nil
	}
	sp := p.space != nil && *p.space
	p.space = nil
	p.emit(lb, tok{indent: ind, space: sp, join: p.joinable(lb), comment: -1})
	inner := p.lineIndent + 1
	p.seq(l, inner, true)
	p.at(inner)
	p.put(rb, len(l.Elems) > 0)
}

func (p *printer) expr(x syntax.Expr) {
	switch x := x.(type) {
	case *syntax.Ident:
		p.put(x.Name, false)
	case *syntax.Literal:
		p.put(x.Tok, false)
	case *syntax.Keyword:
		p.put(x.Amp, false)
		p.put(x.Name, false)
	case *syntax.Unary:
		p.put(x.Op, false)
		p.next(x.Op.Kind == token.NOT)
		p.expr(x.X)
	case *syntax.Binary:
		p.expr(x.X)
		p.put(x.Op, true)
		p.next(true)
		p.expr(x.Y)
	case *syntax.ToBy:
		p.expr(x.X)
		p.put(x.To, true)
		p.next(true)
		p.expr(x.Y)
		if x.By != nil {
			p.put(x.By, true)
			p.next(true)
			p.expr(x.Z)
		}
	case *syntax.Paren:
		p.put(x.Lparen, false)
		p.exprs(x.List)
		p.put(x.Rparen, false)
	case *syntax.ListLit:
		p.put(x.Lbrack, false)
		p.exprs(x.List)
		p.put(x.Rbrack, false)
	case *syntax.Block:
		p.block(x.Lbrace, x.List, x.Rbrace)
	case *syntax.Index:
		p.expr(x.X)
		p.put(x.Lbrack, false)
		p.exprs(x.List)
		p.put(x.Rbrack, false)
	case *syntax.Section:
		p.expr(x.X)
		p.put(x.Lbrack, false)
		p.expr(x.Lo)
		p.put(x.Op, false)
		p.expr(x.Hi)
		p.put(x.Rbrack, false)
	case *syntax.Call:
		p.expr(x.Fun)
		p.put(x.Lparen, false)
		p.exprs(x.Args)
		p.put(x.Rparen, false)
	case *syntax.PDCO:
		p.expr(x.Fun)
		p.put(x.Lbrace, false)
		p.exprs(x.Args)
		p.put(x.Rbrace, false)
	case *syntax.Field:
		p.expr(x.X)
		p.put(x.Dot, false)
		p.put(x.Name, false)
	case *syntax.Create:
		p.put(x.Create, false)
		p.next(true)
		p.expr(x.X)
	case *syntax.If:
		before := p.prev
		p.put(x.If, false)
		l0 := p.lineIndent
		// "else" aligns with "if" when the if begins its line, or follows
		// an "else" that does (an else-if chain). Otherwise (x := if ...,
		// suspend if ...) it is a continuation line.
		elseAt := p.cont
		if p.lineFirst == x.If ||
			before != nil && before.Kind == token.ELSE && p.lineFirst == before {
			elseAt = l0
		}
		p.next(true)
		p.expr(x.Cond)
		p.put(x.Then, true)
		p.stmt(x.Body, l0+1, true)
		if x.Else != nil {
			p.at(elseAt)
			p.put(x.Else, true)
			p.stmt(x.Alt, l0+1, true)
		}
	case *syntax.Loop:
		p.put(x.Keyword, false)
		l0 := p.lineIndent
		if x.Cond != nil {
			p.next(true)
			p.expr(x.Cond)
		}
		p.put(x.Do, true)
		p.stmt(x.Body, l0+1, true)
	case *syntax.Return:
		p.put(x.Keyword, false)
		l0 := p.lineIndent
		if x.X != nil {
			p.next(true)
			p.expr(x.X)
		}
		p.put(x.Do, true)
		p.stmt(x.Body, l0+1, true)
	case *syntax.Break:
		p.put(x.Break, false)
		if x.X != nil {
			p.next(true)
			p.expr(x.X)
		}
	case *syntax.Next:
		p.put(x.Next, false)
	case *syntax.Case:
		p.put(x.Case, false)
		p.next(true)
		p.expr(x.X)
		p.put(x.Of, true)
		p.emit(x.Lbrace, tok{indent: p.cont, space: true, join: p.joinable(x.Lbrace), comment: -1})
		inner := p.lineIndent + 1
		for i, c := range x.Clauses.Elems {
			saveCont, saveSet := p.cont, p.setCont
			p.at(inner)
			p.next(true)
			p.setCont = true
			if c.Default != nil {
				p.put(c.Default, false)
			} else {
				p.expr(c.Label)
			}
			p.put(c.Colon, false)
			p.stmt(c.Body, inner+1, true)
			p.cont, p.setCont = saveCont, saveSet
			if i < len(x.Clauses.Seps) && x.Clauses.Seps[i] != nil {
				p.put(x.Clauses.Seps[i], false)
			}
		}
		p.at(inner)
		p.put(x.Rbrace, true)
	default:
		panic(fmt.Sprintf("format: unexpected %T", x))
	}
}
