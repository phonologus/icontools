/*
 * icontok -- dump the token stream produced by icont's own lexer.
 *
 * Used as an oracle for testing icontools/internal/lexer. Built against a
 * compiled copy of the reference Icon tree by build.sh; links every icont
 * object except tunix.o (which holds icont's main).
 *
 * Output: one line per token, "<spelling>\t<line>[\t<image>]", where
 * <spelling> is the keyword/operator text or a description such as
 * "identifier", and <image> is present for identifiers and numeric
 * literals. Inserted semicolons appear like any other SEMICOL.
 */
#include "../h/gsupport.h"
#include "tproto.h"
#include "tglobals.h"
#include "tree.h"
#include "ttoken.h"
#include "../h/lexdef.h"

extern int yylex(void);
extern struct toktab toktab[];
extern struct optab optab[];

/* spell returns the spelling (or description) of token type t. */
static char *spell(int t) {
   int i;
   for (i = 0; toktab[i].t_type != 0; i++)
      if (toktab[i].t_type == t)
         return toktab[i].t_word;
   for (i = 0; optab[i].tok.t_word != NULL; i++)
      if (optab[i].tok.t_type == t)
         return optab[i].tok.t_word;
   return "?";
}
extern nodeptr yylval;
extern int tfatals;

int main(int argc, char *argv[]) {
   int t;

   initglob();
   tmalloc();
   if (argc != 2) {
      fprintf(stderr, "usage: icontok file.icn\n");
      return 2;
   }
   in_line = 1;
   incol = 0;
   peekc = 0;
   if (!ppinit(argv[1], ""))
      quitf("cannot open %s", argv[1]);
   tok_loc.n_file = argv[1];
   in_line = 1;
   tminit();
   while ((t = yylex()) != 0) {
      printf("%s\t%d", spell(t), tok_loc.n_line);
      if ((t == IDENT || t == INTLIT || t == REALLIT) && yylval != NULL)
         printf("\t%s", Str0(yylval));
      putchar('\n');
      if (t == EOFX)
         break;
   }
   return tfatals ? 1 : 0;
}

/* Stubs for things tunix.c would otherwise supply. */
void report(char *s) { }
