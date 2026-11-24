package syntax_test

import (
	"fmt"
	"strings"

	"github.com/phonologus/icontools/internal/syntax"
	"github.com/phonologus/icontools/internal/token"
)

// icontTree renders a procedure in the S-expression form printed by
// "icontok -t", reproducing the tree icont builds (see the action macros
// in icon/src/icont/tgrammar.c). If pos is nil, leaves are shown with
// their text instead of their position.
func icontTree(d *syntax.ProcDecl, pos func(*syntax.Token) string) string {
	w := &treeWriter{pos: pos}
	w.open("Proc")
	w.leaf("Id", d.Name)
	if d.Initial != nil {
		w.expr(d.Initial.X)
	} else {
		w.empty()
	}
	// procbody: nexpr ; procbody  =>  Slist(x, rest), ending in Empty.
	for _, x := range d.Body.Elems {
		w.open("Slist")
		w.expr(x)
	}
	w.empty()
	for range d.Body.Elems {
		w.close()
	}
	w.word("end")
	w.close()
	return w.b.String()
}

type treeWriter struct {
	b   strings.Builder
	pos func(*syntax.Token) string
}

func (w *treeWriter) sp() {
	if s := w.b.String(); len(s) > 0 && s[len(s)-1] != '(' {
		w.b.WriteByte(' ')
	}
}
func (w *treeWriter) open(name string) { w.sp(); w.b.WriteString("(" + name) }
func (w *treeWriter) close()           { w.b.WriteByte(')') }
func (w *treeWriter) empty()           { w.sp(); w.b.WriteString("()") }
func (w *treeWriter) word(s string)    { w.sp(); w.b.WriteString(s) }

func (w *treeWriter) leaf(kind string, t *syntax.Token) {
	w.sp()
	if t == nil {
		fmt.Fprintf(&w.b, "(%s ?)", kind)
		return
	}
	if w.pos == nil {
		fmt.Fprintf(&w.b, "(%s %s)", kind, t.Text)
	} else {
		fmt.Fprintf(&w.b, "(%s %s)", kind, w.pos(t))
	}
}

// op writes an operator token's spelling, with digraphs normalised.
func (w *treeWriter) op(t *syntax.Token) {
	w.word(t.Kind.String())
}

func (w *treeWriter) expr(x syntax.Expr) {
	switch x := x.(type) {
	case nil:
		w.empty()
	case *syntax.Ident:
		w.leaf("Id", x.Name)
	case *syntax.Literal:
		w.leaf(map[token.Kind]string{
			token.INTLIT: "Int", token.REALLIT: "Real",
			token.STRINGLIT: "Str", token.CSETLIT: "Cset",
		}[x.Tok.Kind], x.Tok)
	case *syntax.Keyword:
		w.sp()
		fmt.Fprintf(&w.b, "(Key %s)", x.Name.Text)
	case *syntax.Unary:
		switch x.Op.Kind {
		case token.NOT:
			w.open("Not")
		case token.BAR, token.CONCAT, token.LCONCAT:
			w.open("Bar")
		case token.AT:
			w.open("Activat")
			w.op(x.Op)
			w.expr(x.X)
			w.empty()
			w.close()
			return
		default:
			w.open("Unop")
			w.op(x.Op)
		}
		w.expr(x.X)
		w.close()
	case *syntax.Binary:
		w.binary(x)
	case *syntax.ToBy:
		if x.By == nil {
			w.open("To")
			w.expr(x.X)
			w.expr(x.Y)
		} else {
			w.open("ToBy")
			w.expr(x.X)
			w.expr(x.Y)
			w.expr(x.Z)
		}
		w.close()
	case *syntax.Paren:
		if len(x.List.Elems) > 1 {
			w.open("Invok")
			w.empty()
			w.elist(x.List.Elems)
			w.close()
		} else {
			w.elist(x.List.Elems)
		}
	case *syntax.Block:
		// compound: nexpr | nexpr ; compound  =>  right-nested Slist.
		es := x.List.Elems
		if len(es) == 0 {
			w.empty()
			return
		}
		for _, e := range es[:len(es)-1] {
			w.open("Slist")
			w.expr(e)
		}
		w.expr(es[len(es)-1])
		for range es[:len(es)-1] {
			w.close()
		}
	case *syntax.ListLit:
		w.open("List")
		w.elist(x.List.Elems)
		w.close()
	case *syntax.Index:
		// buildarray: a[i,j] => Binop([, Binop([, a, i), j).
		es := x.List.Elems
		if len(es) == 0 {
			es = []syntax.Expr{nil}
		}
		for range es {
			w.open("Binop")
			w.word("[")
		}
		w.expr(x.X)
		for _, e := range es {
			w.expr(e)
			w.close()
		}
	case *syntax.Section:
		w.open("Sect")
		w.op(x.Op)
		w.expr(x.X)
		w.expr(x.Lo)
		w.expr(x.Hi)
		w.close()
	case *syntax.Call:
		w.open("Invok")
		w.expr(x.Fun)
		w.elist(x.Args.Elems)
		w.close()
	case *syntax.PDCO:
		w.open("Invok")
		w.expr(x.Fun)
		w.open("List")
		// pdcolist: left-nested Elist of Create nodes.
		es := x.Args.Elems
		if len(es) == 0 {
			w.empty()
		} else {
			for range es[1:] {
				w.open("Elist")
			}
			for i, e := range es {
				w.open("Create")
				w.expr(e)
				w.close()
				if i > 0 {
					w.close()
				}
			}
		}
		w.close()
		w.close()
	case *syntax.Field:
		w.open("Field")
		w.expr(x.X)
		w.leaf("Id", x.Name)
		w.close()
	case *syntax.Create:
		w.open("Create")
		w.expr(x.X)
		w.close()
	case *syntax.If:
		w.open("If")
		w.expr(x.Cond)
		w.expr(x.Body)
		w.expr(x.Alt)
		w.close()
	case *syntax.Loop:
		w.open("Loop")
		w.op(x.Keyword)
		if x.Keyword.Kind == token.REPEAT {
			w.expr(x.Body)
			w.empty()
		} else {
			w.expr(x.Cond)
			w.expr(x.Body)
		}
		w.close()
	case *syntax.Return:
		switch x.Keyword.Kind {
		case token.SUSPEND:
			w.open("Loop")
			w.op(x.Keyword)
			w.expr(x.X)
			w.expr(x.Body)
		default:
			w.open("Ret")
			w.op(x.Keyword)
			w.expr(x.X)
		}
		w.close()
	case *syntax.Break:
		w.open("Break")
		w.expr(x.X)
		w.close()
	case *syntax.Next:
		w.open("Next")
		w.close()
	case *syntax.Case:
		w.open("Case")
		w.expr(x.X)
		// caselist: left-nested Clist of Ccls.
		cs := x.Clauses.Elems
		for range cs[1:] {
			w.open("Clist")
		}
		for i, c := range cs {
			w.open("Ccls")
			if c.Default != nil {
				w.word("default")
			} else {
				w.expr(c.Label)
			}
			w.expr(c.Body)
			w.close()
			if i > 0 {
				w.close()
			}
		}
		w.close()
	default:
		w.word(fmt.Sprintf("(?%T)", x))
	}
}

// elist writes an exprlist: a single element, or a left-nested Elist.
func (w *treeWriter) elist(es []syntax.Expr) {
	if len(es) == 0 {
		w.empty()
		return
	}
	for range es[1:] {
		w.open("Elist")
	}
	for i, e := range es {
		w.expr(e)
		if i > 0 {
			w.close()
		}
	}
}

func (w *treeWriter) binary(x *syntax.Binary) {
	k := x.Op.Kind
	s := k.String()
	switch {
	case k == token.AND || k == token.AUGAND:
		w.open("Conj")
	case k == token.QMARK || k == token.AUGQMARK:
		w.open("Scan")
	case k == token.BAR:
		w.open("Alt")
		w.expr(x.X)
		w.expr(x.Y)
		w.close()
		return
	case k == token.AT || k == token.AUGAT:
		w.open("Activat")
		w.op(x.Op)
		w.expr(x.Y)
		w.expr(x.X)
		w.close()
		return
	case k == token.BACKSLASH:
		w.open("Limit")
		w.expr(x.X)
		w.expr(x.Y)
		w.close()
		return
	case k == token.BANG:
		w.open("Apply")
		w.expr(x.X)
		w.expr(x.Y)
		w.close()
		return
	case strings.HasSuffix(s, ":=") && s != ":=":
		w.open("Augop")
	default:
		w.open("Binop")
	}
	w.op(x.Op)
	w.expr(x.X)
	w.expr(x.Y)
	w.close()
}
