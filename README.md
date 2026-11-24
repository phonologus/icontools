# icontools

Modern tooling for the [Icon programming language](https://www.cs.arizona.edu/icon/):
a source formatter, a language server, and editor support for Zed and
Visual Studio Code.

- **`icfmt`** formats Icon source code, in the manner of `gofmt`.
- **`iconls`** is a language server: diagnostics, hover documentation, go to
  definition, find references, completion, signature help, rename and
  formatting, in any editor that speaks the
  [Language Server Protocol](https://microsoft.github.io/language-server-protocol/).
- **Editor extensions** for [Zed](editors/zed) and [VS Code](editors/vscode).
- A **tree-sitter grammar**, [tree-sitter-icon](https://github.com/phonologus/tree-sitter-icon),
  for syntax highlighting in Zed, Neovim, Helix and other editors.

icontools targets Icon 9.5. Its lexer, parser and name resolution follow the
Icon translator (`icont`) exactly: they have been checked against `icont`
itself on the 1,200 source files of the Icon distribution, including the
Icon Program Library.

## Installation

icontools is written in Go. With Go 1.24 or later installed:

```sh
go install github.com/phonologus/icontools/cmd/icfmt@latest
go install github.com/phonologus/icontools/cmd/iconls@latest
```

Then, optionally, fetch the Icon reference documentation for hover help
on built-in functions and keywords (see [Documentation](#documentation)):

```sh
iconls fetch-docs
```

## icfmt

```sh
icfmt file.icn       # print the formatted file
icfmt -d file.icn    # show what would change, as a diff
icfmt -l dir         # list the files that would change
icfmt -w dir         # format files in place
```

Like `gofmt`, icfmt keeps your line breaks and normalises everything else:
indentation and spacing in the style of the Icon Program Library, closing
braces and alignment in the style of `gofmt`. For example,

```icon
procedure main(args)
  local count,total
    every i:=1 to *args do
  {
  total+:=args[i]   # sum
  count +:= 1 # how many
  }
end
```

becomes

```icon
procedure main(args)
   local count, total
   every i := 1 to *args do {
      total +:= args[i] # sum
      count +:= 1       # how many
   }
end
```

Formatting never changes the meaning of a program: in particular it never
moves a line break where Icon would insert (or stop inserting) a semicolon.
Files with syntax errors are left alone. Hand-aligned assignments and
operators are kept aligned, and sections maintained by VIB, the IPL's
interface builder, are left exactly as they are.

The rules are described in detail in [docs/design.md](docs/design.md#formatter-icfmt).

## iconls

`iconls` is started by your editor and talks to it over standard input and
output. It provides:

- **diagnostics** as you type: syntax errors, and the checks the Icon
  translator makes (undeclared identifiers, redeclarations, invalid
  keywords), plus a warning when an assignment replaces a built-in function
  (`name := ...`);
- **hover** with declarations and their comments, and documentation for
  built-in functions, keywords and `$define`d symbols;
- **go to definition**, **find references**, **highlighting** and
  **rename**, across the files of a program;
- **completion** of names in scope, built-in functions, `&keywords` and
  record fields, and **signature help** for calls;
- the **outline** of a file, and a **workspace symbol** search;
- **formatting** with icfmt.

### Editors

- **Zed**: install the [Icon extension](editors/zed).
- **VS Code** (and compatible editors such as VSCodium and Cursor): install
  the [Icon extension](editors/vscode).
- **Other editors**: configure an LSP client to run `iconls` for `.icn`
  files.

### Programs and libraries

Icon has no project files, so `iconls` decides which files make up a
program: the file being edited, the other `.icn` files in its directory
(except other programs, that is files with their own `main`), and the files
they `link`. Linked files are looked for in the linking file's directory,
in the directories listed in the `ipath` setting (in Zed and VS Code, see
the extensions' READMEs), and in those named by `$IPATH` and `$LPATH`. Point
`ipath` at the Icon Program Library's `procs` and `gprocs` directories to
have library procedures recognised. If a linked file cannot be found,
undeclared identifiers are not reported, since they may be declared there.

### Documentation

Hover help for built-in functions and keywords always includes a one-line
summary, taken from the (public-domain) Icon sources, and a link to the
online Icon reference. For the full reference entries, run
`iconls fetch-docs` once: it downloads the reference pages from the Icon
website into a cache on your machine, which `iconls` then uses. The
website's documentation is not distributed with icontools.

## Repository

| Path | Contents |
|---|---|
| [`cmd/icfmt`](cmd/icfmt), [`cmd/iconls`](cmd/iconls) | the two commands |
| [`internal/lexer`](internal/lexer), [`internal/syntax`](internal/syntax) | a lossless, error-tolerant lexer and parser for Icon |
| [`internal/format`](internal/format) | the formatter |
| [`internal/sema`](internal/sema) | name resolution and semantic checks |
| [`internal/lsp`](internal/lsp) | the language server |
| [`internal/builtin`](internal/builtin) | built-in functions and keywords, and the documentation cache |
| [`editors/zed`](editors/zed), [`editors/vscode`](editors/vscode) | editor extensions |
| [`docs`](docs) | [design notes](docs/design.md) and [development guide](docs/development.md) |

Contributions are welcome; see [docs/development.md](docs/development.md)
for how the project is built and tested.

## Licence

MIT; see [LICENSE](LICENSE). The catalogue of built-in functions and
keywords is generated from the Icon sources, which are in the public domain.
