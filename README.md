# icontools

Modern tooling for the [Icon programming language](https://www.cs.arizona.edu/icon/):

- **`icfmt`** — a canonical source formatter, in the spirit of `gofmt`.
- **`iconls`** — a Language Server Protocol server, in the spirit of `gopls`.

Both are built on a shared lossless lexer and concrete-syntax-tree parser.

## Layout

| Path | Purpose |
|---|---|
| `cmd/icfmt` | formatter CLI (`-l`, `-w`, `-d`) |
| `cmd/iconls` | language server (JSON-RPC over stdio) |
| `internal/token` | token kinds and semicolon-insertion flags (from `lextab.h`) |
| `internal/lexer` | lossless lexer: trivia, positions, inserted-semicolon tracking |
| `internal/syntax` | error-tolerant CST parser following `grammar.h` |
| `internal/format` | pretty printer |
| `internal/lsp` | language-server implementation |
| `internal/corpus` | locates the reference Icon tree for tests |
| `internal/oracle` | runs icont's own lexer/parser (via `tools/icontok`) for tests |
| `tools/icontok` | C driver exposing icont's token stream and parse trees |
| `docs/` | design notes |

## Reference tree

Tests use a clone of <https://github.com/gtownsend/icon> as ground truth
(token tables, grammar) and as a corpus of real Icon code (`tests/`, `ipl/`).
It is found at `$ICON_SRC`, or `../icon` relative to this repository. Tests
that need it are skipped if it is absent.

## Using icfmt

```sh
go install ./cmd/icfmt
icfmt file.icn          # print formatted file
icfmt -d file.icn       # show a diff
icfmt -l dir/           # list files that would change
icfmt -w file.icn       # rewrite in place
```

icfmt keeps your line breaks and normalises indentation and spacing: IPL
indentation and spacing, with gofmt-style closing braces and alignment of
trailing comments and case clauses. It never changes the token sequence
icont sees, and refuses files with syntax errors. See `docs/design.md`.

## Using iconls

```sh
go install ./cmd/iconls
iconls fetch-docs      # optional: cache the Icon reference for hover docs
```

`iconls` is a language server: editors start it and talk to it over
standard input and output. It provides diagnostics (syntax errors, and
icont's semantic checks such as undeclared identifiers), hover with
documentation, go to definition, find references, highlights,
completion, signature help, rename, document and workspace symbols, and
formatting with icfmt.

Linked library sources are found via the `ipath` initialization option
(a list of directories, e.g. the IPL's `procs` and `gprocs`), `$IPATH`
and `$LPATH`. `iconls -log file` writes a server log for debugging.

## Development

```sh
go test ./...
go vet ./...
go build ./cmd/...
```

### The icont oracle

`tools/icontok` is a small C driver linked with icont's own front end. It
prints icont's token stream, or (`-t`) the parse tree of each procedure.
With it, the `TestOracle` tests check our lexer and parser against icont
on the whole corpus (preprocessing files with `icont -E` where needed),
and `TestMutations` does the same on randomly damaged copies.

```sh
ICONTOK=$(tools/icontok/build.sh) go test ./...
ICONTOK=$(tools/icontok/build.sh) MUTATIONS=30000 go test -run Mutations ./internal/syntax
```

`build.sh` copies the reference tree to a scratch directory (default
`$TMPDIR/icontok-build`) and builds there, so `../icon` is never modified.
