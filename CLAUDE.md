# icontools

Go formatter (`icfmt`) and language server (`iconls`) for Icon 9.5, written
together by the owner and Claude. The owner knows C, Go and Icon well; the
parser, pretty-printing and LSP techniques are the areas where Claude leads.
Aim for small, working increments.

- Reference implementation: `../icon` (a clone of gtownsend/icon; read-only,
  never modify it, no PRs). Treat its C sources as the authority on syntax.
- Record syntax facts and design decisions in `docs/design.md`.
- Every change keeps `gofmt -l .`, `go vet ./...` and `go test ./...` clean.
- Test against the real corpus (`internal/corpus`) wherever possible, not
  only on hand-written snippets.
- Standard library only unless there is a strong reason; discuss new
  dependencies first.
- Git: commit locally; push to origin only when the owner asks. Push
  tokens are per repository: `../git/icontools` and
  `../git/tree-sitter-icon` — pass the right one per command via an inline
  credential helper (with `credential.helper=` first, to bypass the
  keychain); never print a token or store it in config or URLs.
