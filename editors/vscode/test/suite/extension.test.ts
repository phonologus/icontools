import * as assert from "assert";
import * as path from "path";
import * as vscode from "vscode";

const root = vscode.workspace.workspaceFolders![0].uri;
const main = vscode.Uri.joinPath(root, "main.icn");

async function until<T>(f: () => T | undefined | Promise<T | undefined>, what: string): Promise<T> {
  for (let i = 0; i < 100; i++) {
    const v = await f();
    if (v) return v;
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error("timed out waiting for " + what);
}

function positionOf(doc: vscode.TextDocument, text: string): vscode.Position {
  const off = doc.getText().indexOf(text);
  assert.ok(off >= 0, `${text} not found`);
  return doc.positionAt(off);
}

suite("Icon extension", () => {
  let doc: vscode.TextDocument;

  suiteSetup(async () => {
    doc = await vscode.workspace.openTextDocument(main);
    await vscode.window.showTextDocument(doc);
  });

  test("files are the icon language", () => {
    assert.strictEqual(doc.languageId, "icon");
  });

  test("diagnostics from iconls", async () => {
    const diags = await until(() => {
      const d = vscode.languages.getDiagnostics(main);
      return d.length > 0 ? d : undefined;
    }, "diagnostics");
    assert.strictEqual(diags.length, 1);
    assert.match(diags[0].message, /"undeclared": undeclared identifier/);
    assert.strictEqual(diags[0].severity, vscode.DiagnosticSeverity.Warning);
  });

  test("hover shows the declaration and its comment", async () => {
    const hovers = await vscode.commands.executeCommand<vscode.Hover[]>(
      "vscode.executeHoverProvider", main, positionOf(doc, "area"));
    const text = hovers.flatMap((h) => h.contents.map((c) => (c as vscode.MarkdownString).value)).join("\n");
    assert.match(text, /procedure area\(s\)/);
    assert.match(text, /area returns the area of a square/);
  });

  test("go to definition crosses files", async () => {
    const locs = await vscode.commands.executeCommand<(vscode.Location | vscode.LocationLink)[]>(
      "vscode.executeDefinitionProvider", main, positionOf(doc, "area"));
    assert.strictEqual(locs.length, 1);
    const uri = "uri" in locs[0] ? locs[0].uri : locs[0].targetUri;
    assert.strictEqual(path.basename(uri.fsPath), "lib.icn");
  });

  test("keyword completion after &", async () => {
    const list = await vscode.commands.executeCommand<vscode.CompletionList>(
      "vscode.executeCompletionItemProvider", main, positionOf(doc, "&subject").translate(0, 1));
    const labels = list.items.map((i) => (typeof i.label === "string" ? i.label : i.label.label));
    assert.ok(labels.includes("subject") && labels.includes("pos"), "keywords offered");
  });

  test("formatting reindents with icfmt", async () => {
    const edits = await vscode.commands.executeCommand<vscode.TextEdit[]>(
      "vscode.executeFormatDocumentProvider", main, { tabSize: 3, insertSpaces: true });
    assert.ok(edits && edits.length > 0, "edits returned");
    const we = new vscode.WorkspaceEdit();
    we.set(main, edits);
    await vscode.workspace.applyEdit(we);
    assert.match(doc.getText(), /^   local p$/m);
  });

  test("format on save and tab size are on for Icon by default", () => {
    const cfg = vscode.workspace.getConfiguration("editor", { languageId: "icon" });
    assert.strictEqual(cfg.get("formatOnSave"), true);
    assert.strictEqual(cfg.get("tabSize"), 3);
  });
});
