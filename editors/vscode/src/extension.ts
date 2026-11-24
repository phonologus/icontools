// VS Code extension for Icon: starts the iconls language server.
import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;
let running: string | undefined; // the settings the client was started with

// Starts and stops are serialised: a configuration change during startup
// must not stop a client that is still starting.
let queue: Promise<void> = Promise.resolve();
function serialise(f: () => Promise<void>): Promise<void> {
  queue = queue.then(f, f);
  return queue;
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  context.subscriptions.push(
    vscode.commands.registerCommand("icon.restartServer", () =>
      serialise(async () => {
        await stop();
        await start();
      }),
    ),
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (!e.affectsConfiguration("icon")) return;
      // Restart only if the server's settings actually changed (VS Code
      // also reports changes while loading the workspace settings).
      return serialise(async () => {
        if (settingsKey() !== running) {
          await stop();
          await start();
        }
      });
    }),
  );
  await serialise(start);
}

export function deactivate(): Promise<void> {
  return serialise(stop);
}

function settings(): { command: string; ipath: string[] } {
  const config = vscode.workspace.getConfiguration("icon");
  return {
    command: config.get<string>("server.path") || "iconls",
    ipath: config.get<string[]>("ipath") ?? [],
  };
}

function settingsKey(): string {
  return JSON.stringify(settings());
}

async function start(): Promise<void> {
  const { command, ipath } = settings();
  running = settingsKey();
  // No transport is given, so the client talks over stdio without adding a
  // --stdio argument (which iconls accepts anyway).
  const serverOptions: ServerOptions = { run: { command }, debug: { command } };
  const clientOptions: LanguageClientOptions = {
    documentSelector: [
      { scheme: "file", language: "icon" },
      { scheme: "untitled", language: "icon" },
    ],
    initializationOptions: { ipath },
  };
  const c = new LanguageClient("iconls", "Icon Language Server", serverOptions, clientOptions);
  try {
    await c.start();
    client = c;
  } catch (err) {
    const install = "How to install";
    vscode.window
      .showErrorMessage(
        `Could not start the Icon language server (${command}). ` +
          "Install it with `go install github.com/phonologus/icontools/cmd/iconls@latest`, " +
          "or set icon.server.path.",
        install,
      )
      .then((choice) => {
        if (choice === install) {
          vscode.env.openExternal(vscode.Uri.parse("https://github.com/phonologus/icontools#using-iconls"));
        }
      });
  }
}

async function stop(): Promise<void> {
  const c = client;
  client = undefined;
  running = undefined;
  if (c) {
    try {
      await c.stop();
    } catch {
      // Already stopped (e.g. the server exited).
    }
  }
}
