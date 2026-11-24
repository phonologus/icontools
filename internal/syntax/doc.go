// Package syntax parses Icon into a concrete syntax tree (CST) that keeps
// every token and its trivia, with error recovery so that incomplete source
// (as seen by a language server) still yields a useful tree.
//
// The grammar follows icon/src/h/grammar.h. Conformance is checked against
// the reference corpus (tests/ and ipl/ in the Icon tree).
package syntax
