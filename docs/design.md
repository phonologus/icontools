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

## Parser

`syntax.Parse` is a hand-written recursive-descent parser over the token
stream, following `grammar.h`.

- **Precedence**: one function level per grammar level, loosest first:
  `&`; `?`; assignments and augmented assignments (right-assoc); `to ...
  by` (left); `|` (right); comparisons; `|| |||`; `+ - ++ --`;
  `* / % **`; `^` (right); `\ @ !` (binary); then prefix operators
  (including `not`), then postfix forms (`x[...]`, `x(...)`, `x{...}`,
  `x.f`).
- **Control structures** (`if`, `while`, `every`, `return`, `suspend`,
  `create`, `break`, ...) are at the tightest level but end in a full
  expression, and yacc's shift preference makes them extend as far right
  as possible: `x := if a then b else c + 1` puts `c + 1` in the else
  branch.
- **Unary forms of binary-only tokens** (`||x`, `--x`, `==x`, ...) parse as
  unary operators. icont treats `|`, `||` and `|||` prefixes all as
  repeated alternation.
- **Statement separators**: a procedure body is `{ nexpr ; }`, so the last
  expression before `end` needs `;` or a newline: `return 1 end` on one
  line is an error. Case clauses are separated (not terminated) by `;`, so
  `{ 1: a; }` is an error too.
- **Empty lists**: `f()`, `f{}`, `[]`, `{}` have no elements in the tree;
  `f(a,,b)` has a nil middle element.
- **Not checked by the parser** (icont reports these from its symbol
  table): redeclared identifiers, inconsistent redeclaration of globals,
  unknown `&keywords`. These belong in a later semantic pass.

### Verification

`tools/icontok -t` replaces icont's code generator with a printer of the
parse tree it builds for each procedure. The tests map our tree to the
same form (`icontree_test.go`, mirroring the action macros in
`icont/tgrammar.c`) and compare:

- `TestOracle`: every corpus file. Both parsers accept or reject the same
  files, and all 7750 procedures produce identical trees, including leaf
  positions.
- `TestMutations`: random token deletions, duplications and swaps applied
  to corpus files (`MUTATIONS=n`, default 300). In a 30,000-mutant run,
  icont and we agreed on validity and trees in every case.

## Formatter (`icfmt`)

Indentation and spacing follow the Icon Program Library; brace placement
and alignment follow gofmt.

**Line breaks are mostly the programmer's.** Like gofmt, icfmt keeps the
line breaks in the source (collapsing runs of blank lines to one) and
normalises indentation and spacing. With no line-length limit it never
needs to break an expression, which is what would be dangerous under
semicolon insertion. The exceptions all involve braces, where a line
break can never matter, because only an Ender followed by a Beginner gets
a semicolon, and `{` is not an Ender and `}` not a Beginner:

- A lone `{` (block or `case`) is pulled up onto the previous line
  (`do\n{` → `do {`), unless the token before it is a semicolon (explicit
  or implicit), i.e. the block is a statement in its own right.
- If a `{ ... }` group spans lines, its contents start on a new line after
  `{`, and `}` goes on a new line. Groups written on one line
  (`{ a; b }`) stay on one line.

**Indentation** (3 spaces per level):

- Procedure body, `local`, `static`, `initial`: one level; `end` at 0.
- `{ ... }` and `case ... of { ... }`: contents one level beyond the line
  holding `{`; the closing `}` at the indentation of that line (Go style),
  so `} else {` falls out naturally.
- Bracketed lists (call arguments, `[...]`, `(...)`, PDCO `{...}`): as
  in Go, if the opening bracket ends its line, the contents are indented
  one level beyond the line holding it, so nested calls written one per
  line indent progressively; a closing bracket that starts a line is
  indented like the line holding its opening bracket.
- Bodies of `then`, `else`, `do`, `repeat`, `suspend ... do` that start
  on a new line: one level beyond the line holding the keyword.
- `then` or `else` on a new line aligns with its `if` when the `if`
  begins its line or follows an `else` that does (else-if chains stay
  flat); otherwise (`x := if ...`) it is a continuation line. Likewise
  `do` with its `while`/`until`/`every`/`suspend`.
- Continuation lines: one level beyond the line where their statement
  (or control-structure body) starts.
- Comments on their own line take the indentation of the following token
  (body level before `end`, contents level before `}`), except comments
  that start in column 0, which stay there. Directives stay in column 0.

**Spacing**: binary operators (including `:=`, `to`, `by`, `&`, `|`, `?`)
are surrounded by spaces; unary operators are not followed by one (`not`
is); no spaces inside brackets or around section operators (`s[i + 1:0]`);
`, ` and `; ` are followed by a space; `case` labels by `: `. Two tokens
are never written adjacent if that would lex differently (`- -x` stays
apart rather than becoming `--x`). Trailing whitespace is removed.

**Alignment**, applied in this order:

1. In a run of consecutive single-line clauses of the same `case`, the
   bodies start in a common column (as gofmt aligns key–value pairs). A
   clause spanning several lines ends the run.
2. In a run of consecutive lines at the same indentation, each holding a
   complete single-line assignment statement (`:=`, `:=:`, `<-`, `<->`, or
   an augmented `op:=`), the operators are aligned on their `:` (on the
   first character for `<-` and `<->`), so augmented assignments line up
   with plain ones on `:=`:
   ```
   files       := before(files, param[i + 1])
   i          +:= 1
   s         ||:= t
   longer_name := f(x)
   ```
   This is Icon's nearest equivalent to the tables gofmt aligns in Go
   `var`/`const` blocks and struct declarations. But Icon has no block
   marking a group as a table, and runs of assignments are everywhere in
   ordinary code, so (as for rule 3) the run is aligned only if the author
   aligned it: at least one of its operators was padded in the source.
3. In a run of consecutive lines at the same indentation that end with the
   same binary operator, the operators start in a common column, as in
   ```
   b ||:=
      fmt_TAG     || # SubChunkID
      wu32(16r10) || # SubChunkSize
      wu16(br)       # BitsPerSample
   ```
   gofmt has no such rule, and applied blindly it makes one long line push
   a whole run far right. So it applies only when the author aligned the
   run: at least one of its operators was preceded by padding (two or more
   blanks, or a tab) in the source. Otherwise operators get single spaces.
4. In a run of consecutive lines with trailing comments, the comments
   start in a common column one space beyond the longest code (as gofmt). (Like gofmt, this can make one long line push a whole
run of comments to the right; gofmt's heuristics for breaking such runs
are a possible refinement.)

**Safety checks** (`internal/format` tests, over the whole corpus): the
output has the same significant token stream as the input, including
implicit semicolons; every comment and directive is preserved; and
formatting is idempotent.

**VIB sections** are left exactly as they are: everything from a
`#===<<vib:begin>>===` marker line through the matching
`#===<<vib:end>>===` line (and the older `xie` markers). VIB, the IPL's
interface builder, generates that code and rewrites it on every save, so
formatting it would only cause churn. The file is formatted as usual and
the original lines are then put back between the markers; since the
markers are full-line comments, this cannot split a token or a line.

Not handled: inactive `$ifdef` regions (files whose preprocessor branches
do not parse as a whole are refused).
