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
	p := &printer{
		toks:    f.Tokens,
		index:   map[*syntax.Token]int{},
		tokLine: map[*syntax.Token]int{},
		marks:   map[int]*marks{},
	}
	for i := range f.Tokens {
		p.index[&f.Tokens[i]] = i
	}
	for _, d := range f.Decls {
		p.decl(d)
	}
	p.emit(f.EOF, tok{indent: 0})
	out := p.align(p.b.String())
	out = strings.TrimRight(out, " \t\n")
	if out == "" {
		return ""
	}
	return out + "\n"
}

// printer writes tokens, deciding for each whether it starts a new line,
// its indentation if so, and the spacing before it if not. Line breaks are
// those of the source, except that a lone "{" may be joined to the
// previous line and multi-line blocks get their contents and closing
// brace on lines of their own (see block).
type printer struct {
	toks  []syntax.Token
	index map[*syntax.Token]int // position of each token in toks

	b          strings.Builder
	line       int // current output line (0-based)
	lineStart  int // offset in b of the current line
	prev       *syntax.Token
	lineIndent int           // indentation of the current output line
	lineFirst  *syntax.Token // first token on the current output line
	tokLine    map[*syntax.Token]int

	// Frame of the innermost statement: continuation lines are indented
	// to cont. setCont is set until the statement's first token has been
	// written, which fixes cont relative to that token's line.
	cont    int
	setCont bool

	space   *bool // if set, overrides the spacing of the next token written
	start   *int  // if set, indentation for the next token if it starts a line
	cmt     *int  // if set, indentation for comment lines before the next token
	forceNL bool  // the next token starts a new line
	markOp  bool  // record the next token as a line-final operator

	// assign is the assignment statement being printed, if any; its
	// operator position is recorded for alignment.
	assign     *syntax.Binary
	markAssign bool

	marks map[int]*marks // alignment points, by output line
	cases int            // counter identifying case expressions
}

// marks records alignment points on an output line (byte offsets, -1 if
// absent): the end of a case label's colon; the end of the code before a
// binary operator that ends the line; and the end of the code before a
// trailing comment.
type marks struct {
	caseAt, caseGroup int
	opAt              int
	opText            string
	opIndent          int
	opPadded          bool // the source had padding before the operator
	assignAt          int  // before the operator of an assignment statement
	assignIndent      int
	assignAnchor      int // offset within the operator of the column to align
	assignPadded      bool
	commentAt         int
}

func (p *printer) mark(line int) *marks {
	m := p.marks[line]
	if m == nil {
		m = &marks{caseAt: -1, opAt: -1, assignAt: -1, commentAt: -1}
		p.marks[line] = m
	}
	return m
}

// tok describes how to place one token.
type tok struct {
	indent     int  // indentation if the token starts a line
	space      bool // separated from the previous token on the same line
	join       bool // pull onto the previous line if only blank lines intervene
	newline    bool // start a new line even if the source does not
	markOp     bool // record an operator alignment point before the token
	markAssign bool // record an assignment alignment point before the token
	comment    int  // indentation for comment lines before it (-1: use indent)
}

// put writes t with the default placement: a continuation of the current
// statement, separated by a space unless sp is false. Pending settings
// from next, at, cmtAt and forceNL apply.
func (p *printer) put(t *syntax.Token, sp bool) {
	if t == nil {
		return
	}
	o := tok{indent: p.cont, space: sp, comment: -1}
	if p.start != nil {
		o.indent, p.start = *p.start, nil
	}
	if p.space != nil {
		o.space, p.space = *p.space, nil
	}
	if p.cmt != nil {
		o.comment, p.cmt = *p.cmt, nil
	}
	if p.forceNL && !t.Implicit {
		o.newline, p.forceNL = true, false
	}
	o.markOp, p.markOp = p.markOp, false
	o.markAssign, p.markAssign = p.markAssign, false
	p.emit(t, o)
}

// next sets the spacing for the next token written (the first token of a
// child node), overriding that token's default.
func (p *printer) next(sp bool) { p.space = &sp }

// at sets the indentation for the next token, if it starts a line.
func (p *printer) at(ind int) { p.start = &ind }

// cmtAt sets the indentation of comment lines before the next token.
func (p *printer) cmtAt(ind int) { p.cmt = &ind }

func (p *printer) write(s string) {
	p.b.WriteString(s)
	for i := strings.IndexByte(s, '\n'); i >= 0; i = strings.IndexByte(s, '\n') {
		p.line++
		s = s[i+1:]
		p.lineStart = p.b.Len() - len(s)
	}
}

func (p *printer) col() int { return p.b.Len() - p.lineStart }

func (p *printer) emit(t *syntax.Token, o tok) {
	if t.Implicit {
		return
	}
	if o.comment < 0 {
		o.comment = o.indent
	}
	nl := 0 // newlines pending
	commented := false
	col0 := p.prev == nil // at column 0 of a source line
	for _, tr := range t.Leading {
		switch tr.Kind {
		case lexer.Space:
			col0 = false
		case lexer.Newline:
			nl++
			col0 = true
		case lexer.Comment, lexer.Directive:
			commented = true
			text := strings.TrimRight(tr.Text, " \t\r")
			if nl == 0 && p.prev != nil {
				// A trailing comment, on the line of the previous token.
				p.mark(p.line).commentAt = p.col()
				p.write(" " + text)
			} else {
				p.newlines(nl)
				// Comments are re-indented, except those starting in
				// column 0, which are kept there (directives always are).
				if tr.Kind == lexer.Comment && !col0 {
					p.write(strings.Repeat(Indent, o.comment))
				}
				p.write(text)
			}
			nl = 0
			col0 = false
		}
	}
	if o.join && !commented {
		nl = 0
	}
	if o.newline {
		nl = max(nl, 1)
	}
	if t.Kind == token.EOF {
		return
	}
	switch {
	case nl > 0 || (commented && p.prev != nil):
		p.newlines(max(nl, 1))
		p.write(strings.Repeat(Indent, o.indent))
		p.lineIndent = o.indent
		p.lineFirst = t
	case p.prev == nil:
		p.lineIndent = 0
		p.lineFirst = t
	default:
		if o.markOp {
			m := p.mark(p.line)
			m.opAt, m.opText, m.opIndent = p.col(), t.Text, p.lineIndent
			m.opPadded = padded(t)
		}
		if o.markAssign {
			m := p.mark(p.line)
			m.assignAt, m.assignIndent = p.col(), p.lineIndent
			m.assignAnchor = max(strings.IndexByte(t.Text, ':'), 0)
			m.assignPadded = padded(t)
		}
		if o.space || wouldMerge(p.prev, t) {
			p.write(" ")
		}
	}
	p.tokLine[t] = p.line
	p.write(t.Text)
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
	p.write("\n")
	if n > 1 {
		p.write("\n")
	}
}

// wouldMerge reports whether writing b directly after a would lex
// differently, e.g. "-" "-x" becoming "--x", or "1" ".x" becoming "1." "x".
func wouldMerge(a, b *syntax.Token) bool {
	toks, _ := lexer.Tokenize(a.Text + b.Text)
	return len(toks) != 3 || toks[0].Text != a.Text || toks[1].Text != b.Text
}

// multiLine reports whether the source has a line break anywhere between
// tokens a and b (exclusive of the trivia before a).
func (p *printer) multiLine(a, b *syntax.Token) bool {
	i, j := p.index[a], p.index[b]
	for k := i; k <= j; k++ {
		t := &p.toks[k]
		if k > i {
			for _, tr := range t.Leading {
				if tr.Kind == lexer.Newline {
					return true
				}
			}
		}
		if k < j && strings.Contains(t.Text, "\n") {
			return true
		}
	}
	return false
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
		p.assignStmt(x, ind, sp || i > 0)
		if i < len(l.Seps) && l.Seps[i] != nil {
			p.at(ind)
			p.put(l.Seps[i], false)
		}
	}
}

// assignStmt is stmt, but if x is an assignment written on a line of its
// own, it records the position of the assignment operator for alignment.
func (p *printer) assignStmt(x syntax.Expr, ind int, sp bool) {
	b, ok := x.(*syntax.Binary)
	if !ok || !isAssign(b.Op.Kind) {
		p.stmt(x, ind, sp)
		return
	}
	saveAssign := p.assign
	p.assign = b
	p.stmt(x, ind, sp)
	p.assign = saveAssign
	toks := syntax.Tokens(b)
	first, line := toks[0], p.tokLine[b.Op]
	if m := p.marks[line]; m != nil && m.assignAt >= 0 &&
		!(p.startsLine(first) && p.tokLine[first] == line && p.tokLine[toks[len(toks)-1]] == line &&
			!strings.Contains(toks[len(toks)-1].Text, "\n")) {
		m.assignAt = -1 // not a single-line statement on a line of its own
	}
}

// isAssign reports whether k is an assignment operator: :=, :=:, <-, <->
// or an augmented assignment such as +:=.
func isAssign(k token.Kind) bool {
	s := k.String()
	return s == ":=:" || s == "<-" || s == "<->" || strings.HasSuffix(s, ":=")
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
		p.at(0)
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
	p.at(0)
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

// bracketed writes open, the list, and close. If close starts a line it
// is indented like the line holding open (as in Go).
func (p *printer) bracketed(open *syntax.Token, l syntax.List[syntax.Expr], close *syntax.Token) {
	p.put(open, false)
	outer := p.lineIndent
	p.exprs(l)
	p.at(outer)
	p.put(close, false)
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

// openBrace writes the "{" of a block or case and returns the indentation
// of its line. If the braces enclose a line break, the contents are
// forced onto a new line. (A newline after "{" or before "}" cannot insert
// a semicolon: "{" is not an Ender and "}" is not a Beginner.)
func (p *printer) openBrace(lb, rb *syntax.Token, sp bool) (outer int, multi bool) {
	ind := p.cont
	if p.start != nil {
		ind, p.start = *p.start, nil
	}
	if p.space != nil {
		sp, p.space = *p.space, nil
	}
	p.emit(lb, tok{indent: ind, space: sp, join: p.joinable(lb), comment: -1})
	multi = p.multiLine(lb, rb)
	p.forceNL = multi
	return p.lineIndent, multi
}

// closeBrace writes "}" Go style: on a line of its own (if the group spans
// lines) at the indentation of the line holding "{", with comments before
// it at the contents' indentation.
func (p *printer) closeBrace(rb *syntax.Token, outer int, multi, sp bool) {
	p.forceNL = multi
	p.at(outer)
	p.cmtAt(outer + 1)
	p.put(rb, sp)
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
		p.markOp = p.endsLine(x.Op)
		p.markAssign = x == p.assign
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
		p.bracketed(x.Lparen, x.List, x.Rparen)
	case *syntax.ListLit:
		p.bracketed(x.Lbrack, x.List, x.Rbrack)
	case *syntax.Block:
		outer, multi := p.openBrace(x.Lbrace, x.Rbrace, false)
		p.seq(x.List, outer+1, true)
		p.closeBrace(x.Rbrace, outer, multi, len(x.List.Elems) > 0)
	case *syntax.Index:
		p.expr(x.X)
		p.bracketed(x.Lbrack, x.List, x.Rbrack)
	case *syntax.Section:
		p.expr(x.X)
		p.put(x.Lbrack, false)
		outer := p.lineIndent
		p.expr(x.Lo)
		p.put(x.Op, false)
		p.expr(x.Hi)
		p.at(outer)
		p.put(x.Rbrack, false)
	case *syntax.Call:
		p.expr(x.Fun)
		p.bracketed(x.Lparen, x.Args, x.Rparen)
	case *syntax.PDCO:
		p.expr(x.Fun)
		p.bracketed(x.Lbrace, x.Args, x.Rbrace)
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
		// "then" and "else" starting a line align with "if" when the if
		// begins its line, or follows an "else" that does (an else-if
		// chain). Otherwise (x := if ..., suspend if ...) they are
		// continuation lines.
		elseAt := p.cont
		if p.lineFirst == x.If ||
			before != nil && before.Kind == token.ELSE && p.lineFirst == before {
			elseAt = l0
		}
		p.next(true)
		p.expr(x.Cond)
		p.at(elseAt)
		p.put(x.Then, true)
		p.stmt(x.Body, l0+1, true)
		if x.Else != nil {
			p.at(elseAt)
			p.put(x.Else, true)
			p.stmt(x.Alt, l0+1, true)
		}
	case *syntax.Loop:
		p.put(x.Keyword, false)
		l0, doAt := p.lineIndent, p.keywordAt(x.Keyword)
		if x.Cond != nil {
			p.next(true)
			p.expr(x.Cond)
		}
		p.at(doAt)
		p.put(x.Do, true)
		p.stmt(x.Body, l0+1, true)
	case *syntax.Return:
		p.put(x.Keyword, false)
		l0, doAt := p.lineIndent, p.keywordAt(x.Keyword)
		if x.X != nil {
			p.next(true)
			p.expr(x.X)
		}
		p.at(doAt)
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
		p.caseExpr(x)
	default:
		panic(fmt.Sprintf("format: unexpected %T", x))
	}
}

func (p *printer) caseExpr(x *syntax.Case) {
	p.put(x.Case, false)
	p.next(true)
	p.expr(x.X)
	p.put(x.Of, true)
	outer, multi := p.openBrace(x.Lbrace, x.Rbrace, true)
	inner := outer + 1
	p.cases++
	for i, c := range x.Clauses.Elems {
		saveCont, saveSet := p.cont, p.setCont
		p.at(inner)
		p.next(i > 0 || !multi)
		p.setCont = true
		first := c.Default
		if first != nil {
			p.put(c.Default, false)
		} else {
			first = syntax.Tokens(c.Label)[0]
			p.expr(c.Label)
		}
		p.put(c.Colon, false)
		line, at := p.line, p.col()
		p.stmt(c.Body, inner+1, true)
		p.cont, p.setCont = saveCont, saveSet
		// Record an alignment point for the body of a clause that is on a
		// line of its own.
		if c.Body != nil && p.tokLine[first] == line && p.startsLine(first) &&
			p.line == line && p.tokLine[syntax.Tokens(c.Body)[0]] == line {
			m := p.mark(line)
			m.caseAt, m.caseGroup = at, p.cases
		}
		if i < len(x.Clauses.Seps) && x.Clauses.Seps[i] != nil {
			p.put(x.Clauses.Seps[i], false)
		}
	}
	p.closeBrace(x.Rbrace, outer, multi, true)
}

// keywordAt returns the indentation for a "do" that starts a line, given
// the keyword (while, every, suspend, ...) just written: aligned with it if
// it begins its line, otherwise a continuation line.
func (p *printer) keywordAt(kw *syntax.Token) int {
	if p.lineFirst == kw {
		return p.lineIndent
	}
	return p.cont
}

// padded reports whether t is preceded in the source, on the same line, by
// more than a single space: a sign of deliberate alignment.
func padded(t *syntax.Token) bool {
	for _, tr := range t.Leading {
		if tr.Kind == lexer.Space && (len(tr.Text) > 1 || strings.Contains(tr.Text, "\t")) {
			return true
		}
	}
	return false
}

// endsLine reports whether t is the last token on its source line.
func (p *printer) endsLine(t *syntax.Token) bool {
	for i := p.index[t] + 1; i < len(p.toks); i++ {
		if p.toks[i].Implicit {
			continue
		}
		for _, tr := range p.toks[i].Leading {
			if tr.Kind == lexer.Newline {
				return true
			}
		}
		return false
	}
	return false
}

// startsLine reports whether t was written as the first token of a line.
func (p *printer) startsLine(t *syntax.Token) bool {
	l, ok := p.tokLine[t]
	if !ok {
		return false
	}
	i := p.index[t]
	return i == 0 || p.tokLine[p.prevWritten(i)] != l
}

// prevWritten returns the last non-implicit token before index i.
func (p *printer) prevWritten(i int) *syntax.Token {
	for i--; i >= 0; i-- {
		if !p.toks[i].Implicit {
			return &p.toks[i]
		}
	}
	return nil
}
