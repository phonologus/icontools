// Package format formats Icon source code, in the manner of go/format:
// indentation and spacing in the style of the Icon Program Library, brace
// placement and alignment in the style of gofmt.
//
// Line breaks are kept as written, except around braces; indentation and
// spacing are normalised. Formatting never changes the token sequence the Icon
// translator sees, including inserted semicolons. See docs/design.md.
package format
