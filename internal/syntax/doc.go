// Package syntax parses Icon into a lossless syntax tree.
//
// The tree is typed, in the style of go/ast, but keeps every token (with
// its comments and whitespace as leading trivia), so the source can be
// reproduced exactly; Tokens(file) yields every token once, in order.
// Parse never fails: errors are collected in File.Errors and recovery
// produces BadExpr/BadDecl nodes and nil fields for missing parts, so an
// incomplete file (as seen by a language server) still yields a useful
// tree.
//
// The grammar follows icon/src/h/grammar.h. Conformance is checked against
// icont's own parser on the reference corpus and on mutated copies of it;
// see TestOracle and TestMutations.
package syntax
