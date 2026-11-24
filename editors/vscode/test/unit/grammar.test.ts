// Tokenizes Icon with the TextMate grammar, using the same engine as
// VS Code (vscode-textmate with the Oniguruma regex library).
import * as assert from "assert";
import * as fs from "fs";
import * as path from "path";
import * as oniguruma from "vscode-oniguruma";
import * as tm from "vscode-textmate";

const ext = path.resolve(__dirname, "..", "..", "..");

async function loadGrammar(): Promise<tm.IGrammar> {
  const wasm = fs.readFileSync(require.resolve("vscode-oniguruma/release/onig.wasm"));
  await oniguruma.loadWASM(wasm);
  const registry = new tm.Registry({
    onigLib: Promise.resolve({
      createOnigScanner: (s: string[]) => new oniguruma.OnigScanner(s),
      createOnigString: (s: string) => new oniguruma.OnigString(s),
    }),
    loadGrammar: async () =>
      tm.parseRawGrammar(fs.readFileSync(path.join(ext, "syntaxes", "icon.tmLanguage.json"), "utf8"), "icon.json"),
  });
  return (await registry.loadGrammar("source.icon"))!;
}

// scopes returns, for each line, the innermost scope of each token text.
function scopes(grammar: tm.IGrammar, src: string): Map<string, string> {
  const out = new Map<string, string>();
  let state = tm.INITIAL;
  for (const line of src.split("\n")) {
    const r = grammar.tokenizeLine(line, state);
    for (const t of r.tokens) {
      const text = line.slice(t.startIndex, t.endIndex).trim();
      if (text && !out.has(text)) out.set(text, t.scopes[t.scopes.length - 1]);
    }
    state = r.ruleStack;
  }
  return out;
}

suite("TextMate grammar", () => {
  test("token scopes", async () => {
    const g = await loadGrammar();
    const s = scopes(g, [
      "$define WIDTH 80",
      "link strings",
      "record point(x, y)",
      "procedure main(args)   # entry",
      "   local n",
      '   n +:= find("a\\"b", s) + 16rFF + 1.5e3',
      "   if &subject then every write(p.cx, 'aeiou')",
      "end",
    ].join("\n"));
    const want: [string, string][] = [
      ["$define", "keyword.control.directive.icon"],
      ["link", "keyword.other.icon"],
      ["record", "storage.type.record.icon"],
      ["point", "entity.name.type.icon"],
      ["procedure", "storage.type.function.icon"],
      ["main", "entity.name.function.icon"],
      ["# entry", "comment.line.number-sign.icon"],
      ["local", "storage.modifier.icon"],
      ["+:=", "keyword.operator.assignment.icon"],
      ["find", "support.function.builtin.icon"],
      ['\\"', "constant.character.escape.icon"],
      ["16rFF", "constant.numeric.icon"],
      ["1.5e3", "constant.numeric.icon"],
      ["if", "keyword.control.icon"],
      ["&subject", "variable.language.keyword.icon"],
      ["every", "keyword.control.icon"],
      ["cx", "variable.other.member.icon"],
      ["end", "keyword.other.icon"],
    ];
    for (const [text, scope] of want) {
      assert.strictEqual(s.get(text), scope, `scope of ${text}`);
    }
  });

  test("a string continued with _ spans lines; otherwise it ends at the line end", async () => {
    const g = await loadGrammar();
    let r = g.tokenizeLine('s := "abc_', tm.INITIAL);
    r = g.tokenizeLine('   def" || t', r.ruleStack);
    assert.match(r.tokens[0].scopes.join(" "), /string\.quoted\.double/);
    const after = r.tokens.find((t) => t.startIndex >= 10)!;
    assert.doesNotMatch(after.scopes.join(" "), /string/);

    r = g.tokenizeLine('s := "unclosed', tm.INITIAL);
    r = g.tokenizeLine("x := 1", r.ruleStack);
    assert.doesNotMatch(r.tokens[0].scopes.join(" "), /string/);
  });
});
