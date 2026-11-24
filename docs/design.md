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
