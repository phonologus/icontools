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
| `docs/` | design notes |

## Reference tree

Tests use a clone of <https://github.com/gtownsend/icon> as ground truth
(token tables, grammar) and as a corpus of real Icon code (`tests/`, `ipl/`).
It is found at `$ICON_SRC`, or `../icon` relative to this repository. Tests
that need it are skipped if it is absent.

## Development

```sh
go test ./...
go vet ./...
go build ./cmd/...
```

### The icont oracle

`tools/icontok` is a tiny C driver that links icont's own lexer and prints
its token stream. With it, `TestOracle` checks our lexer against icont on
the whole corpus (preprocessing files with `icont -E` where needed).

```sh
ICONTOK=$(tools/icontok/build.sh) go test ./internal/lexer
```

`build.sh` copies the reference tree to a scratch directory (default
`$TMPDIR/icontok-build`) and builds there, so `../icon` is never modified.
