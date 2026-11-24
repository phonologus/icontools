/*
 * icontok -- dump what icont's own front end produces, as an oracle for
 * testing icontools.
 *
 *   icontok file.icn      token stream (lexer)
 *   icontok -t file.icn   parse trees of procedures (parser)
 *
 * Built against a compiled copy of the reference Icon tree by build.sh. It
 * links every icont object except tunix.o (icont's main) and tcode.o (code
 * generation), supplying its own codegen() that prints the tree instead.
 *
 * Token mode prints one line per token, "<spelling>\t<line>[\t<image>]",
 * where <spelling> is the keyword/operator text or a description such as
 * "identifier", and <image> is present for identifiers and numeric
 * literals. Inserted semicolons appear like any other ";".
 *
 * Tree mode prints one S-expression per procedure; see dump() below.
 * Leaves for identifiers and literals show the source position as
 * line:column, with icont's column convention (1-based, tabs to
 * multiples of 8), since icont replaces their text with table indices.
 */
#include "../h/gsupport.h"
#include "tproto.h"
#include "tglobals.h"
#include "tree.h"
#include "ttoken.h"
#include "../h/lexdef.h"

extern int yylex(void);
extern int yyparse(void);
extern struct toktab toktab[];
extern struct optab optab[];
extern nodeptr yylval;
extern int tfatals;
extern FILE *codefile, *globfile;

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

/* Node type names and the number of subtree fields each has. */
static struct { char *name; int nfields; } ninfo[] = {
   [N_Activat] = {"Activat", 3}, [N_Alt] = {"Alt", 2},
   [N_Augop] = {"Augop", 3},     [N_Bar] = {"Bar", 1},
   [N_Binop] = {"Binop", 3},     [N_Break] = {"Break", 1},
   [N_Case] = {"Case", 2},       [N_Ccls] = {"Ccls", 2},
   [N_Clist] = {"Clist", 2},     [N_Conj] = {"Conj", 3},
   [N_Create] = {"Create", 1},   [N_Elist] = {"Elist", 2},
   [N_Field] = {"Field", 2},     [N_If] = {"If", 3},
   [N_Invok] = {"Invok", 2},     [N_Limit] = {"Limit", 2},
   [N_List] = {"List", 1},       [N_Loop] = {"Loop", 3},
   [N_Not] = {"Not", 1},         [N_Next] = {"Next", 0},
   [N_Proc] = {"Proc", 4},       [N_Ret] = {"Ret", 2},
   [N_Scan] = {"Scan", 3},       [N_Sect] = {"Sect", 4},
   [N_Slist] = {"Slist", 2},     [N_To] = {"To", 2},
   [N_ToBy] = {"ToBy", 3},       [N_Unop] = {"Unop", 2},
   [N_Apply] = {"Apply", 2},
};

static void dump(nodeptr t) {
   int i;

   switch (TType(t)) {
   case N_Empty:
      printf("()");
      return;
   case N_Id:   printf("(Id %d:%d)", Line(t), Col(t)); return;
   case N_Int:  printf("(Int %d:%d)", Line(t), Col(t)); return;
   case N_Real: printf("(Real %d:%d)", Line(t), Col(t)); return;
   case N_Str:  printf("(Str %d:%d)", Line(t), Col(t)); return;
   case N_Cset: printf("(Cset %d:%d)", Line(t), Col(t)); return;
   case N_Key:  printf("(Key %s)", Str0(t)); return;
   case N_Op:
   case N_Res:
      printf("%s", spell((int)Val0(t)));
      return;
   }
   if (TType(t) < 0 || TType(t) >= (int)(sizeof ninfo / sizeof ninfo[0]) ||
       ninfo[TType(t)].name == NULL) {
      printf("(?%d)", TType(t));
      return;
   }
   printf("(%s", ninfo[TType(t)].name);
   for (i = 0; i < ninfo[TType(t)].nfields; i++) {
      putchar(' ');
      dump(t->n_field[i].n_ptr);
   }
   putchar(')');
}

/* codegen replaces icont's code generator: it is called once per procedure. */
void codegen(nodeptr t) {
   dump(t);
   putchar('\n');
}

int main(int argc, char *argv[]) {
   int t, trees = 0;
   char *file;

   initglob();
   tmalloc();
   if (argc == 3 && strcmp(argv[1], "-t") == 0) {
      trees = 1;
      file = argv[2];
   } else if (argc == 2) {
      file = argv[1];
   } else {
      fprintf(stderr, "usage: icontok [-t] file.icn\n");
      return 2;
   }
   in_line = 1;
   incol = 0;
   peekc = 0;
   if (!ppinit(file, ""))
      quitf("cannot open %s", file);
   tok_loc.n_file = file;
   in_line = 1;
   tminit();
   if (trees) {
      codefile = fopen("/dev/null", "w");
      globfile = fopen("/dev/null", "w");
      yyparse();
      return tfatals ? 1 : 0;
   }
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
