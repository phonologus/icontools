# Icon for Zed

Icon language support for [Zed](https://zed.dev): syntax highlighting,
outline and bracket matching (from
[tree-sitter-icon](https://github.com/phonologus/tree-sitter-icon)), and
the `iconls` language server from icontools: diagnostics, hover with
documentation, go to definition, find references, completion, signature
help, rename and formatting.

## Setup

Install the language server (it must be on your PATH):

```sh
go install github.com/phonologus/icontools/cmd/iconls@latest
iconls fetch-docs   # optional: full documentation for built-ins on hover
```

To format with icfmt when saving, turn on format-on-save for Icon in
Zed's `settings.json` (Zed's default is off, and an extension cannot
change that). `editor: format` works without it.

```json
"languages": {
  "Icon": { "format_on_save": "on" }
}
```

Files with syntax errors are not formatted (as with gofmt).

Optional settings (Zed `settings.json`):

```json
"lsp": {
  "iconls": {
    "binary": { "path": "/path/to/iconls" },
    "initialization_options": {
      "ipath": ["/path/to/icon/ipl/procs", "/path/to/icon/ipl/gprocs"]
    }
  }
}
```

`ipath` lists directories in which to find the sources of linked
libraries (`link` declarations), so that their procedures are known.

## Development

In Zed, run `zed: install dev extension` and choose this directory. Zed
builds the Rust part (needs Rust via rustup) and the grammar.
