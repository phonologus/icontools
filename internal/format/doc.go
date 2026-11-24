// Package format pretty-prints a syntax tree in canonical style, in the
// manner of go/format.
//
// Invariant: formatting never changes the token sequence the Icon
// translator sees, including inserted semicolons. A line may be broken
// only where doing so cannot insert a semicolon, and a line may be joined
// only where doing so cannot remove one.
package format
