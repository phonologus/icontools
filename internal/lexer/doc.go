// Package lexer converts Icon source into a lossless token stream.
//
// Unlike icont's lexer (icon/src/common/yylex.h), it keeps comments,
// whitespace and preprocessor directive lines as trivia attached to the
// following token, records byte offsets, and marks the semicolons it
// inserts (Ender at end of line followed by a Beginner on the next) as
// Implicit. Concatenating every token's leading trivia and text reproduces
// the input exactly.
//
// Directives ($define, $include, $ifdef, ...) are not expanded.
//
// Behaviour is checked against icont's own lexer over the reference corpus;
// see TestOracle and tools/icontok.
package lexer
