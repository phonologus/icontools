# Design notes

Facts about Icon's surface syntax that the tools depend on, with their
source in the reference tree. Keep this up to date as we learn things.

## Semicolon insertion

`icon/src/common/yylex.h` (around line 167): if a newline separates two
tokens, the first an *Ender* and the second a *Beginner*, the lexer returns
a `;` between them. Flags are in `icon/src/common/lextab.h`
(mirrored in `internal/token`, checked by `TestAgainstLextab`).

Surprises:

- Several binary-only operators are Beginners (`||`, `|||`, `**`, `++`,
  `--`, `==`, `===`, `~=`, `~==`, `~===`), because the grammar lets
  e.g. `||x` parse as `|(|x)`. So a line ending in an operand followed by a
  line starting with `||` gets a semicolon. Continuation lines must
  therefore end with the operator, not begin with it.
- `:=`, `<`, `>`, `%`, `,` and the augmented assignments are neither Beginner
  nor Ender, so a line may be broken safely after them.

Consequences for the formatter:

- Break a line only after a token that is not an Ender, or before a token
  that is not a Beginner.
- Join two lines only if no semicolon was inserted between them, or write
  the `;` explicitly.
- Self-check: re-lex the output and compare the token stream, including
  inserted semicolons, with the input.

## Preprocessor

`$define`, `$undef`, `$include`, `$ifdef`, `$ifndef`, `$else`, `$endif`,
`$line`, `$error` (`icon/src/common/ipp.c`). icont's parser sees only
expanded text. Our tools keep directives in the tree and do not expand them,
except where the LSP needs to resolve defined names.

`$(`, `$)`, `$<`, `$>` are digraphs for `{`, `}`, `[`, `]`, not directives.

## Comments

`#` to end of line. icont discards them (`yylex.h` around line 88); we keep
them as trivia.

## Grammar

`icon/src/h/grammar.h` (yacc). Error messages: `icon/src/h/parserr.h`.

## Lexical details (from `yylex.h`, `getopr` in `lextab.h`, `ipp.c`)

Confirmed against icont's own lexer on the whole corpus (see
`TestOracle`).

- **Operators** are matched greedily with no backtracking: `%:x` is an
  error, not `%` `:` `x`.
- **`.`** followed by a digit begins a real literal (`.5`); otherwise it is
  the DOT operator. `3.foo` is `3.` then `foo`.
- **Numbers**: `16rFF`, `2r101` (radix 2–36, digits case-insensitive);
  `1e10`, `1.5E-3`. `10x` is `10` then `x`.
- **Keywords** (`&subject`) are `&` followed by an identifier.
- **String/cset literals**:
  - A backslash takes the next character literally, *including a newline*,
    which continues the literal onto the next line.
  - `\^c` takes one more character, so `"\^""` is a complete literal.
  - `_` as the last non-blank on a line continues the literal at the first
    non-blank of the next line (blank lines are skipped too).
  - Otherwise an unescaped newline ends the literal with "unclosed quote".
  - A newline inside a continued literal does not count for semicolon
    insertion.
- **Invalid characters** are skipped by icont without resetting the
  semicolon-insertion state; we emit ILLEGAL tokens with the same
  behaviour. An implicit semicolon is placed immediately before the
  Beginner that triggers it.
- **Directives**: a line whose first non-blank is `$`, followed by a blank
  or a non-punctuation character. So `$define` and `$ include` are
  directives, but `$(` at the start of a line is a digraph.
- **`#line n "file"`** comments make icont renumber lines. We always report
  physical positions, which is what editors need.
- **ipp predefines** the symbols in `src/h/features.h` (`_UNIX`, `_PIPES`,
  `_V9`, ...) as `1`, so code using them is expanded even with no
  `$define` in the file.

## Lexer output model

`lexer.Tokenize` returns every token with its *leading* trivia (Space,
Newline, Comment, Directive); trailing trivia at end of file hangs on the
EOF token. Implicit semicolons are zero-width SEMICOL tokens with
`Implicit` set. Concatenating trivia and token text reproduces the input
byte for byte. Offsets are bytes; `lexer.Lines` maps them to lines.

Not yet handled: code inside inactive `$ifdef` branches is lexed like any
other code, which can produce spurious errors. The formatter will need to
treat such regions verbatim.
