// Package format formats Icon source code in the style of the Icon Program
// Library, in the manner of go/format.
//
// Line breaks are kept as written; indentation and spacing are
// normalised. Formatting never changes the token sequence the Icon
// translator sees, including inserted semicolons. See docs/design.md.
package format
