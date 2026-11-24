# Icon for Visual Studio Code

Support for the [Icon programming language](https://www.cs.arizona.edu/icon/):

- syntax highlighting;
- diagnostics as you type: syntax errors, and the Icon translator's checks
  (undeclared identifiers, redeclarations, invalid keywords);
- hover documentation for your procedures, records and globals (from their
  comments) and for built-in functions and keywords;
- go to definition, find references, highlight, rename;
- completion of names in scope, built-ins, `&keywords` and record fields;
- signature help;
- formatting (in the style of the Icon Program Library), on save by default.

The language features come from `iconls`, the language server in
[icontools](https://github.com/phonologus/icontools).

## Requirements

Install the language server:

```sh
go install github.com/phonologus/icontools/cmd/iconls@latest
iconls fetch-docs   # optional: full documentation for built-ins on hover
```

`iconls` must be on your `PATH`, or set `icon.server.path`.

## Settings

| Setting | Description |
|---|---|
| `icon.server.path` | Path to `iconls` (default: found on the `PATH`). |
| `icon.ipath` | Directories in which to find the sources of linked files (`link` declarations), e.g. the Icon Program Library's `procs` and `gprocs`. `$IPATH` and `$LPATH` are also searched. |
| `iconls.trace.server` | Trace the protocol in the Output panel. |

Format on save, 3-space indentation and spaces are on by default for Icon
files; change them under `"[icon]"` in your settings.

## Unicon

The Unicon extensions also claim `.icn` files (as the `unicon` language).
If both are installed, choose per workspace with, for example:

```json
"files.associations": { "*.icn": "icon" }
```
