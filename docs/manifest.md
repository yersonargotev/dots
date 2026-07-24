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
dependencies: []
entries: []
provisioners: []
```

| Field | Required | Meaning |
|-------|----------|---------|
| `version` | Yes | Manifest schema version. Only `1` is supported. |
| `profiles` | Yes | Named installation selections. A Profile selects entries and provisioners by matching tags. |
| `dependencies` | No | Tag-scoped Dependency Sets for shared toolchain baselines selected before Managed Entry and Provisioner Dependencies. |
| `entries` | Yes | Managed Entries that install repository-owned files or directories into `$HOME`. |
| `provisioners` | No | Allowlisted external tool invocations run after file entries are installed. |

Unknown YAML fields are rejected during manifest loading.

## Profiles

A Profile is selected explicitly during install. Read-only selection-aware
commands (`status`, `doctor`, `plan`, `deps check`, and `deps plan`) reuse the
authoritative Installed Selection when both `--profile` and `--tag` are omitted.
Any explicit selection flag makes the current invocation's complete selection
win without rewriting Installation Metadata. Repeat `--profile` to compose
multiple Profiles by the ordered union of their tags. Commands that expose
`--tag` can add optional capability tags on top of the selected Profile set
without creating another Profile. For example, `--tag adaptive-theme` installs the opt-in marker and
app-specific fragments used by managed configs to follow macOS light appearance
where the app has a safe seam.

When v1/v2 Installation Metadata has no authoritative Installed Selection,
these read-only commands return `selection-migration-required` rather than
silently consuming historical Profile or Tag evidence. Inspect the
non-authoritative candidate with `dots installed`, then run one complete
explicit `--profile`/`--tag` selection; an interactive `update` or `upgrade`
may instead offer an unambiguous candidate for confirmation.

| Field | Required | Supported values |
|-------|----------|------------------|
| `tags` | Yes | Non-empty strings. Entries and Provisioners are selected when at least one tag matches. Optional CLI `--tag` values join this set at runtime. |
| `dependencies` | No | Profile-level Dependencies, using the same dependency fields as entries. |

Current Profiles:

| Profile | Tags | Intent | Profile Dependencies |
|---------|------|--------|----------------------|
| `core` | `core` | Core dotfiles without agent/web/mobile provisioners. | Core Development Baseline via the `core` tag-scoped Dependency Set |
| `desktop` | `desktop` | Desktop-only configuration and integrations; compose with `--profile core` when core dotfiles are desired too. | `Desktop Nerd Font` via Homebrew cask `font-cascadia-code-nf`, detected with `CascadiaCodeNF*` or `CaskaydiaCoveNerdFont*` |
| `agents` | `agents` | gentle-ai memory/context setup, cleanup, shared engineering skills, the dots-owned `delegation` skill, and compact dots-owned global rules for supported agents. Add `--profile core` separately for core dotfiles. | `GitHub CLI` via the `agents` tag-scoped Dependency Set |
| `codex-delegation` | `codex-delegation` | Codex-only delegation skill, generic delegation overlay, and dots-owned native explorer/worker agents without the broader agent baseline. | `npx` via the selected Provisioner Dependency |
| `web` | `web` | Optional frontend/browser workbench: web design skills plus Chrome DevTools integrations. | `Playwright CLI` via Homebrew formula `playwright-cli` |
| `mobile` | `mobile` | Optional Dart and Flutter mobile development agent skills plus Dart/Flutter MCP integration for Claude, Codex, Antigravity, and GitHub Copilot in VS Code. | — |
| `workstation` | `core`, `desktop`, `agents` | Opinionated composite when core, desktop integrations, and agent setup are desired; web and mobile remain explicit opt-ins. | Core Development Baseline via the `core` tag-scoped Dependency Set; `GitHub CLI` via the `agents` tag-scoped Dependency Set; `Desktop Nerd Font` via Homebrew cask `font-cascadia-code-nf`, detected with `CascadiaCodeNF*` or `CaskaydiaCoveNerdFont*` |

## Tag-scoped Dependency Sets

Top-level `dependencies` declare shared toolchain baselines selected by tag,
before Managed Entry and Provisioner Dependencies. The current `core` set is the
Core Development Baseline from ADR 0010: shell/terminal foundations, shared
terminal tools, and common development runtimes are owned by the `core`
tag-scoped Dependency Set when they are useful across shell, editor, Git, or CLI
workflows, even when a tool has no Managed Entry of its own. This includes
runtime/package tools such as `fnm`, `rustup`, `go`, `uv`, `pnpm`, and `bun`,
plus terminal/developer tools such as `fzf`, `zoxide`, `lazygit`, `eza`,
`ripgrep`, `delta`, `unzip`, and `fd`. The `agents` set adds `GitHub CLI` (`gh`)
for issue and PR automation. Linux Homebrew fallback is explicit with
`linux_homebrew: true`; GUI apps and fonts stay manual unless they have
validated Linux support. Node and Rust use constrained built-in toolchain
bootstrap commands after manager installation:
`fnm install --lts` for Node LTS and `rustup default stable` for Rust stable.
Bootstrap declarations do not make an action executable by themselves: the
manager executable must already be on `PATH` or a concrete installer/provider
must run first.

On Linux, when `fnm` is absent, no executable package provider is available, and
`curl`, `bash`, and `unzip` are present, Node may use the constrained official
fnm installer flow (`curl -fsSL https://fnm.vercel.app/install | bash -s --
--skip-shell`) before running `fnm install --lts`. The `--skip-shell` flag is
required because dots owns shell activation through the managed zsh integration,
not the upstream installer.

On Linux, when `rustup` is absent but `curl` and `sh` are available, the Rust
toolchain may use the constrained official rustup installer flow (`curl --proto
'=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path`)
before running `rustup default stable`. These are built-in runtime actions, not
arbitrary shell from the manifest. When Rust bootstrap succeeds but `rustc` or
`cargo` still are not available on `PATH`, the install result includes repair
guidance for the rustup proxy/PATH surface instead of only reporting a generic
unresolved Dependency.

## Managed Entries

### Adaptive theme tag

`adaptive-theme` is an explicit opt-in tag, not part of any default Profile. It
installs `~/.config/dots/adaptive-theme` plus app-specific fragments such as
Ghostty's optional `adaptive-theme.ghostty`. Shell-readable configs source
`~/.config/dots/theme.sh` and select Catppuccin Latte only when both the marker
exists and macOS light appearance is proven. Missing marker, macOS dark mode,
Linux, missing appearance APIs, and unknown values keep Mocha/dark fallbacks.

Static apps without a safe optional include seam can use a `source_overrides`
entry when a whole target must switch sources behind the opt-in. The override
keeps one Managed Entry/target in the Install Plan while selecting the tagged
source, so `--tag adaptive-theme` does not create duplicate target actions.
When none of an entry's override tags is selected, dots uses its base `source`;
an override does not change the entry's `tags` selection or `os` applicability.
Override keys match selected tags exactly. When a conflicting target exactly
matches an alternate source whose tag was not selected, diagnostics retain the
Conflict and report all such unselected tags in deterministic order. Omit
explicit `--profile` and `--tag` flags to reuse an available Installed
Selection, or pass each intended exact tag with repeated `--tag <tag>`.
Co-owned files are still documented in [`docs/adaptive-theme-audit.md`](adaptive-theme-audit.md)
when no safe dots-owned source can be selected.


A Managed Entry declares one repository source and one target under `$HOME`.
Targets must be `~` or `~/...`; `dots` rejects targets that escape the selected
home directory.

| Field | Required | Supported values |
|-------|----------|------------------|
| `source` | Yes | Repository-relative path to Managed Configuration. |
| `source_overrides` | No | Map of exact selected tag to alternate repository-relative source for the same target. The base `source` is used when no key matches; overrides do not alter entry tag selection or OS applicability. |
| `target` | Yes | Home-relative target: `~` or `~/...`. |
| `strategy` | Yes | `symlink`, `copy`, or `template` in the manifest schema. Current install execution supports `symlink` and `copy`. |
| `ownership` | No | Empty, `json-subset`, or `toml-subset`. Subset ownership requires `strategy: copy`. |
| `tags` | Yes | Non-empty strings matched against the selected Profile. |
| `os` | No | `darwin`, `linux`; empty means all supported operating systems. |
| `dependencies` | No | Entry-level Dependencies. |

For a `json-subset` target already trusted by matching Installation Metadata,
missing source-owned object keys and array elements are applied as a conservative
update: target-only values are preserved, a Backup Set is created, and existing
incompatible scalar or object/array values remain a Conflict. A compatible
pre-existing target without matching metadata is still a Conflict.

Current Managed Entries:

| Source | Target | Strategy | Tags | OS | Dependencies |
|--------|--------|----------|------|----|--------------|
| `configs/zsh/zshrc` | `~/.zshrc` | `symlink` | `core` | `darwin`, `linux` | `zsh` |
| `configs/zsh/zimrc` | `~/.zimrc` | `symlink` | `core` | `darwin`, `linux` | `zsh` |
| `configs/zsh/zshenv` | `~/.zshenv` | `symlink` | `core` | `darwin`, `linux` | `zsh` |
| `configs/git/gitconfig` | `~/.gitconfig` | `symlink` | `core` | `darwin`, `linux` | `git` |
| `configs/dots/theme.sh` | `~/.config/dots/theme.sh` | `symlink` | `core` | `darwin`, `linux` | None |
| `configs/dots/adaptive-theme` | `~/.config/dots/adaptive-theme` | `symlink` | `adaptive-theme` | `darwin`, `linux` | None |
| `configs/starship/starship.toml` | `~/.config/starship.toml` | `symlink` | `core` | `darwin`, `linux` | `starship` |
| `configs/tmux/tmux.conf` | `~/.tmux.conf` | `symlink` | `core` | `darwin`, `linux` | `tmux` |
| `configs/herdr/config.toml` (`adaptive-theme` override: `configs/herdr/config-adaptive.toml`) | `~/.config/herdr/config.toml` | `symlink` | `core` | `darwin` | `herdr` |
| `configs/zellij/config.kdl` (`adaptive-theme` override: `configs/zellij/config-adaptive.kdl`) | `~/.config/zellij/config.kdl` | `symlink` | `core` | `darwin`, `linux` | `zellij` |
| `configs/zellij/layouts/default.kdl` | `~/.config/zellij/layouts/default.kdl` | `symlink` | `core` | `darwin`, `linux` | `zellij` |
| `configs/ghostty/config.ghostty` | `~/.config/ghostty/config.ghostty` | `symlink` | `desktop` | `darwin`, `linux` | `ghostty` |
| `configs/ghostty/adaptive/adaptive-theme.ghostty` | `~/.config/ghostty/adaptive-theme.ghostty` | `symlink` | `adaptive-theme` | `darwin` | None |
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
| `configs/codex/config.toml` (`codegraph` override: `configs/codex/config-codegraph.toml`) | `~/.codex/config.toml` | `copy` | `agents` | `darwin`, `linux` | None; owns TOML subset; CodeGraph tag adds a Codex SessionStart hook |
| `configs/copilot/settings.json` | `~/.copilot/settings.json` | `copy` | `agents` | `darwin`, `linux` | None; owns JSON subset |
| `configs/copilot/statusline-command.sh` | `~/.copilot/statusline-command.sh` | `copy` | `agents` | `darwin`, `linux` | None |
| `configs/antigravity/settings.json` | `~/.gemini/antigravity-cli/settings.json` | `copy` | `agents` | `darwin`, `linux` | None; owns the broad Antigravity JSON baseline |
| `configs/antigravity/mobile-mcp-settings.json` | `~/.gemini/antigravity-cli/settings.json` | `copy` | `mobile` | `darwin`, `linux` | None; owns only the Dart/Flutter MCP JSON subset |
| `configs/vscode/settings.json` | `~/Library/Application Support/Code/User/settings.json` | `copy` | `mobile` | `darwin` | None; owns JSON subset enabling Dart MCP for GitHub Copilot in VS Code |
| `configs/vscode/settings.json` | `~/.config/Code/User/settings.json` | `copy` | `mobile` | `linux` | None; owns JSON subset enabling Dart MCP for GitHub Copilot in VS Code |
| `configs/opencode/mcp.json` | `~/.config/opencode-dots.json` | `symlink` | `web` | `darwin`, `linux` | `opencode` |

## Dependencies

Dependencies can be declared in tag-scoped Dependency Sets, Profiles, Managed
Entries, and Provisioners. They are detection and installation guidance, not
arbitrary shell hooks.

Tag-scoped Dependency Sets use:

| Field | Required | Meaning |
|-------|----------|---------|
| `tags` | Yes | Non-empty strings. The set is selected when at least one tag matches the Profile or an explicit `--tag`. |
| `os` | No | `darwin`, `linux`; empty means all supported operating systems. |
| `dependencies` | Yes | One or more Dependency declarations. |

| Field | Required | Meaning |
|-------|----------|---------|
| `name` | Yes | Human-readable dependency name. Also used as the executable probe when `command` is omitted. |
| `requirement` | No | `required` or `optional`. Omitted means `required`. Required Dependencies gate integrated install; optional Dependencies are reported but do not block Managed Configuration. |
| `command` | No | Single executable name to probe on `PATH`. |
| `manual` | No | Dependency-specific remediation for manual-only tools. Use it when generic package-manager guidance would hide required user choices, privileges, or verification steps. |
| `manual_debian` | No | Debian/Ubuntu-specific manual remediation. Prefer this over `manual` when instructions mention Debian/Ubuntu-only packages, commands, privileges, or verification. |
| `commands` | No | Multiple executable names that must all be present. Use this for manager-owned toolchains where both the manager and runtime commands matter. |
| `toolchain` | No | Built-in runtime bootstrap flow. Supported values: `node-lts-fnm`, `rust-stable-rustup`. These values map to fixed argv commands, not arbitrary shell hooks. |
| `brew` | No | Homebrew formula token. Mutually exclusive with `brew_cask`. |
| `linux_homebrew` | No | Allows Linux distro tiers to fall back to the Homebrew formula when the distro provider is absent or unavailable. Keep this opt-in for reviewed CLI/runtime tools only. |
| `user_local` | No | Linux-only opt-in for a reviewed User-Local Provider recipe. Requires `recipe`, `version`, and `checksum` or per-platform `checksums`; installs into `~/.local/bin` or `~/.local/opt/<tool>` without using system package managers. |
| `brew_cask` | No | Homebrew cask token. Renders as `brew install --cask <token>`. |
| `apt` | No | Debian/Ubuntu package name. |
| `dnf` | No | Fedora package name. |
| `pacman` | No | Arch package name. |
| `font_match` | No | Installed-font filename glob used for Font Dependencies. |
| `font_fallback_matches` | No | Additional filename globs that can satisfy the same Font Dependency. |


User-Local Providers are not system packages. They are reviewed, allowlisted Go recipes that write only to the selected home-owned environment and require explicit manifest policy. `dots` does not mutate shell startup files or `PATH`; after installation it re-runs normal Dependency probes, so `~/.local/bin` must already be visible in the current `PATH` for the Dependency to become present. Invalid `user_local` policy such as an unknown recipe, missing version, unsupported platform, or missing checksum is a manifest/planning error, not a silent fallback to Linuxbrew or manual guidance.

`pnpm` is intentionally installed from the pinned standalone `@pnpm/linux-*` executable artifacts instead of through Corepack. Node remains part of the Core Development Baseline through `Node LTS (fnm)`, but the pnpm provider does not enable Corepack, create Corepack shims, run global `npm install -g`, or mutate shell configuration.

`neovim` uses the upstream multi-file Linux tarball through its User-Local Provider. The bundle is extracted under `~/.local/opt/nvim/<version>` and `~/.local/bin/nvim` points at the bundled executable.

`gentle-ai` and `engram` use pinned upstream GitHub release tarballs through their User-Local Providers. They are Dependency providers only: they make the reviewed executables available for later probes, while Provisioners still own agent configuration changes. These providers do not run `npm install -g`, use `npx`, require `sudo`, or execute arbitrary package scripts.

Current dependency package coverage:

| Dependency | Detection | macOS/Homebrew | Debian/Ubuntu | Fedora | Arch |
|------------|-----------|----------------|---------------|--------|------|
| `Desktop Nerd Font` | Font globs `CascadiaCodeNF*` or `CaskaydiaCoveNerdFont*` | `brew install --cask font-cascadia-code-nf` | Manual | Manual | Manual |
| `zsh` | `zsh` | `zsh` | `zsh` | `zsh` | `zsh` |
| `git` | `git` | `git` | `git` | `git` | `git` |
| `starship` | `starship` | `starship` | User-local / Linuxbrew opt-in/manual | `starship` | `starship` |
| `tmux` | `tmux` | `tmux` | `tmux` | `tmux` | `tmux` |
| `zellij` | `zellij` | `zellij` | User-local / Linuxbrew opt-in/manual | `zellij` | `zellij` |
| `Node LTS (fnm)` | `fnm`, `node`; bootstrap `fnm install --lts` | `fnm` | Linuxbrew opt-in or official fnm installer (`curl`, `bash`, `unzip`) | Linuxbrew opt-in or official fnm installer (`curl`, `bash`, `unzip`) | Linuxbrew opt-in or official fnm installer (`curl`, `bash`, `unzip`) |
| `Rust stable (rustup)` | `rustup`, `rustc`, `cargo`; bootstrap `rustup default stable` | `rustup` | Linuxbrew opt-in/manual | Linuxbrew opt-in/manual | Linuxbrew opt-in/manual |
| `go` | `go` | `go` | Linuxbrew opt-in/manual | Linuxbrew opt-in/manual | Linuxbrew opt-in/manual |
| `uv` | `uv` | `uv` | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual |
| `pnpm` | `pnpm` | `pnpm` | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual |
| `bun` | `bun` | `bun` | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual |
| `fzf` | `fzf` | `fzf` | `fzf` | `fzf` | `fzf` |
| `zoxide` | `zoxide` | `zoxide` | `zoxide` | `zoxide` | `zoxide` |
| `lazygit` | `lazygit` | `lazygit` | Linuxbrew opt-in/manual | `lazygit` | `lazygit` |
| `eza` | `eza` | `eza` | `eza` | `eza` | `eza` |
| `ripgrep` | `rg` | `ripgrep` | `ripgrep` | `ripgrep` | `ripgrep` |
| `delta` | `delta` | `git-delta` | Linuxbrew opt-in/manual | `git-delta` | `git-delta` |
| `unzip` | `unzip` | `unzip` | `unzip` | `unzip` | `unzip` |
| `fd` | `fd` | `fd` | Linuxbrew opt-in/manual | `fd-find` | `fd` |
| `GitHub CLI` | `gh` | `gh` | Linuxbrew opt-in/manual | Linuxbrew opt-in/manual | Linuxbrew opt-in/manual |
| `Playwright CLI` | `playwright-cli` | `playwright-cli` | Linuxbrew opt-in/manual | Linuxbrew opt-in/manual | Linuxbrew opt-in/manual |
| `ghostty` | `ghostty` | `ghostty` | Manual; Ubuntu guidance includes Ghostty upstream binary docs, the community `.deb` installer command, and notes `snap install ghostty --classic` requires sudo/password interactivity | Manual | Manual |
| `Warp` | `warp-terminal` | Manual | Manual | Manual | Manual |
| `atuin` | `atuin` | `atuin` | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual |
| `bat` | `bat` | `bat` | User-local / Linuxbrew opt-in/manual | `bat` | `bat` |
| `neovim` | `nvim` | `neovim` | User-local / Linuxbrew opt-in/manual | `neovim` | `neovim` |
| `zed` | `zed` | `zed` | Manual | Manual | Manual |
| `opencode` | `opencode` | Manual | Manual | Manual | Manual |
| `gentle-ai` | `gentle-ai` | `gentleman-programming/tap/gentle-ai` | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual |
| `engram` | `engram` | `gentleman-programming/tap/engram` | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual |
| `claude` | `claude` | Manual | Manual | Manual | Manual |
| `codex` | `codex` | Manual | Manual | Manual | Manual |
| `dart` | `dart` | Manual | Manual; Ubuntu guidance points to Flutter SDK installation because `dart mcp-server` ships with Dart/Flutter tooling, then asks users to verify `dart --version` | Manual | Manual |
| `curl` | `curl` | `curl` | `curl` | `curl` | `curl` |
| `npx` | `npx` | Manual | Manual | Manual | Manual |

Reviewed Linuxbrew opt-ins include CLI/runtime tools whose Homebrew formulas are
expected to work on Linuxbrew: Starship, the core runtimes/package tools, GitHub CLI,
Playwright CLI, lazygit, delta, fd, atuin, gentle-ai, and engram. On Linux, gentle-ai and engram also
have reviewed User-Local Providers from pinned release tarballs, so Linuxbrew is
only a fallback when that opt-in is absent or unavailable. Brew-only GUI apps such
as Ghostty and Zed remain manual on Linux.

Homebrew dependencies declared with a fully-qualified tap formula such as
`gentleman-programming/tap/gentle-ai` may require explicit Homebrew Tap Trust on
fresh macOS machines. `dots deps plan` and `dots deps install --dry-run` surface
the formula-level trust command, for example
`brew trust --formula gentleman-programming/tap/gentle-ai`, but `dots` does not
run trust commands automatically.

## Provisioners

Provisioners are a closed allowlist. They run after selected Managed Entries are
installed and only after their declared Dependencies are present. During execution,
`dots` threads the selected `--home`, sets `NPM_CONFIG_PREFIX` to `~/.local`, and
puts `~/.local/bin` first on `PATH` so npm-backed provisioners can install and
reuse user-local tools without requiring sudo in non-interactive runs.

| Field | Required | Supported values |
|-------|----------|------------------|
| `tool` | Yes | `gentle-ai`, `claude`, `codex`, `codegraph`, `skills`, or `zimfw`. |
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
- `mcp: <name>` plus `command: [...]` renders a stdio MCP server registration.

Claude specs must not mix gentle-ai fields, skills fields, or multiple Claude shapes. `env` remains Codex-only.

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
agents. CodeGraph's installer owns the generated MCP/instruction setup for
those targets; dots only adds its scoped `<!-- dots:codegraph-mode -->` policy
overlay for agent instruction files it manages, so generic CodeGraph usage text
stays with CodeGraph.

Constraints:

- `agents` is required and renders the comma-separated `--target` list.
  Supported CodeGraph targets are `codex`, `claude`, `antigravity`, and
  `opencode`.
- `scope`, when set, must be `global` or `local` and renders `--location`.
- `yes` is required and must be `true`; it renders CodeGraph's non-interactive
  `--yes` flag.
- CodeGraph specs must not mix gentle-ai action/channel/persona/preset/sdd,
  Claude plugin fields, MCP fields, or skills.sh fields.

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

### `zimfw` spec

Supported fields: `yes`.

`zimfw` renders one fixed non-interactive `zsh -c` invocation. It ensures
`~/.zim/zimfw.zsh` exists by downloading the latest ZimFW runtime when missing,
then runs `zimfw init -q` against the dots-managed `~/.zimrc`. The generated
runtime stays under `~/.zim/`; `~/.zimrc` remains the managed Source of Truth.

Constraints:

- `yes` is required and must be `true`.
- ZimFW specs must not mix gentle-ai fields, Claude fields, MCP fields, or
  skills.sh fields.

Current Provisioners:

| Tool | Tags | OS | Rendered intent | Dependencies |
|------|------|----|-----------------|--------------|
| `zimfw` | `core` | all | Install the ZimFW runtime under `~/.zim` when missing and run `zimfw init -q` using the dots-managed `~/.zimrc`. | `zsh`, `git`, `curl` |
| `gentle-ai` | `agents` | all | Uninstall `sdd` and `persona` for `codex`, `claude-code`, `opencode`, `antigravity`, and `vscode-copilot` with `--yes`. | `gentle-ai` |
| `gentle-ai` | `sdd` | all | Install global stable custom SDD setup in multi-agent mode for `codex`, `claude-code`, `opencode`, `antigravity`, and `vscode-copilot`, without installing persona. Select with `--profile agents --tag sdd`. | `gentle-ai` |
| `gentle-ai` | `agents` | all | Install global stable custom baseline for `codex` with `engram` and `context7`; persona prompt Regenerated Content is not installed by default. | `gentle-ai`, `engram` |
| `gentle-ai` | `agents` | all | Install global stable custom baseline for `claude-code` with `engram`, `context7`, and `permissions`; persona prompt Regenerated Content is not installed by default. | `gentle-ai`, `engram` |
| `gentle-ai` | `agents` | all | Install global stable custom baseline for `antigravity` with `engram` and `context7` only; no `sdd`, `permissions`, or persona prompt Regenerated Content. | `gentle-ai`, `engram` |
| `gentle-ai` | `agents` | all | Install global stable custom baseline for `opencode` with `engram` and `context7` only; no `sdd`, `permissions`, or persona prompt Regenerated Content. | `gentle-ai`, `engram` |
| `gentle-ai` | `agents` | all | Install global stable custom baseline for `vscode-copilot` with `engram` and `context7` only; no `sdd`, `permissions`, or persona prompt Regenerated Content. | `gentle-ai`, `engram` |
| `skills` | `web` | all | Install `playwright-cli` from `microsoft/playwright-cli` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`, copying the skill and references into the agent skill roots. | `npx` |
| `skills` | `web` | all | Install `frontend-design` from `anthropics/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `web` | all | Install `vercel-react-best-practices`, `vercel-composition-patterns`, `vercel-react-view-transitions`, and `web-design-guidelines` from `vercel-labs/agent-skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `mobile` | all | Install the upstream Dart skill package from `dart-lang/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `mobile` | all | Install the upstream Flutter skill package from `flutter/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `mobile` | all | Install `android-cli` from `android/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `claude` | `mobile` | `darwin`, `linux` | Add the Dart and Flutter MCP server using `claude mcp add --transport stdio dart -- dart mcp-server`; on Ubuntu, missing Dart points to Flutter SDK installation and `dart --version` verification before rerunning install. | `claude`, `dart` |
| `codex` | `mobile` | `darwin`, `linux` | Add the Dart and Flutter MCP server using `codex mcp add dart -- dart mcp-server --force-roots-fallback`; on Ubuntu, missing Dart points to Flutter SDK installation and `dart --version` verification before rerunning install. | `codex`, `dart` |
| `skills` | `agents` | all | Install the reviewed Matt Pocock engineering skill set from `mattpocock/skills/skills/engineering` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `agents` | all | Install `grilling`, `loop-me`, `review`, and `writing-great-skills` from `mattpocock/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`, copying the skills into agent skill roots. | `npx` |
| `skills` | `agents`, `codex-delegation` | all | Install the dots-owned `delegation` skill from `yersonargotev/dots/skills/delegation` globally for `codex` through pinned `skills@1.5.12`, copying the skill into the Codex skill root. | `npx` |
| `skills` | `agents` | all | Install the dots-owned `delegation` skill globally for `claude-code`, `antigravity`, `opencode`, and `github-copilot`; splitting this Provisioner keeps `codex-delegation` narrow while preserving the `agents` Profile behavior. | `npx` |
| `claude` | `web` | `darwin`, `linux` | Register marketplace `ChromeDevTools/chrome-devtools-mcp`. | `claude` |
| `claude` | `web` | `darwin`, `linux` | Install `chrome-devtools-mcp` from `chrome-devtools-plugins` with user scope. | `claude` |
| `codex` | `web` | `darwin`, `linux` | Add MCP server `chrome-devtools` using `npx -y chrome-devtools-mcp@latest --no-performance-crux`. | `codex` |
| `codegraph` | `codegraph` | `darwin`, `linux` | Reuse `codegraph` when already on `PATH`; otherwise install it with the official curl bootstrap, then run `codegraph install --target codex,claude,antigravity,opencode --location global --yes` so CodeGraph configures MCP plus instructions for Codex, Claude Code, Antigravity, and OpenCode. Select with `--tag codegraph`. | `curl` |

After any `gentle-ai` provisioner runs, `dots` converges supported agent
instruction files by removing `<!-- gentle-ai:trigger-rules -->` and
`<!-- gentle-ai:persona -->` marker sections, cleaning known legacy markerless
persona prose, and upserting a compact `<!-- dots:rules -->` block with
critical dots-owned behavioral rules plus a pointer to the installed
`delegation` skill. The upstream 4R identifiers referenced by the trigger-rules
block (`review-readability`, `review-risk`, `review-resilience`, and
`review-reliability`) are not portable `gentle-ai` skills that the dots-managed
baseline can install for every supported agent, so keeping the recommendation
would leave agents pointing at commands that may not exist.

## Selection rules

1. The chosen Profile or repeated Profiles provide the base tag union in CLI order.
2. Each repeated `--tag` adds an optional tag to that selection. Duplicate tags are ignored.
3. A Managed Entry or Provisioner is selected when any of its tags matches the effective tag set.
4. If `os` is empty, the item matches all supported operating systems.
5. If `os` is set, the item only matches the current OS (`darwin` or `linux`).
6. Selected Managed Entries are installed before selected Provisioners run.

During `dots update` and `dots upgrade`, dots resolves the same authoritative
Profile and explicit extra Tag intent before and after Source of Truth refresh.
It reports effective Tag and selected Managed Entry, Dependency, and Provisioner
additions and removals before application. A saved Profile that no longer
exists, or an explicit extra Tag no longer declared by any Managed Entry,
Dependency Set, or Provisioner, blocks application without rewriting the
Installed Selection. Dots-owned selection modifiers and their documented legacy
aliases remain valid even though they affect installation behavior rather than
selecting a manifest surface. Removed surfaces are reported only; they are not
automatically deleted or uninstalled.

## Related docs

- [`CONTEXT.md`](../CONTEXT.md) defines the domain vocabulary.
- [`docs/scope.md`](scope.md) explains why the MVP Configuration Set is intentionally bounded.
- [`docs/adr/0003-claude-plugin-provisioner.md`](adr/0003-claude-plugin-provisioner.md) records the Claude plugin provisioner decision.
- [`docs/adr/0004-codex-mcp-provisioner.md`](adr/0004-codex-mcp-provisioner.md) records the Codex MCP provisioner decision.
- [`docs/adr/0005-opencode-mcp-config-overlay.md`](adr/0005-opencode-mcp-config-overlay.md) records the OpenCode config overlay decision.
- [`docs/adr/0007-hybrid-skill-provisioning.md`](adr/0007-hybrid-skill-provisioning.md) records the hybrid repo-owned/external skill ownership decision.
