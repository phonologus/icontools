// Package token defines the lexical tokens of Icon (version 9.5) and the
// per-token flags that drive automatic semicolon insertion.
//
// The tables here are transcribed from the reference implementation's
// src/common/lextab.h, which is the authority. Note that several binary-only
// operators (||, **, ==, ~=, ...) are flagged Beginner there, because the
// grammar accepts e.g. "||x" as "|(|x)". Do not "correct" these from
// op.txt; token_test.go cross-checks this file against lextab.h.
package token

import "fmt"

// Kind identifies the lexical class of a token.
type Kind uint8

const (
	ILLEGAL Kind = iota
	EOF
	COMMENT // # ... to end of line (trivia; never seen by icont's parser)

	// Primitives.
	IDENT
	INTLIT
	REALLIT
	STRINGLIT
	CSETLIT

	// Reserved words.
	keywordBeg
	BREAK
	BY
	CASE
	CREATE
	DEFAULT
	DO
	ELSE
	END
	EVERY
	FAIL
	GLOBAL
	IF
	INITIAL
	INVOCABLE
	LINK
	LOCAL
	NEXT
	NOT
	OF
	PROCEDURE
	RECORD
	REPEAT
	RETURN
	STATIC
	SUSPEND
	THEN
	TO
	UNTIL
	WHILE
	keywordEnd

	// Operators and punctuation.
	operatorBeg
	BANG
	MOD
	AUGMOD
	AND
	AUGAND
	STAR
	AUGSTAR
	INTER
	AUGINTER
	PLUS
	AUGPLUS
	UNION
	AUGUNION
	MINUS
	AUGMINUS
	DIFF
	AUGDIFF
	DOT
	SLASH
	AUGSLASH
	ASSIGN
	SWAP
	NMLT
	AUGNMLT
	REVASSIGN
	REVSWAP
	SLT
	AUGSLT
	SLE
	AUGSLE
	NMLE
	AUGNMLE
	NMEQ
	AUGNMEQ
	SEQ
	AUGSEQ
	EQUIV
	AUGEQUIV
	NMGT
	AUGNMGT
	NMGE
	AUGNMGE
	SGT
	AUGSGT
	SGE
	AUGSGE
	QMARK
	AUGQMARK
	AT
	AUGAT
	BACKSLASH
	CARET
	AUGCARET
	BAR
	CONCAT
	AUGCONCAT
	LCONCAT
	AUGLCONCAT
	TILDE
	NMNE
	AUGNMNE
	SNE
	AUGSNE
	NEQUIV
	AUGNEQUIV
	LPAREN
	RPAREN
	PCOLON
	COMMA
	MCOLON
	COLON
	SEMICOL
	LBRACK
	RBRACK
	LBRACE
	RBRACE
	operatorEnd
)

// Flags record whether a token may begin and/or end an expression.
// A semicolon is inserted between two tokens on different lines when the
// first is an Ender and the second a Beginner (see icon/src/common/yylex.h).
type Flags uint8

const (
	Beginner Flags = 1 << iota
	Ender
)

type info struct {
	name  string // spelling for keywords/operators; description otherwise
	flags Flags
}

var infos = [...]info{
	ILLEGAL: {"ILLEGAL", 0},
	EOF:     {"end-of-file", 0},
	COMMENT: {"comment", 0},

	IDENT:     {"identifier", Beginner | Ender},
	INTLIT:    {"integer-literal", Beginner | Ender},
	REALLIT:   {"real-literal", Beginner | Ender},
	STRINGLIT: {"string-literal", Beginner | Ender},
	CSETLIT:   {"cset-literal", Beginner | Ender},

	BREAK:     {"break", Beginner | Ender},
	BY:        {"by", 0},
	CASE:      {"case", Beginner},
	CREATE:    {"create", Beginner},
	DEFAULT:   {"default", Beginner},
	DO:        {"do", 0},
	ELSE:      {"else", 0},
	END:       {"end", Beginner},
	EVERY:     {"every", Beginner},
	FAIL:      {"fail", Beginner | Ender},
	GLOBAL:    {"global", 0},
	IF:        {"if", Beginner},
	INITIAL:   {"initial", Beginner},
	INVOCABLE: {"invocable", 0},
	LINK:      {"link", 0},
	LOCAL:     {"local", Beginner},
	NEXT:      {"next", Beginner | Ender},
	NOT:       {"not", Beginner},
	OF:        {"of", 0},
	PROCEDURE: {"procedure", 0},
	RECORD:    {"record", 0},
	REPEAT:    {"repeat", Beginner},
	RETURN:    {"return", Beginner | Ender},
	STATIC:    {"static", Beginner},
	SUSPEND:   {"suspend", Beginner | Ender},
	THEN:      {"then", 0},
	TO:        {"to", 0},
	UNTIL:     {"until", Beginner},
	WHILE:     {"while", Beginner},

	BANG:       {"!", Beginner},
	MOD:        {"%", 0},
	AUGMOD:     {"%:=", 0},
	AND:        {"&", Beginner},
	AUGAND:     {"&:=", 0},
	STAR:       {"*", Beginner},
	AUGSTAR:    {"*:=", 0},
	INTER:      {"**", Beginner},
	AUGINTER:   {"**:=", 0},
	PLUS:       {"+", Beginner},
	AUGPLUS:    {"+:=", 0},
	UNION:      {"++", Beginner},
	AUGUNION:   {"++:=", 0},
	MINUS:      {"-", Beginner},
	AUGMINUS:   {"-:=", 0},
	DIFF:       {"--", Beginner},
	AUGDIFF:    {"--:=", 0},
	DOT:        {".", Beginner},
	SLASH:      {"/", Beginner},
	AUGSLASH:   {"/:=", 0},
	ASSIGN:     {":=", 0},
	SWAP:       {":=:", 0},
	NMLT:       {"<", 0},
	AUGNMLT:    {"<:=", 0},
	REVASSIGN:  {"<-", 0},
	REVSWAP:    {"<->", 0},
	SLT:        {"<<", 0},
	AUGSLT:     {"<<:=", 0},
	SLE:        {"<<=", 0},
	AUGSLE:     {"<<=:=", 0},
	NMLE:       {"<=", 0},
	AUGNMLE:    {"<=:=", 0},
	NMEQ:       {"=", Beginner},
	AUGNMEQ:    {"=:=", 0},
	SEQ:        {"==", Beginner},
	AUGSEQ:     {"==:=", 0},
	EQUIV:      {"===", Beginner},
	AUGEQUIV:   {"===:=", 0},
	NMGT:       {">", 0},
	AUGNMGT:    {">:=", 0},
	NMGE:       {">=", 0},
	AUGNMGE:    {">=:=", 0},
	SGT:        {">>", 0},
	AUGSGT:     {">>:=", 0},
	SGE:        {">>=", 0},
	AUGSGE:     {">>=:=", 0},
	QMARK:      {"?", Beginner},
	AUGQMARK:   {"?:=", 0},
	AT:         {"@", Beginner},
	AUGAT:      {"@:=", 0},
	BACKSLASH:  {"\\", Beginner},
	CARET:      {"^", Beginner},
	AUGCARET:   {"^:=", 0},
	BAR:        {"|", Beginner},
	CONCAT:     {"||", Beginner},
	AUGCONCAT:  {"||:=", 0},
	LCONCAT:    {"|||", Beginner},
	AUGLCONCAT: {"|||:=", 0},
	TILDE:      {"~", Beginner},
	NMNE:       {"~=", Beginner},
	AUGNMNE:    {"~=:=", 0},
	SNE:        {"~==", Beginner},
	AUGSNE:     {"~==:=", 0},
	NEQUIV:     {"~===", Beginner},
	AUGNEQUIV:  {"~===:=", 0},
	LPAREN:     {"(", Beginner},
	RPAREN:     {")", Ender},
	PCOLON:     {"+:", 0},
	COMMA:      {",", 0},
	MCOLON:     {"-:", 0},
	COLON:      {":", 0},
	SEMICOL:    {";", 0},
	LBRACK:     {"[", Beginner},
	RBRACK:     {"]", Ender},
	LBRACE:     {"{", Beginner},
	RBRACE:     {"}", Ender},
}

// String returns the token's spelling (keywords, operators) or a
// description (primitives).
func (k Kind) String() string {
	if int(k) < len(infos) && infos[k].name != "" {
		return infos[k].name
	}
	return fmt.Sprintf("Kind(%d)", k)
}

// Flags returns the semicolon-insertion flags for k.
func (k Kind) Flags() Flags {
	if int(k) < len(infos) {
		return infos[k].flags
	}
	return 0
}

func (k Kind) IsBeginner() bool { return k.Flags()&Beginner != 0 }
func (k Kind) IsEnder() bool    { return k.Flags()&Ender != 0 }
func (k Kind) IsKeyword() bool  { return keywordBeg < k && k < keywordEnd }
func (k Kind) IsOperator() bool { return operatorBeg < k && k < operatorEnd }

// Digraphs are alternate spellings accepted by the lexer.
var Digraphs = map[string]Kind{
	"$(": LBRACE,
	"$)": RBRACE,
	"$<": LBRACK,
	"$>": RBRACK,
}

var keywords map[string]Kind

// Operators maps each operator/punctuation spelling to its Kind
// (digraphs excluded; see Digraphs).
var Operators map[string]Kind

func init() {
	keywords = make(map[string]Kind)
	for k := keywordBeg + 1; k < keywordEnd; k++ {
		keywords[infos[k].name] = k
	}
	Operators = make(map[string]Kind)
	for k := operatorBeg + 1; k < operatorEnd; k++ {
		Operators[infos[k].name] = k
	}
}

// Lookup maps an identifier to its reserved-word Kind, or IDENT.
func Lookup(ident string) Kind {
	if k, ok := keywords[ident]; ok {
		return k
	}
	return IDENT
}
