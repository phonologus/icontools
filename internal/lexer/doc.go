// Package lexer converts Icon source into a lossless token stream.
//
// Unlike icont's lexer (icon/src/common/yylex.h), it keeps comments and
// whitespace as trivia attached to tokens, records byte-accurate positions,
// and reports semicolons it inserts (Ender at end of line followed by a
// Beginner on the next) as distinct from semicolons written in the source.
// Concatenating every token's leading trivia and text reproduces the input
// exactly.
//
// Preprocessor lines ($define, $include, $ifdef, ...) are recognised and
// kept as directive tokens; they are not expanded here.
package lexer
