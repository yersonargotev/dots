# Install Dart, Flutter, and Android CLI agent skills through an explicit mobile profile

The user wants Dart, Flutter, and Android CLI agent skills available on demand without making mobile development part of the default, desktop, web, or broad agent setup. We add a dedicated `mobile` Profile/tag for the mobile workbench and attach three skills.sh provisioners: `dart-lang/skills`, `flutter/skills`, and `android/skills --skill android-cli`.

Later requirements for this profile also need Dart/Flutter MCP server wiring for supported agent surfaces, so the `mobile` tag now also selects:

- `claude` stdio MCP registration for `dart` via `dart mcp-server`,
- `codex` `mcp add dart -- dart mcp-server --force-roots-fallback`,
- Antigravity JSON subset config for the Dart MCP server,
- VS Code Copilot MCP enablement via `dart.mcpServer`.

This keeps profile names aligned with user intent. Choosing `--profile mobile` means “prepare this machine/session for Dart, Flutter, and Android app work”. Choosing `--profile web` remains focused on frontend/browser tooling, choosing `--profile agents` remains focused on general agent setup, and choosing `--profile desktop` remains focused on terminal/editor/GUI machine configuration.

**Considered Options**: Adding Dart, Flutter, and Android CLI skills to `agents` was rejected because it would make mobile-specific guidance part of every agent setup. Adding them to `web` was rejected because Flutter can target the web, but the requested capability is primarily mobile app development. Adding Flutter, Dart, or Android SDK package-manager dependencies was rejected for now because the request only asked to install the upstream skills; SDK installation can be added later if the profile is expanded from agent guidance into full mobile runtime/toolchain setup.

**Consequences**: Manifest profile selection now has a `mobile` tag. The mobile profile installs all upstream skills from `dart-lang/skills` and `flutter/skills`, plus `android-cli` from `android/skills`, globally for Codex, Claude Code, and Antigravity through the pinned skills.sh CLI. It also provisions Dart/Flutter MCP for Claude, Codex, Antigravity, and VS Code to remove manual setup from fresh installs. Antigravity and VS Code use `json-subset` Managed Entries, so pre-existing unowned settings files remain conflicts until replace or adopt is explicitly selected. Documentation and repository manifest tests treat Dart, Flutter, Android CLI skills, and Dart/Flutter MCP enablement as mobile-scoped from this point forward. Initial implementation is tracked in [#173](https://github.com/yersonargotev/dots/issues/173); the Android CLI extension is tracked in [#151](https://github.com/yersonargotev/dots/issues/151).

**Ubuntu amendment**: Issue [#217](https://github.com/yersonargotev/dots/issues/217) clarified that the mobile profile is not complete on Ubuntu when `dart` is absent: the Claude and Codex Dart MCP provisioners cannot run, and there is no reviewed distro package mapping in the manifest. The profile still does not auto-install Flutter or the Android SDK, but the `dart` Dependency now carries Ubuntu-specific manual guidance that points to Flutter SDK installation, adding the SDK `bin` directory to `PATH`, verifying `dart --version`, and rerunning `dots install --profile mobile`. `dots doctor` also probes `claude --version` when Claude is on `PATH`, so a broken Claude Code native binary install is detected before mobile MCP registration fails.

**Superseded in part by ADR 0018**: the Mobile capability boundary and Ubuntu
guidance remain valid, but its Profile is now an ordered preset over atomic
skills and agent-integration Tags. The broad `mobile` Tag is transitional
compatibility only.
