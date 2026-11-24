// Runs the integration tests in a downloaded copy of VS Code, against a
// freshly built iconls and a temporary copy of the fixture project.
import * as cp from "child_process";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { runTests } from "@vscode/test-electron";

async function main(): Promise<void> {
  const ext = path.resolve(__dirname, "..", "..");
  const repo = path.resolve(ext, "..", "..");
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "icon-vscode-"));
  const iconls = path.join(tmp, "iconls");
  cp.execFileSync("go", ["build", "-o", iconls, "./cmd/iconls"], { cwd: repo, stdio: "inherit" });

  const workspace = path.join(tmp, "project");
  fs.cpSync(path.join(ext, "test", "fixtures", "project"), workspace, { recursive: true });
  fs.mkdirSync(path.join(workspace, ".vscode"));
  fs.writeFileSync(
    path.join(workspace, ".vscode", "settings.json"),
    JSON.stringify({ "icon.server.path": iconls }, null, 2),
  );
  await runTests({
    extensionDevelopmentPath: ext,
    extensionTestsPath: path.join(__dirname, "suite", "index.js"),
    launchArgs: [workspace, "--disable-extensions", "--skip-welcome", "--skip-release-notes"],
  });
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
