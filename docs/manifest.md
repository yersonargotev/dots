# Install Manifest Configuration

`dots.yaml` is the Install Manifest: the reviewable contract that tells `dots`
which Managed Configuration to install, which Dependencies to check, and which
allowlisted Provisioners to run.

Use this page when reviewing a manifest change. It shows both the supported
configuration surface and the configuration currently declared by this repository.

## Supported manifest shape

```yaml
version: 1
profiles: {}
entries: []
provisioners: []
```

| Field | Required | Meaning |
|-------|----------|---------|
| `version` | Yes | Manifest schema version. Only `1` is supported. |
| `profiles` | Yes | Named installation selections. A Profile selects entries and provisioners by matching tags. |
| `entries` | Yes | Managed Entries that install repository-owned files or directories into `$HOME`. |
| `provisioners` | No | Allowlisted external tool invocations run after file entries are installed. |

Unknown YAML fields are rejected during manifest loading.

## Profiles

A Profile is selected by commands such as `dots plan`, `dots install`,
`dots status`, and `dots deps check`. Commands that expose `--tag` can add
optional capability tags on top of the selected Profile without creating another
Profile.

| Field | Required | Supported values |
|-------|----------|------------------|
| `tags` | Yes | Non-empty strings. Entries and Provisioners are selected when at least one tag matches. Optional CLI `--tag` values join this set at runtime. |
| `dependencies` | No | Profile-level Dependencies, using the same dependency fields as entries. |

Current Profiles:

| Profile | Tags | Intent | Profile Dependencies |
|---------|------|--------|----------------------|
| `default` | `core` | Core dotfiles without provisioners. | None |
| `desktop` | `core`, `desktop` | Desktop dotfiles and desktop-only tool integrations; no gentle-ai agent setup. | `Desktop Nerd Font` via Homebrew cask `font-cascadia-code-nf`, detected with `CascadiaCodeNF*` |
| `agents` | `core`, `agents` | Core dotfiles plus gentle-ai agent setup/cleanup and shared engineering skills for supported agents. Add `--tag sdd` to opt into Gentle-AI SDD setup. | None |
| `web` | `core`, `web` | Optional frontend/browser workbench: web design skills plus Chrome DevTools integrations. | None |
| `mobile` | `core`, `mobile` | Optional Dart and Flutter mobile development agent skills. | None |
| `workstation` | `core`, `desktop`, `agents` | Full workstation setup when both desktop integrations and agent setup are desired; web tooling remains explicit opt-in. | `Desktop Nerd Font` via Homebrew cask `font-cascadia-code-nf`, detected with `CascadiaCodeNF*` |

## Managed Entries

A Managed Entry declares one repository source and one target under `$HOME`.
Targets must be `~` or `~/...`; `dots` rejects targets that escape the selected
home directory.

| Field | Required | Supported values |
|-------|----------|------------------|
| `source` | Yes | Repository-relative path to Managed Configuration. |
| `target` | Yes | Home-relative target: `~` or `~/...`. |
| `strategy` | Yes | `symlink`, `copy`, or `template` in the manifest schema. Current install execution supports `symlink` and `copy`. |
| `ownership` | No | Empty, `json-subset`, or `toml-subset`. Subset ownership requires `strategy: copy`. |
| `tags` | Yes | Non-empty strings matched against the selected Profile. |
| `os` | No | `darwin`, `linux`; empty means all supported operating systems. |
| `dependencies` | No | Entry-level Dependencies. |

Current Managed Entries:

| Source | Target | Strategy | Tags | OS | Dependencies |
|--------|--------|----------|------|----|--------------|
| `configs/zsh/zshrc` | `~/.zshrc` | `symlink` | `core` | `darwin`, `linux` | `zsh` |
| `configs/zsh/zimrc` | `~/.zimrc` | `symlink` | `core` | `darwin`, `linux` | `zsh` |
| `configs/zsh/zshenv` | `~/.zshenv` | `symlink` | `core` | `darwin`, `linux` | `zsh` |
| `configs/git/gitconfig` | `~/.gitconfig` | `symlink` | `core` | `darwin`, `linux` | `git` |
| `configs/starship/starship.toml` | `~/.config/starship.toml` | `symlink` | `core` | `darwin`, `linux` | `starship` |
| `configs/tmux/tmux.conf` | `~/.tmux.conf` | `symlink` | `core` | `darwin`, `linux` | `tmux` |
| `configs/zellij/config.kdl` | `~/.config/zellij/config.kdl` | `symlink` | `core` | `darwin`, `linux` | `zellij` |
| `configs/zellij/layouts/default.kdl` | `~/.config/zellij/layouts/default.kdl` | `symlink` | `core` | `darwin`, `linux` | `zellij` |
| `configs/ghostty/config.ghostty` | `~/.config/ghostty/config.ghostty` | `symlink` | `desktop` | `darwin`, `linux` | `ghostty` |
| `configs/warp/settings.toml` | `~/.warp/settings.toml` | `copy` | `desktop` | `darwin` | None |
| `configs/warp/keybindings.yaml` | `~/.warp/keybindings.yaml` | `copy` | `desktop` | `darwin` | None |
| `configs/warp/settings.toml` | `~/.config/warp-terminal/settings.toml` | `copy` | `desktop` | `linux` | `Warp` |
| `configs/warp/keybindings.yaml` | `~/.config/warp-terminal/keybindings.yaml` | `copy` | `desktop` | `linux` | `Warp` |
| `configs/atuin/config.toml` | `~/.config/atuin/config.toml` | `symlink` | `core` | `darwin`, `linux` | `atuin` |
| `configs/atuin/themes/catppuccin-mocha.toml` | `~/.config/atuin/themes/catppuccin-mocha.toml` | `symlink` | `core` | `darwin`, `linux` | `atuin` |
| `configs/bat/config` | `~/.config/bat/config` | `symlink` | `core` | `darwin`, `linux` | `bat` |
| `configs/nvim` | `~/.config/nvim` | `symlink` | `core` | `darwin`, `linux` | `neovim` |
| `configs/zed/settings.json` | `~/.config/zed/settings.json` | `symlink` | `desktop` | `darwin`, `linux` | `zed` |
| `configs/zed/keymap.json` | `~/.config/zed/keymap.json` | `symlink` | `desktop` | `darwin`, `linux` | `zed` |
| `configs/zed/themes/catppuccin-blue.json` | `~/.config/zed/themes/catppuccin-blue.json` | `symlink` | `desktop` | `darwin`, `linux` | `zed` |
| `configs/claude/settings.json` | `~/.claude/settings.json` | `copy` | `core` | `darwin`, `linux` | None; owns JSON subset |
| `configs/claude/statusline-command.sh` | `~/.claude/statusline-command.sh` | `copy` | `core` | `darwin`, `linux` | None |
| `configs/codex/config.toml` | `~/.codex/config.toml` | `copy` | `agents` | `darwin`, `linux` | None; owns TOML subset |
| `configs/copilot/settings.json` | `~/.copilot/settings.json` | `copy` | `agents` | `darwin`, `linux` | None; owns JSON subset |
| `configs/copilot/statusline-command.sh` | `~/.copilot/statusline-command.sh` | `copy` | `agents` | `darwin`, `linux` | None |
| `configs/antigravity/settings.json` | `~/.gemini/antigravity-cli/settings.json` | `copy` | `agents` | `darwin`, `linux` | None; owns JSON subset |
| `configs/opencode/mcp.json` | `~/.config/opencode-dots.json` | `symlink` | `web` | `darwin`, `linux` | `opencode` |

## Dependencies

Dependencies can be declared on Profiles, Managed Entries, and Provisioners.
They are detection and installation guidance, not arbitrary shell hooks.

| Field | Required | Meaning |
|-------|----------|---------|
| `name` | Yes | Human-readable dependency name. Also used as the executable probe when `command` is omitted. |
| `command` | No | Executable name to probe on `PATH`. |
| `brew` | No | Homebrew formula token. Mutually exclusive with `brew_cask`. |
| `brew_cask` | No | Homebrew cask token. Renders as `brew install --cask <token>`. |
| `apt` | No | Debian/Ubuntu package name. |
| `dnf` | No | Fedora package name. |
| `pacman` | No | Arch package name. |
| `font_match` | No | Installed-font filename glob used for Font Dependencies. |
| `font_fallback_matches` | No | Additional filename globs that can satisfy the same Font Dependency. |

Current dependency package coverage:

| Dependency | Detection | macOS/Homebrew | Debian/Ubuntu | Fedora | Arch |
|------------|-----------|----------------|---------------|--------|------|
| `Desktop Nerd Font` | Font glob `CascadiaCodeNF*` | `brew install --cask font-cascadia-code-nf` | Manual | Manual | Manual |
| `zsh` | `zsh` | `zsh` | `zsh` | `zsh` | `zsh` |
| `git` | `git` | `git` | `git` | `git` | `git` |
| `starship` | `starship` | `starship` | `starship` | `starship` | `starship` |
| `tmux` | `tmux` | `tmux` | `tmux` | `tmux` | `tmux` |
| `zellij` | `zellij` | `zellij` | `zellij` | `zellij` | `zellij` |
| `ghostty` | `ghostty` | `ghostty` | Manual | Manual | Manual |
| `Warp` | `warp-terminal` | Manual | Manual | Manual | Manual |
| `atuin` | `atuin` | `atuin` | Manual | Manual | Manual |
| `bat` | `bat` | `bat` | `bat` | `bat` | `bat` |
| `neovim` | `nvim` | `neovim` | `neovim` | `neovim` | `neovim` |
| `zed` | `zed` | `zed` | Manual | Manual | Manual |
| `opencode` | `opencode` | Manual | Manual | Manual | Manual |
| `gentle-ai` | `gentle-ai` | `gentleman-programming/tap/gentle-ai` | Manual | Manual | Manual |
| `engram` | `engram` | `gentleman-programming/tap/engram` | Manual | Manual | Manual |
| `claude` | `claude` | Manual | Manual | Manual | Manual |
| `codex` | `codex` | Manual | Manual | Manual | Manual |
| `curl` | `curl` | `curl` | `curl` | `curl` | `curl` |
| `npx` | `npx` | Manual | Manual | Manual | Manual |

## Provisioners

Provisioners are a closed allowlist. They run after selected Managed Entries are
installed and only after their declared Dependencies are present.

| Field | Required | Supported values |
|-------|----------|------------------|
| `tool` | Yes | `gentle-ai`, `claude`, `codex`, `codegraph`, or `skills`. |
| `tags` | Yes | Non-empty strings matched against the selected Profile. |
| `os` | No | `darwin`, `linux`; empty means all supported operating systems. |
| `spec` | Yes | Tool-specific declaration. Each spec must speak exactly one tool dialect. |
| `dependencies` | No | Dependencies required before running the Provisioner. |

### `gentle-ai` spec

Supported fields: `action`, `scope`, `channel`, `persona`, `preset`, `sdd-mode`,
`agents`, `components`, `skills`, and `yes`.

Constraints:

- `action` may be `install`, `uninstall`, or omitted. Omitted defaults to `install`.
- `persona` may be `gentleman` or `neutral`.
- `yes` is only valid with `action: uninstall`.
- `uninstall` must not set install-only fields: `scope`, `channel`, `persona`,
  `preset`, `sdd-mode`, or `skills`.

### `claude` spec

Supported shapes:

- `marketplace: <source>` renders a marketplace registration.
- `plugin: <name>` plus `from: <marketplace>` renders a plugin install.

Claude specs must not mix gentle-ai fields or Codex MCP fields.

### `codex` spec

Supported fields: `mcp`, `command`, and `env`.

Constraints:

- `mcp` is required.
- `command` must contain at least one non-empty argument.
- `env` keys must not be empty.
- Codex specs must not mix gentle-ai or Claude fields.

### `codegraph` spec

Supported fields: `scope`, `agents`, and `yes`.

`codegraph` renders one fixed shell invocation. It first checks whether
`codegraph` is already on `PATH`; when missing, it runs CodeGraph's official
`curl -fsSL https://raw.githubusercontent.com/colbymchenry/codegraph/main/install.sh | sh`
bootstrap, adds `$HOME/.local/bin` to the child process `PATH`, and then runs
`codegraph install` to wire MCP config plus instructions for the selected
agents.

Constraints:

- `agents` is required and renders the comma-separated `--target` list.
  Supported CodeGraph targets are `codex`, `claude`, `antigravity`, and
  `opencode`.
- `scope`, when set, must be `global` or `local` and renders `--location`.
- `yes` is required and must be `true`; it renders CodeGraph's non-interactive
  `--yes` flag.
- CodeGraph specs must not mix gentle-ai action/channel/persona/preset/sdd,
  Claude, Codex MCP, or skills.sh fields.

### `skills` spec

Supported fields: `package`, `agents`, `skills`, `global`, and `copy`.

`skills` renders one exact `npx --yes skills@1.5.12 add <package>` command.
`package` is an external skills.sh source reference constrained to an `owner/repo`
form with an optional repo path or `@ref`, such as `vercel-labs/agent-skills`.

Constraints:

- `package` is required, must match the allowed package-reference format, and
  must not start with `-` or contain control characters.
- `agents` renders repeated `--agent` flags; values must be data tokens, not
  flag-like strings.
- `skills` renders repeated `--skill` flags; values must be data tokens, not
  flag-like strings.
- `global` is required and must be `true`; local skills installs are not modeled
  yet because they write relative to the process working directory.
- `copy` renders `--copy`.
- Skills specs must not mix gentle-ai scalar/action fields, Claude fields, or
  Codex MCP fields.

Current Provisioners:

| Tool | Tags | OS | Rendered intent | Dependencies |
|------|------|----|-----------------|--------------|
| `gentle-ai` | `agents` | all | Uninstall `sdd` for `codex`, `claude-code`, `opencode`, `antigravity`, and `vscode-copilot` with `--yes`. | `gentle-ai` |
| `gentle-ai` | `sdd` | all | Install global stable neutral custom SDD setup in multi-agent mode for `codex`, `claude-code`, `opencode`, `antigravity`, and `vscode-copilot`. Select with `--profile agents --tag sdd`. | `gentle-ai` |
| `gentle-ai` | `agents` | all | Install global stable neutral custom setup for `codex` with `engram`, `context7`, and `persona`. | `gentle-ai`, `engram` |
| `gentle-ai` | `agents` | all | Install global stable neutral custom setup for `claude-code` with `engram`, `context7`, `persona`, and `permissions`. | `gentle-ai`, `engram` |
| `gentle-ai` | `agents` | all | Install global stable neutral custom setup for `antigravity` with `engram`, `context7`, and `persona` only; no `sdd` or `permissions`. | `gentle-ai`, `engram` |
| `gentle-ai` | `agents` | all | Install global stable neutral custom setup for `opencode` with `engram`, `context7`, and `persona` only; no `sdd` or `permissions`. | `gentle-ai`, `engram` |
| `gentle-ai` | `agents` | all | Install global stable neutral custom setup for `vscode-copilot` with `engram`, `context7`, and `persona` only; no `sdd` or `permissions`. | `gentle-ai`, `engram` |
| `skills` | `web` | all | Install `playwright-cli` from `microsoft/playwright-cli` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`, copying the skill and references into the agent skill roots. | `npx` |
| `skills` | `web` | all | Install `frontend-design` from `anthropics/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `web` | all | Install `vercel-react-best-practices`, `vercel-composition-patterns`, `vercel-react-view-transitions`, and `web-design-guidelines` from `vercel-labs/agent-skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `mobile` | all | Install the upstream Dart skill package from `dart-lang/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `mobile` | all | Install the upstream Flutter skill package from `flutter/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `mobile` | all | Install `android-cli` from `android/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `agents` | all | Install the reviewed Matt Pocock engineering skill set from `mattpocock/skills/skills/engineering` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `agents` | all | Install `review` and `writing-great-skills` from `mattpocock/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`, copying the skills into agent skill roots. | `npx` |
| `claude` | `web` | `darwin`, `linux` | Register marketplace `ChromeDevTools/chrome-devtools-mcp`. | `claude` |
| `claude` | `web` | `darwin`, `linux` | Install `chrome-devtools-mcp` from `chrome-devtools-plugins` with user scope. | `claude` |
| `codex` | `web` | `darwin`, `linux` | Add MCP server `chrome-devtools` using `npx -y chrome-devtools-mcp@latest --no-performance-crux`. | `codex` |
| `codegraph` | `codegraph` | `darwin`, `linux` | Reuse `codegraph` when already on `PATH`; otherwise install it with the official curl bootstrap, then run `codegraph install --target codex,claude,antigravity,opencode --location global --yes` so CodeGraph configures MCP plus instructions for Codex, Claude Code, Antigravity, and OpenCode. Select with `--tag codegraph`. | `curl` |

## Selection rules

1. The chosen Profile provides the base tags.
2. Each repeated `--tag` adds an optional tag to that selection. Duplicate tags are ignored.
3. A Managed Entry or Provisioner is selected when any of its tags matches the effective tag set.
4. If `os` is empty, the item matches all supported operating systems.
5. If `os` is set, the item only matches the current OS (`darwin` or `linux`).
6. Selected Managed Entries are installed before selected Provisioners run.

## Related docs

- [`CONTEXT.md`](../CONTEXT.md) defines the domain vocabulary.
- [`docs/scope.md`](scope.md) explains why the MVP Configuration Set is intentionally bounded.
- [`docs/adr/0003-claude-plugin-provisioner.md`](adr/0003-claude-plugin-provisioner.md) records the Claude plugin provisioner decision.
- [`docs/adr/0004-codex-mcp-provisioner.md`](adr/0004-codex-mcp-provisioner.md) records the Codex MCP provisioner decision.
- [`docs/adr/0005-opencode-mcp-config-overlay.md`](adr/0005-opencode-mcp-config-overlay.md) records the OpenCode config overlay decision.
- [`docs/adr/0007-hybrid-skill-provisioning.md`](adr/0007-hybrid-skill-provisioning.md) records the hybrid repo-owned/external skill ownership decision.
