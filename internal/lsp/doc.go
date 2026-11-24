// Package lsp implements iconls, a Language Server Protocol server for
// Icon.
//
// The server speaks LSP 3.17 over a single connection (normally the
// process's standard input and output), processing messages in order. It
// keeps the full text of each open document (full-document sync),
// reparses it on every change with the error-tolerant parser in package
// syntax, and publishes the syntax errors as diagnostics.
//
// Supported: diagnostics, document symbols (procedures with their
// parameters and locals, records with fields, globals, links), and
// formatting (icfmt; files with syntax errors are left alone). Positions
// are UTF-16 code units unless the client offers UTF-8.
//
// The JSON-RPC layer is a small hand-written implementation using only
// the standard library.
package lsp
