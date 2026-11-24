//! Zed extension for Icon: starts the iconls language server.
//!
//! iconls is found on the PATH of the project's shell environment, unless
//! Zed's settings give its path:
//!
//! ```json
//! "lsp": {
//!   "iconls": {
//!     "binary": { "path": "/path/to/iconls" },
//!     "initialization_options": { "ipath": ["/path/to/ipl/procs"] }
//!   }
//! }
//! ```

use zed_extension_api::{self as zed, settings::LspSettings, LanguageServerId, Result};

struct IconExtension;

impl zed::Extension for IconExtension {
    fn new() -> Self {
        IconExtension
    }

    fn language_server_command(
        &mut self,
        _language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        let binary = LspSettings::for_worktree("iconls", worktree)
            .ok()
            .and_then(|s| s.binary);
        let path = binary
            .as_ref()
            .and_then(|b| b.path.clone())
            .or_else(|| worktree.which("iconls"))
            .ok_or_else(|| {
                "iconls not found on PATH: install it with \
                 `go install github.com/phonologus/icontools/cmd/iconls@latest`, \
                 or set lsp.iconls.binary.path in Zed's settings"
                    .to_string()
            })?;
        let args = binary
            .as_ref()
            .and_then(|b| b.arguments.clone())
            .unwrap_or_default();
        let mut env = worktree.shell_env();
        if let Some(extra) = binary.and_then(|b| b.env) {
            env.extend(extra);
        }
        Ok(zed::Command {
            command: path,
            args,
            env,
        })
    }

    fn language_server_initialization_options(
        &mut self,
        _language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<Option<zed::serde_json::Value>> {
        Ok(LspSettings::for_worktree("iconls", worktree)
            .ok()
            .and_then(|s| s.initialization_options))
    }
}

zed::register_extension!(IconExtension);
