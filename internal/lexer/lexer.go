package lexer

import (
	"strings"
	"unicode/utf8"

	"github.com/phonologus/icontools/internal/token"
)

// TriviaKind classifies text that carries no tokens.
type TriviaKind uint8

const (
	Space     TriviaKind = iota // run of blanks, tabs, \r, \v, \f (never \n)
	Newline                     // a single "\n"
	Comment                     // "#" up to, not including, end of line
	Directive                   // preprocessor line "$..." up to, not including, end of line
)

func (k TriviaKind) String() string {
	switch k {
	case Space:
		return "Space"
	case Newline:
		return "Newline"
	case Comment:
		return "Comment"
	case Directive:
		return "Directive"
	}
	return "TriviaKind(?)"
}

// Trivia is a piece of source text between tokens.
type Trivia struct {
	Kind   TriviaKind
	Offset int // byte offset in the source
	Text   string
}

// Token is a lexical token together with the trivia that precedes it.
//
// Implicit semicolons (inserted by the newline rule) have Kind SEMICOL,
// Implicit set, empty Text, and no Leading trivia; their Offset is the end
// of the preceding token. The final token always has Kind EOF and carries
// any trailing trivia.
type Token struct {
	Kind     token.Kind
	Offset   int    // byte offset of Text in the source
	Text     string // exact source text of the token
	Leading  []Trivia
	Implicit bool
}

// End returns the byte offset just past the token's text.
func (t *Token) End() int { return t.Offset + len(t.Text) }

// Error is a lexical error. The messages match icont's where icont has one.
type Error struct {
	Offset int
	Msg    string
}

func (e Error) Error() string { return e.Msg }

// Tokenize splits src into tokens. It never fails: malformed input yields
// ILLEGAL tokens or best-effort literals, with the problems reported in
// the returned errors. Concatenating each token's Leading trivia text and
// Text reproduces src exactly.
func Tokenize(src string) ([]Token, []Error) {
	l := &lexer{src: src, atLineStart: true}
	l.run()
	return l.toks, l.errs
}

type lexer struct {
	src  string
	pos  int
	toks []Token
	errs []Error

	atLineStart bool // only whitespace seen since the last newline
	sawNewline  bool // a newline occurred since the last significant token
	lastEnder   bool // the last significant token was an Ender
}

func (l *lexer) errorf(off int, msg string) {
	l.errs = append(l.errs, Error{off, msg})
}

func (l *lexer) peek(i int) byte {
	if l.pos+i < len(l.src) {
		return l.src[l.pos+i]
	}
	return 0
}

func (l *lexer) run() {
	for {
		lead := l.trivia()
		start := l.pos
		if start >= len(l.src) {
			l.toks = append(l.toks, Token{Kind: token.EOF, Offset: start, Leading: lead})
			return
		}
		kind := l.scan()
		tok := Token{Kind: kind, Offset: start, Text: l.src[start:l.pos], Leading: lead}

		if kind == token.ILLEGAL {
			// icont skips bad characters without disturbing the
			// semicolon-insertion state.
			l.toks = append(l.toks, tok)
			continue
		}
		if l.sawNewline && l.lastEnder && kind.IsBeginner() {
			at := start - triviaLen(lead)
			l.toks = append(l.toks, Token{Kind: token.SEMICOL, Offset: at, Implicit: true})
		}
		l.toks = append(l.toks, tok)
		l.sawNewline = false
		l.lastEnder = kind.IsEnder()
		l.atLineStart = false
	}
}

func triviaLen(ts []Trivia) int {
	n := 0
	for _, t := range ts {
		n += len(t.Text)
	}
	return n
}

// trivia consumes whitespace, comments and directive lines.
func (l *lexer) trivia() []Trivia {
	var out []Trivia
	for l.pos < len(l.src) {
		start := l.pos
		var kind TriviaKind
		switch c := l.src[l.pos]; {
		case c == '\n':
			l.pos++
			kind = Newline
			l.sawNewline = true
			l.atLineStart = true
		case isSpace(c):
			for l.pos < len(l.src) && isSpace(l.src[l.pos]) {
				l.pos++
			}
			kind = Space
		case c == '#':
			l.toEOL()
			kind = Comment
		case c == '$' && l.atLineStart && isDirectiveStart(l.peek(1)):
			// ipp.c: a line whose first non-blank is '$', followed by
			// a non-punctuation character or a blank, is a directive.
			l.toEOL()
			kind = Directive
		default:
			return out
		}
		out = append(out, Trivia{kind, start, l.src[start:l.pos]})
	}
	return out
}

func (l *lexer) toEOL() {
	if i := strings.IndexByte(l.src[l.pos:], '\n'); i >= 0 {
		l.pos += i
	} else {
		l.pos = len(l.src)
	}
}

// scan consumes one token starting at l.pos and returns its kind.
func (l *lexer) scan() token.Kind {
	c := l.src[l.pos]
	switch {
	case isLetter(c) || c == '_':
		start := l.pos
		for l.pos < len(l.src) && (isAlnum(l.src[l.pos]) || l.src[l.pos] == '_') {
			l.pos++
		}
		return token.Lookup(l.src[start:l.pos])
	case isDigit(c) || c == '.':
		return l.number()
	case c == '"' || c == '\'':
		return l.quoted()
	}
	return l.operator()
}

// number follows getnum in icon/src/common/yylex.h, including its
// treatment of a leading '.' (a real literal if a digit follows, otherwise
// the DOT operator).
func (l *lexer) number() token.Kind {
	start := l.pos
	c := l.src[l.pos]
	l.pos++
	var state, radix int
	isReal := false
	if c == '.' {
		state, isReal = 7, true
	} else {
		radix = int(c - '0')
	}
	for {
		var c byte
		if l.pos < len(l.src) {
			c = l.src[l.pos]
		}
		switch state {
		case 0: // integer part
			switch {
			case isDigit(c):
				if radix < 1000 {
					radix = radix*10 + int(c-'0')
				}
				l.pos++
				continue
			case c == '.':
				state, isReal = 1, true
				l.pos++
				continue
			case c == 'e' || c == 'E':
				state, isReal = 2, true
				l.pos++
				continue
			case c == 'r' || c == 'R':
				if radix < 2 || radix > 36 {
					l.errorf(start, "invalid radix for integer literal")
				}
				state = 5
				l.pos++
				continue
			}
		case 1: // fractional part
			if isDigit(c) {
				l.pos++
				continue
			}
			if c == 'e' || c == 'E' {
				state = 2
				l.pos++
				continue
			}
		case 2, 3: // optional exponent sign, then first exponent digit
			if state == 2 && (c == '+' || c == '-') {
				state = 3
				l.pos++
				continue
			}
			if isDigit(c) {
				state = 4
				l.pos++
				continue
			}
			l.errorf(start, "invalid real literal")
		case 4: // remaining exponent digits
			if isDigit(c) {
				l.pos++
				continue
			}
		case 5: // first digit after r
			if isAlnum(c) && digitVal(c) < radix {
				state = 6
				l.pos++
				continue
			}
			l.errorf(start, "invalid integer literal")
		case 6: // remaining digits after r
			if isAlnum(c) {
				if digitVal(c) >= radix {
					l.errorf(start, "invalid digit in integer literal")
					radix = 36 // as icont: report once
				}
				l.pos++
				continue
			}
		case 7: // began with '.'
			if isDigit(c) {
				state = 1
				l.pos++
				continue
			}
			return token.DOT
		}
		break
	}
	if isReal {
		return token.REALLIT
	}
	return token.INTLIT
}

// quoted follows getstring in icon/src/common/yylex.h: escapes, and the
// rule that '_' as the last non-blank on a line continues the literal at
// the first non-blank of the next line. An unescaped newline otherwise
// ends an (unclosed) literal.
func (l *lexer) quoted() token.Kind {
	start := l.pos
	q := l.src[l.pos]
	kind := token.STRINGLIT
	if q == '\'' {
		kind = token.CSETLIT
	}
	l.pos++
	cont := false // '_' seen with only blanks after it
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == q {
			l.pos++
			return kind
		}
		if c == '\n' {
			break
		}
		if c == '_' {
			cont = true
		} else if !isSpace(c) {
			cont = false
		}
		l.pos++
		if c == '\\' && l.pos < len(l.src) {
			// The escaped character is taken literally, even a quote or
			// newline; "\^c" takes one more.
			e := l.src[l.pos]
			l.pos++
			if e == '^' && l.pos < len(l.src) {
				l.pos++
			}
		}
		if l.pos < len(l.src) && l.src[l.pos] == '\n' && cont {
			for l.pos < len(l.src) && (isSpace(l.src[l.pos]) || l.src[l.pos] == '\n') {
				l.pos++
			}
		}
	}
	l.errorf(start, "unclosed quote")
	return kind
}

var opPrefixes = func() map[string]bool {
	m := map[string]bool{}
	add := func(s string) {
		for i := 1; i <= len(s); i++ {
			m[s[:i]] = true
		}
	}
	for s := range token.Operators {
		add(s)
	}
	for s := range token.Digraphs {
		add(s)
	}
	return m
}()

// operator takes the longest run of characters that is a prefix of some
// operator, as icont's getopr does (without backtracking).
func (l *lexer) operator() token.Kind {
	start := l.pos
	end := start
	for end < len(l.src) && opPrefixes[l.src[start:end+1]] {
		end++
	}
	if end == start {
		_, n := utf8.DecodeRuneInString(l.src[start:])
		l.pos += n
		l.errorf(start, "invalid character")
		return token.ILLEGAL
	}
	l.pos = end
	s := l.src[start:end]
	if k, ok := token.Operators[s]; ok {
		return k
	}
	if k, ok := token.Digraphs[s]; ok {
		return k
	}
	l.errorf(start, "invalid character")
	return token.ILLEGAL
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f'
}
func isDigit(c byte) bool  { return '0' <= c && c <= '9' }
func isLetter(c byte) bool { return 'a' <= c|0x20 && c|0x20 <= 'z' }
func isAlnum(c byte) bool  { return isLetter(c) || isDigit(c) }

func digitVal(c byte) int {
	if isDigit(c) {
		return int(c - '0')
	}
	return int(c|0x20-'a') + 10
}

// isDirectiveStart reports whether c, following a '$' that begins a line,
// makes the line a preprocessor directive (ipp.c: !ispunct(c) || c == ' ').
func isDirectiveStart(c byte) bool {
	isPunct := c > ' ' && c < 0x7f && !isAlnum(c)
	return !isPunct
}
