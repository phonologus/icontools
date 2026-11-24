package syntax

import (
	"fmt"

	"github.com/phonologus/icontools/internal/lexer"
	"github.com/phonologus/icontools/internal/token"
)

// Parse parses an Icon source file. It always returns a File whose tokens
// cover the whole source; problems are reported in File.Errors.
//
// The grammar is that of icon/src/h/grammar.h. Precedence and
// associativity follow its expr..expr11 levels, and control structures
// (if, while, every, return, create, ...) extend as far to the right as
// possible, as yacc's shift preference makes them do in icont.
func Parse(src string) *File {
	toks, lerrs := lexer.Tokenize(src)
	p := &parser{toks: toks, lastErr: -1}
	for _, e := range lerrs {
		p.errs = append(p.errs, Error{e.Offset, e.Msg})
	}
	f := &File{Src: src, Tokens: toks}
	f.Decls = p.decls()
	f.EOF = p.next()
	f.Errors = p.errs
	return f
}

type parser struct {
	toks    []Token
	pos     int // index of the current token
	errs    []Error
	lastErr int // token index of the last syntax error, to suppress cascades
}

// tok returns the current token. ILLEGAL tokens (already reported by the
// lexer) are not skipped here; list-level loops fold them into BadExpr or
// BadDecl nodes.
func (p *parser) tok() *Token { return &p.toks[p.pos] }
func (p *parser) kind() token.Kind {
	return p.toks[p.pos].Kind
}

func (p *parser) next() *Token {
	t := &p.toks[p.pos]
	if t.Kind != token.EOF {
		p.pos++
	}
	return t
}

func (p *parser) got(k token.Kind) *Token {
	if p.kind() == k {
		return p.next()
	}
	return nil
}

func (p *parser) errorf(format string, args ...any) {
	if p.lastErr == p.pos {
		return
	}
	p.lastErr = p.pos
	p.errs = append(p.errs, Error{p.tok().Offset, fmt.Sprintf(format, args...)})
}

func describe(t *Token) string {
	switch {
	case t.Implicit:
		return "newline"
	case t.Kind == token.EOF:
		return "end of file"
	case t.Kind == token.IDENT:
		return "identifier " + t.Text
	case t.Kind.IsKeyword() || t.Kind.IsOperator():
		return fmt.Sprintf("%q", t.Text)
	}
	return t.Kind.String()
}

// expect consumes a token of kind k, or reports an error and returns nil.
func (p *parser) expect(k token.Kind) *Token {
	if t := p.got(k); t != nil {
		return t
	}
	p.errorf("expected %q, found %s", k.String(), describe(p.tok()))
	return nil
}

// ---------------------------------------------------------------------
// Declarations

func isDeclStart(k token.Kind) bool {
	switch k {
	case token.PROCEDURE, token.GLOBAL, token.RECORD, token.LINK, token.INVOCABLE:
		return true
	}
	return false
}

func (p *parser) decls() []Decl {
	var ds []Decl
	for p.kind() != token.EOF {
		switch p.kind() {
		case token.LINK:
			ds = append(ds, p.link())
		case token.GLOBAL:
			ds = append(ds, p.global())
		case token.RECORD:
			ds = append(ds, p.record())
		case token.INVOCABLE:
			ds = append(ds, p.invocable())
		case token.PROCEDURE:
			ds = append(ds, p.proc())
		default:
			if p.kind() != token.ILLEGAL {
				p.errorf("expected declaration, found %s", describe(p.tok()))
			}
			bad := &BadDecl{}
			for p.kind() != token.EOF && !isDeclStart(p.kind()) {
				bad.Elems = append(bad.Elems, p.next())
			}
			ds = append(ds, bad)
		}
	}
	return ds
}

// identList parses IDENT {, IDENT}.
func (p *parser) identList() List[*Token] {
	var l List[*Token]
	l.Elems = append(l.Elems, p.expect(token.IDENT))
	for {
		c := p.got(token.COMMA)
		if c == nil {
			return l
		}
		l.Seps = append(l.Seps, c)
		l.Elems = append(l.Elems, p.expect(token.IDENT))
	}
}

func (p *parser) link() *LinkDecl {
	d := &LinkDecl{Link: p.next()}
	for {
		switch p.kind() {
		case token.IDENT, token.STRINGLIT:
			d.Files.Elems = append(d.Files.Elems, p.next())
		default:
			p.errorf("expected file name, found %s", describe(p.tok()))
			d.Files.Elems = append(d.Files.Elems, nil)
		}
		c := p.got(token.COMMA)
		if c == nil {
			return d
		}
		d.Files.Seps = append(d.Files.Seps, c)
	}
}

func (p *parser) global() *GlobalDecl {
	return &GlobalDecl{Global: p.next(), Names: p.identList()}
}

func (p *parser) record() *RecordDecl {
	d := &RecordDecl{Record: p.next()}
	d.Name = p.expect(token.IDENT)
	d.Lparen = p.expect(token.LPAREN)
	if p.kind() == token.IDENT {
		d.Fields = p.identList()
	}
	d.Rparen = p.expect(token.RPAREN)
	return d
}

func (p *parser) invocable() *InvocableDecl {
	d := &InvocableDecl{Invocable: p.next()}
	for {
		it := &InvocableItem{}
		switch p.kind() {
		case token.IDENT:
			it.Name = p.next()
		case token.STRINGLIT:
			it.Name = p.next()
			if it.Colon = p.got(token.COLON); it.Colon != nil {
				it.Arity = p.expect(token.INTLIT)
			}
		default:
			p.errorf("expected name or string, found %s", describe(p.tok()))
		}
		d.Items.Elems = append(d.Items.Elems, it)
		c := p.got(token.COMMA)
		if c == nil {
			return d
		}
		d.Items.Seps = append(d.Items.Seps, c)
	}
}

func (p *parser) proc() *ProcDecl {
	d := &ProcDecl{Procedure: p.next()}
	d.Name = p.expect(token.IDENT)
	d.Lparen = p.expect(token.LPAREN)
	if p.kind() == token.IDENT {
		d.Params = p.identList()
		if d.Lbrack = p.got(token.LBRACK); d.Lbrack != nil {
			d.Rbrack = p.expect(token.RBRACK)
		}
	}
	d.Rparen = p.expect(token.RPAREN)
	d.Semi = p.expect(token.SEMICOL)

	for p.kind() == token.LOCAL || p.kind() == token.STATIC {
		l := &LocalDecl{Keyword: p.next()}
		l.Names = p.identList()
		l.Semi = p.expect(token.SEMICOL)
		d.Locals = append(d.Locals, l)
	}
	if p.kind() == token.INITIAL {
		in := &InitialClause{Initial: p.next()}
		in.X = p.exprRequired()
		in.Semi = p.expect(token.SEMICOL)
		d.Initial = in
	}

	// procbody: { nexpr ; }
	for p.kind() != token.END && p.kind() != token.EOF && !isDeclStart(p.kind()) {
		start := p.pos
		x := p.statement(token.SEMICOL)
		semi := p.got(token.SEMICOL)
		if p.pos == start {
			// No progress (e.g. a stray "}"): consume one token.
			p.errorf("unexpected %s", describe(p.tok()))
			x = &BadExpr{Elems: []any{p.next()}}
		} else if semi == nil && p.kind() != token.EOF && !isDeclStart(p.kind()) {
			// (At EOF or a declaration, "missing end" is reported below.)
			p.errorf("expected \";\" or newline, found %s", describe(p.tok()))
		}
		d.Body.Elems = append(d.Body.Elems, x)
		d.Body.Seps = append(d.Body.Seps, semi)
	}
	if d.End = p.got(token.END); d.End == nil {
		p.errorf("missing \"end\" of procedure")
	}
	return d
}

// statement parses one element of a semicolon-separated sequence: a
// possibly empty expression. If what follows is neither the separator nor
// a token that ends the sequence, the rest of the element (up to a
// separator, closing brace, "end" or declaration keyword, skipping
// balanced brackets) is gathered into a BadExpr.
func (p *parser) statement(sep token.Kind) Expr {
	x := p.nexpr()
	switch k := p.kind(); {
	case k == sep, k == token.END, k == token.EOF, k == token.RBRACE, isDeclStart(k):
		return x
	}
	if p.kind() != token.ILLEGAL {
		p.errorf("unexpected %s", describe(p.tok()))
	}
	bad := &BadExpr{}
	emit(func(c any) { bad.Elems = append(bad.Elems, c) }, x)
	depth := 0
	for {
		k := p.kind()
		if k == token.EOF || isDeclStart(k) || k == token.END {
			break
		}
		if depth == 0 && (k == sep || k == token.RBRACE) {
			break
		}
		switch k {
		case token.LPAREN, token.LBRACK, token.LBRACE:
			depth++
		case token.RPAREN, token.RBRACK, token.RBRACE:
			if depth > 0 {
				depth--
			}
		}
		bad.Elems = append(bad.Elems, p.next())
	}
	return bad
}

// ---------------------------------------------------------------------
// Expressions

// canStart reports whether k can begin an expression.
func canStart(k token.Kind) bool {
	switch k {
	case token.END, token.DEFAULT, token.INITIAL, token.LOCAL, token.STATIC:
		return false
	}
	return k.IsBeginner()
}

// nexpr parses an optional expression; it returns nil if none is present.
func (p *parser) nexpr() Expr {
	if !canStart(p.kind()) {
		return nil
	}
	return p.expr()
}

// exprRequired parses an expression, reporting an error if there is none.
func (p *parser) exprRequired() Expr {
	if !canStart(p.kind()) {
		p.errorf("expected expression, found %s", describe(p.tok()))
		return nil
	}
	return p.expr()
}

type level struct {
	ops   map[token.Kind]bool
	right bool
	to    bool // the special "to ... by" level
}

func ops(ks ...token.Kind) map[token.Kind]bool {
	m := map[token.Kind]bool{}
	for _, k := range ks {
		m[k] = true
	}
	return m
}

// levels lists the binary operator levels of grammar.h, loosest first:
// expr, expr1a, expr1, expr2, ..., expr9.
var levels = []level{
	{ops: ops(token.AND)},
	{ops: ops(token.QMARK)},
	{right: true, ops: ops(
		token.ASSIGN, token.SWAP, token.REVASSIGN, token.REVSWAP,
		token.AUGCONCAT, token.AUGLCONCAT, token.AUGDIFF, token.AUGUNION,
		token.AUGPLUS, token.AUGMINUS, token.AUGSTAR, token.AUGINTER,
		token.AUGSLASH, token.AUGMOD, token.AUGCARET, token.AUGNMEQ,
		token.AUGEQUIV, token.AUGNMGE, token.AUGNMGT, token.AUGNMLE,
		token.AUGNMLT, token.AUGNMNE, token.AUGNEQUIV, token.AUGSEQ,
		token.AUGSGE, token.AUGSGT, token.AUGSLE, token.AUGSLT,
		token.AUGSNE, token.AUGQMARK, token.AUGAND, token.AUGAT)},
	{to: true},
	{right: true, ops: ops(token.BAR)},
	{ops: ops(
		token.SEQ, token.SGE, token.SGT, token.SLE, token.SLT, token.SNE,
		token.NMEQ, token.NMGE, token.NMGT, token.NMLE, token.NMLT, token.NMNE,
		token.EQUIV, token.NEQUIV)},
	{ops: ops(token.CONCAT, token.LCONCAT)},
	{ops: ops(token.PLUS, token.DIFF, token.UNION, token.MINUS)},
	{ops: ops(token.STAR, token.INTER, token.SLASH, token.MOD)},
	{right: true, ops: ops(token.CARET)},
	{ops: ops(token.BACKSLASH, token.AT, token.BANG)},
}

// unaryOps are the prefix operators of expr10.
var unaryOps = ops(
	token.AT, token.NOT, token.BAR, token.CONCAT, token.LCONCAT, token.DOT,
	token.BANG, token.DIFF, token.PLUS, token.STAR, token.SLASH, token.CARET,
	token.INTER, token.TILDE, token.MINUS, token.NMEQ, token.NMNE, token.SEQ,
	token.SNE, token.EQUIV, token.UNION, token.QMARK, token.NEQUIV,
	token.BACKSLASH)

func (p *parser) expr() Expr { return p.binary(0) }

func (p *parser) binary(lv int) Expr {
	if lv == len(levels) {
		return p.unary()
	}
	l := &levels[lv]
	x := p.binary(lv + 1)
	switch {
	case l.to:
		for p.kind() == token.TO {
			n := &ToBy{X: x, To: p.next()}
			n.Y = p.operand(lv + 1)
			if n.By = p.got(token.BY); n.By != nil {
				n.Z = p.operand(lv + 1)
			}
			x = n
		}
	case l.right:
		if l.ops[p.kind()] {
			op := p.next()
			return &Binary{X: x, Op: op, Y: p.operand(lv)}
		}
	default:
		for l.ops[p.kind()] {
			op := p.next()
			x = &Binary{X: x, Op: op, Y: p.operand(lv + 1)}
		}
	}
	return x
}

// operand parses the right operand of a binary operator at level lv.
func (p *parser) operand(lv int) Expr {
	if !canStart(p.kind()) {
		p.errorf("expected expression, found %s", describe(p.tok()))
		return nil
	}
	return p.binary(lv)
}

func (p *parser) unary() Expr {
	if unaryOps[p.kind()] {
		op := p.next()
		if !canStart(p.kind()) {
			p.errorf("expected expression, found %s", describe(p.tok()))
			return &Unary{Op: op}
		}
		return &Unary{Op: op, X: p.unary()}
	}
	return p.postfix(p.primary())
}

// primary parses the non-postfix forms of expr11.
func (p *parser) primary() Expr {
	switch p.kind() {
	case token.IDENT:
		return &Ident{Name: p.next()}
	case token.INTLIT, token.REALLIT, token.STRINGLIT, token.CSETLIT:
		return &Literal{Tok: p.next()}
	case token.AND:
		n := &Keyword{Amp: p.next()}
		if p.kind() == token.IDENT || p.kind() == token.FAIL {
			n.Name = p.next()
		} else {
			p.errorf("expected keyword name after \"&\", found %s", describe(p.tok()))
		}
		return n
	case token.LPAREN:
		n := &Paren{Lparen: p.next()}
		n.List = p.exprList(token.RPAREN)
		n.Rparen = p.expect(token.RPAREN)
		return n
	case token.LBRACE:
		n := &Block{Lbrace: p.next()}
		n.List = p.compound()
		n.Rbrace = p.expect(token.RBRACE)
		return n
	case token.LBRACK:
		n := &ListLit{Lbrack: p.next()}
		n.List = p.exprList(token.RBRACK)
		n.Rbrack = p.expect(token.RBRACK)
		return n
	case token.CREATE:
		return &Create{Create: p.next(), X: p.exprRequired()}
	case token.NEXT:
		return &Next{Next: p.next()}
	case token.BREAK:
		return &Break{Break: p.next(), X: p.nexpr()}
	case token.FAIL:
		return &Return{Keyword: p.next()}
	case token.RETURN:
		return &Return{Keyword: p.next(), X: p.nexpr()}
	case token.SUSPEND:
		n := &Return{Keyword: p.next(), X: p.nexpr()}
		if n.Do = p.got(token.DO); n.Do != nil {
			n.Body = p.exprRequired()
		}
		return n
	case token.IF:
		n := &If{If: p.next()}
		n.Cond = p.exprRequired()
		n.Then = p.expect(token.THEN)
		n.Body = p.exprRequired()
		if n.Else = p.got(token.ELSE); n.Else != nil {
			n.Alt = p.exprRequired()
		}
		return n
	case token.WHILE, token.UNTIL, token.EVERY:
		n := &Loop{Keyword: p.next()}
		n.Cond = p.exprRequired()
		if n.Do = p.got(token.DO); n.Do != nil {
			n.Body = p.exprRequired()
		}
		return n
	case token.REPEAT:
		n := &Loop{Keyword: p.next()}
		n.Body = p.exprRequired()
		return n
	case token.CASE:
		return p.caseExpr()
	}
	// Not reached for tokens satisfying canStart.
	p.errorf("expected expression, found %s", describe(p.tok()))
	return nil
}

// postfix parses the postfix forms of expr11 applied to x.
func (p *parser) postfix(x Expr) Expr {
	if x == nil {
		return nil
	}
	for {
		switch p.kind() {
		case token.LBRACK:
			lb := p.next()
			first := p.nexpr()
			switch p.kind() {
			case token.COLON, token.PCOLON, token.MCOLON:
				if first == nil {
					p.errorf("expected expression before %q", p.tok().Text)
				}
				n := &Section{X: x, Lbrack: lb, Lo: first, Op: p.next()}
				n.Hi = p.exprRequired()
				n.Rbrack = p.expect(token.RBRACK)
				x = n
			default:
				n := &Index{X: x, Lbrack: lb}
				n.List = p.exprListFrom(first, token.RBRACK)
				n.Rbrack = p.expect(token.RBRACK)
				x = n
			}
		case token.LPAREN:
			n := &Call{Fun: x, Lparen: p.next()}
			n.Args = p.exprList(token.RPAREN)
			n.Rparen = p.expect(token.RPAREN)
			x = n
		case token.LBRACE:
			n := &PDCO{Fun: x, Lbrace: p.next()}
			n.Args = p.exprList(token.RBRACE)
			n.Rbrace = p.expect(token.RBRACE)
			x = n
		case token.DOT:
			n := &Field{X: x, Dot: p.next()}
			n.Name = p.expect(token.IDENT)
			x = n
		default:
			return x
		}
	}
}

// exprList parses nexpr {, nexpr}, followed by the closer (not consumed).
func (p *parser) exprList(closer token.Kind) List[Expr] {
	return p.exprListFrom(p.nexpr(), closer)
}

func (p *parser) exprListFrom(first Expr, closer token.Kind) List[Expr] {
	var l List[Expr]
	l.Elems = append(l.Elems, first)
	for {
		c := p.got(token.COMMA)
		if c == nil {
			break
		}
		l.Seps = append(l.Seps, c)
		l.Elems = append(l.Elems, p.nexpr())
	}
	if len(l.Elems) == 1 && l.Elems[0] == nil {
		l.Elems = nil
	}
	if p.kind() != closer {
		p.errorf("expected \",\" or %q, found %s", closer.String(), describe(p.tok()))
	}
	return l
}

// compound parses nexpr {; nexpr} inside braces.
func (p *parser) compound() List[Expr] {
	var l List[Expr]
	for {
		l.Elems = append(l.Elems, p.statement(token.SEMICOL))
		c := p.got(token.SEMICOL)
		if c == nil {
			break
		}
		l.Seps = append(l.Seps, c)
	}
	if len(l.Elems) == 1 && l.Elems[0] == nil {
		l.Elems = nil
	}
	return l
}

func (p *parser) caseExpr() Expr {
	n := &Case{Case: p.next()}
	n.X = p.exprRequired()
	n.Of = p.expect(token.OF)
	n.Lbrace = p.expect(token.LBRACE)
	if n.Lbrace == nil {
		return n
	}
	for {
		cl := &CaseClause{}
		if cl.Default = p.got(token.DEFAULT); cl.Default == nil {
			cl.Label = p.exprRequired()
		}
		cl.Colon = p.expect(token.COLON)
		cl.Body = p.exprRequired()
		n.Clauses.Elems = append(n.Clauses.Elems, cl)
		c := p.got(token.SEMICOL)
		if c == nil {
			break
		}
		n.Clauses.Seps = append(n.Clauses.Seps, c)
	}
	n.Rbrace = p.expect(token.RBRACE)
	return n
}
