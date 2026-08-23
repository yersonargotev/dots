# Install Manifest Configuration

`dots.yaml` is the Install Manifest: the reviewable contract that tells `dots`
which Managed Configuration to install, which Dependencies to check, and which
allowlisted Provisioners to run.

Use this page when reviewing a manifest change. It shows both the supported
configuration surface and the configuration currently declared by this repository.

## Supported manifest shape

```yaml
version: 1
tags: {}
profiles: {}
dependencies: []
entries: []
provisioners: []
```

| Field | Required | Meaning |
|-------|----------|---------|
| `version` | Yes | Manifest schema version. Only `1` is supported. |
| `tags` | No | Declared selection Tags and their lifecycle metadata. When present, every referenced Tag must be declared. |
| `profiles` | Yes | Named installation selections. A Profile selects entries and provisioners by matching tags. |
| `dependencies` | No | Tag-scoped Dependency Sets for shared toolchain baselines selected before Managed Entry and Provisioner Dependencies. |
| `entries` | Yes | Managed Entries that install repository-owned files or directories into `$HOME`. |
| `provisioners` | No | Allowlisted external tool invocations run after file entries are installed. |

Unknown YAML fields are rejected during manifest loading.

## Profiles

A Profile is an ordered convenience preset over independently meaningful Tags
and is selected explicitly during install. Read-only selection-aware
commands (`status`, `doctor`, `plan`, `deps check`, and `deps plan`) reuse the
authoritative Installed Selection when both `--profile` and `--tag` are omitted.
Any explicit selection flag makes the current invocation's complete selection
win without rewriting Installation Metadata. Repeat `--profile` to compose
multiple Profiles by the ordered union of their tags. Commands that expose
`--tag` can form a complete selection without a Profile or add optional
capability Tags to a selected Profile set. For example, `--tag adaptive-theme`
installs the opt-in marker and
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

## Tags

Each current Tag is the smallest capability that makes sense to install or stop
managing independently. It may select Managed Entries, Dependencies,
Provisioners, or a cohesive combination of them. Declared Tags have a `kind`
(`surface`, `cleanup`, or `compatibility`) and a
`status` (`current` or `legacy`). Every legacy Tag declares an ordered,
non-empty `replaced_by` sequence of distinct current Tags. Replacement targets
must be declared and cannot point to another legacy Tag. Current Tags cannot
declare replacements.

```yaml
tags:
  shell:
    kind: surface
    status: current
  terminal:
    kind: surface
    status: current
  old-workstation:
    kind: compatibility
    status: legacy
    replaced_by: [shell, terminal]
```

The historical scalar spelling (`replaced_by: shell`) remains readable as a
one-Tag alias. New and updated declarations should use the sequence form.
Selection expands a legacy alias in declared order, de-duplicates the resulting
current Tags without reordering them, and reports deterministic migration
evidence. Persisted successful intent contains only normalized current Tags.

The following compact catalog is generated from the Install Manifest. It lists
current and legacy declarations; the `dots catalog` command hides legacy items
from compact discovery unless `--all` is supplied.

<!-- dots:catalog:start -->

### Profiles

| Profile | Status | Tags | Description |
|---------|--------|------|-------------|
| `agents` | current | `codex`, `claude`, `opencode`, `antigravity`, `copilot` | Native configuration for Codex, Claude Code, OpenCode, Antigravity, and Copilot CLI. |
| `core` | current | `zsh`, `zimfw`, `git`, `starship`, `tmux`, `herdr`, `zellij`, `atuin`, `neovim`, `tuicr`, `bat`, `node`, `rust`, `go`, `uv`, `pnpm`, `bun`, `fzf`, `zoxide`, `lazygit`, `eza`, `ripgrep`, `delta`, `fd`, `gh`, `jq` | Core dotfiles and general developer tooling without agent, web, or mobile provisioners. |
| `desktop` | current | `ghostty`, `warp`, `zed`, `codexbar` | Desktop-only configuration and integrations; compose with core when core dotfiles are desired too. |
| `mobile` | current | `dart-skills`, `flutter-skills`, `android-skills`, `claude-dart-mcp`, `codex-dart-mcp`, `antigravity-dart-mcp`, `vscode-mobile` | Optional Dart and Flutter mobile development capabilities. |
| `web` | current | `playwright`, `frontend-design`, `vercel-web-skills`, `claude-chrome-devtools`, `codex-chrome-devtools`, `opencode-chrome-devtools` | Optional frontend and browser workbench. |
| `workstation` | current | `zsh`, `zimfw`, `git`, `starship`, `tmux`, `herdr`, `zellij`, `atuin`, `neovim`, `tuicr`, `bat`, `node`, `rust`, `go`, `uv`, `pnpm`, `bun`, `fzf`, `zoxide`, `lazygit`, `eza`, `ripgrep`, `delta`, `fd`, `gh`, `jq`, `ghostty`, `warp`, `zed`, `codex`, `claude`, `opencode`, `antigravity`, `copilot` | Composite core, desktop, and Agent CLI Baseline selection; web and mobile stay opt-in. |

### Tags

| Tag | Kind | Status | Description | Replacement |
|-----|------|--------|-------------|-------------|
| `adaptive-theme` | surface | current | Opt-in adaptive-theme marker and supported app-specific sources or fragments. |  |
| `agents` | compatibility | legacy | Legacy Agent CLI Baseline alias; use the five atomic Agent capability Tags. | `codex`, `claude`, `opencode`, `antigravity`, `copilot` |
| `android-skills` | surface | current | Android CLI agent skill for supported agents. |  |
| `antigravity` | surface | current | Antigravity CLI requirement and dots-owned native configuration. |  |
| `antigravity-dart-mcp` | surface | current | Antigravity Dart and Flutter MCP settings contribution. |  |
| `atuin` | surface | current | Atuin shell history configuration and Catppuccin theme. |  |
| `bat` | surface | current | bat syntax-highlighting pager configuration. |  |
| `bun` | surface | current | Bun JavaScript runtime and toolkit. |  |
| `claude` | surface | current | Claude Code CLI requirement and dots-owned native configuration. |  |
| `claude-chrome-devtools` | surface | current | Claude Chrome DevTools marketplace and plugin integration. |  |
| `claude-dart-mcp` | surface | current | Claude Dart and Flutter MCP integration. |  |
| `codegraph` | surface | current | Opt-in CodeGraph provisioner and Codex SessionStart hook source override. |  |
| `codex` | surface | current | Codex CLI requirement and dots-owned native configuration. |  |
| `codex-chrome-devtools` | surface | current | Codex Chrome DevTools MCP integration. |  |
| `codex-dart-mcp` | surface | current | Codex Dart and Flutter MCP integration. |  |
| `codexbar` | surface | current | CodexBar menu-bar monitor for AI coding-provider usage limits. |  |
| `copilot` | surface | current | Copilot CLI requirement and dots-owned native configuration. |  |
| `core` | compatibility | legacy | Legacy Core Development Baseline alias; use the atomic replacement Tags. | `zsh`, `zimfw`, `git`, `starship`, `tmux`, `herdr`, `zellij`, `atuin`, `neovim`, `tuicr`, `bat`, `node`, `rust`, `go`, `uv`, `pnpm`, `bun`, `fzf`, `zoxide`, `lazygit`, `eza`, `ripgrep`, `delta`, `fd`, `gh`, `jq` |
| `dart-skills` | surface | current | Dart agent skills for supported agents. |  |
| `delta` | surface | current | Delta syntax-highlighting pager for Git diffs. |  |
| `desktop` | compatibility | legacy | Legacy desktop alias; use the Ghostty, Warp, and Zed capability Tags. | `ghostty`, `warp`, `zed` |
| `eza` | surface | current | eza modern directory listing utility. |  |
| `fd` | surface | current | fd filesystem search utility. |  |
| `flutter-skills` | surface | current | Flutter agent skills for supported agents. |  |
| `frontend-design` | surface | current | Anthropic frontend design skill for supported agents. |  |
| `fzf` | surface | current | fzf command-line fuzzy finder. |  |
| `gh` | surface | current | GitHub CLI for repository and workflow operations. |  |
| `ghostty` | surface | current | Ghostty terminal configuration and application requirement. |  |
| `git` | surface | current | Portable Git configuration and native Git entrypoint. |  |
| `go` | surface | current | Go language toolchain. |  |
| `herdr` | surface | current | Herdr terminal theme configuration on macOS. |  |
| `jq` | surface | current | jq command-line JSON processor. |  |
| `lazygit` | surface | current | Lazygit terminal interface for Git. |  |
| `mobile` | compatibility | legacy | Legacy Mobile workbench alias; use the atomic mobile capability Tags. | `dart-skills`, `flutter-skills`, `android-skills`, `claude-dart-mcp`, `codex-dart-mcp`, `antigravity-dart-mcp`, `vscode-mobile` |
| `neovim` | surface | current | Neovim configuration and seeded plugin lockfile. |  |
| `node` | surface | current | Node.js LTS toolchain managed through fnm. |  |
| `opencode` | surface | current | OpenCode CLI requirement and dots-owned native configuration. |  |
| `opencode-chrome-devtools` | surface | current | OpenCode Chrome DevTools JSON subset integration. |  |
| `playwright` | surface | current | Playwright CLI dependency and upstream agent skill. |  |
| `pnpm` | surface | current | pnpm JavaScript package manager. |  |
| `ripgrep` | surface | current | ripgrep recursive text search utility. |  |
| `rust` | surface | current | Rust stable toolchain managed through rustup. |  |
| `starship` | surface | current | Starship prompt configuration. |  |
| `tmux` | surface | current | Tmux terminal multiplexer configuration. |  |
| `tuicr` | surface | current | tuicr terminal interface configuration. |  |
| `uv` | surface | current | uv Python project and package manager. |  |
| `vercel-web-skills` | surface | current | Vercel React and web design skills for supported agents. |  |
| `vscode-mobile` | surface | current | VS Code Dart MCP settings for mobile development. |  |
| `warp` | surface | current | Warp terminal settings and keybindings. |  |
| `web` | compatibility | legacy | Legacy Web workbench alias; use the atomic web capability Tags. | `playwright`, `frontend-design`, `vercel-web-skills`, `claude-chrome-devtools`, `codex-chrome-devtools`, `opencode-chrome-devtools` |
| `zed` | surface | current | Zed editor settings, keybindings, and authored theme. |  |
| `zellij` | surface | current | Zellij terminal workspace configuration and default layout. |  |
| `zimfw` | surface | current | Zim framework module configuration and managed runtime provisioning. |  |
| `zoxide` | surface | current | zoxide smarter directory navigator. |  |
| `zsh` | surface | current | Portable Zsh entrypoints and shell environment configuration. |  |

<!-- dots:catalog:end -->

Any explicit `--profile` or `--tag` flags describe the complete selection for
that invocation; they are not merged with an Installed Selection. These
examples therefore repeat every intended Profile and Tag:

```bash
# Compose core configuration with the optional web workbench.
dots install --profile core --profile web

# Add an optional capability to the complete workstation selection.
dots install --profile workstation --tag adaptive-theme

```

Plain `dots install` in an interactive terminal provides the human workflow for
editing the same complete selection. It starts from the authoritative Installed
Selection, and Profile presets select their ordered current Tags in the draft.
After the operator reviews and confirms the candidate, terminal success stores
those current Tags explicitly rather than preserving Profile names. Removing
Tags requires a separate Installed Selection reduction acknowledgement before
Conflict Resolution. An empty draft additionally requires the literal `clear`;
canceling or entering any other phrase leaves Managed Entries and the previous
Installed Selection unchanged.

The selector uses the current initialized Installed Repository and never
refreshes the Source of Truth. Use `dots update` first when a refreshed checkout
is intended. It remains a human-only surface: non-interactive and JSON commands
must express the complete selection with `--profile`, `--tag`, or
`--clear-selection`. Deselected Dependencies and Provisioners are Retained
External State, not uninstall or rollback instructions.

Tags are declarative selection only: they select Managed Entries, Dependencies,
Provisioners, and source overrides, but never authorize hidden built-in cleanup
or historical migration behavior. Historical retirement uses sufficiently
specific Installation Metadata receipts independently from current selection.

The `web` and `mobile` Profiles are behavior-preserving ordered presets over
their atomic skill and agent-integration Tags. Their former broad Tags are
hidden compatibility aliases with the same ordered replacements. The
`adaptive-theme` and `codegraph` Tags remain single current global opt-ins:
they apply only at supported consumer seams instead of multiplying into
consumer-specific variants.

See the [adaptive theme audit](adaptive-theme-audit.md) and the
[`codegraph` Provisioner specification](#codegraph-spec) for the detailed
behavior behind those focused capabilities.

## Tag-scoped Dependency Sets

Top-level `dependencies` declare capability Dependencies selected by Tag before
Managed Entry and Provisioner Dependencies. The Core Development Baseline from
ADR 0010 is now composed from atomic Dependency-only Tags: `node`, `rust`,
`go`, `uv`, `pnpm`, `bun`, `fzf`, `zoxide`, `lazygit`, `eza`, `ripgrep`,
`delta`, `fd`, `gh`, and `jq`. The internal `unzip` prerequisite follows the
`node` Tag because the constrained fnm bootstrap consumes it; it is not an
artificial catalog capability. The atomic `codex`, `claude`, `opencode`,
`antigravity`, and `copilot` sets each pair one Agent CLI requirement with its
Managed Configuration. An internal set selected by `claude` and `copilot`
shares `jq` without making it a compulsory standalone selection. The Desktop
Nerd Font is similarly selected by `ghostty`, `warp`, and `zed`. Linux Homebrew
fallback is explicit with `linux_homebrew: true`; GUI apps and fonts stay manual
unless they have validated Linux support. Node and Rust use constrained
built-in toolchain bootstrap commands after manager installation:
`fnm install --lts` for Node LTS and `rustup default stable` for Rust stable.
Bootstrap declarations do not make an action executable by themselves: the
manager executable must already be on `PATH` or a concrete installer/provider
must run first.

The `codexbar` set is Darwin-only. It detects `CodexBar.app` in the supported
macOS application locations and uses the Homebrew cask `codexbar` when the app
is missing. The `desktop` Profile includes this independently selectable Tag;
the legacy `desktop` alias expands only to `ghostty`, `warp`, and `zed`.
Linux selections remain valid and omit the CodexBar Dependency.

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


A Managed Entry declares one repository source and one confined target. Targets
use `~` or `~/...` by default. The allowlisted `xdg-state` target root instead
resolves a relative target beneath absolute `$XDG_STATE_HOME`, defaulting to
`~/.local/state`; it remains confined to the selected home and is distinct from
dots' own `--state-root`.

| Field | Required | Supported values |
|-------|----------|------------------|
| `source` | Yes | Repository-relative path to Managed Configuration. |
| `source_overrides` | No | Map of exact selected tag to alternate repository-relative source for the same target. The base `source` is used when no key matches; overrides do not alter entry tag selection or OS applicability. |
| `target` | Yes | Home-relative `~` or `~/...`; with `target_root: xdg-state`, a confined relative path. |
| `target_root` | No | `xdg-state`. It requires `ownership: seeded`; empty keeps home-target resolution. |
| `strategy` | Yes | `symlink`, `copy`, or `template` in the manifest schema. Current install execution supports `symlink` and `copy`. |
| `ownership` | No | Empty, `json-subset`, `jsonc-subset`, `toml-subset`, `marked-block`, or `seeded`. Explicit ownership requires `strategy: copy`. |
| `tags` | Yes | Non-empty strings matched against the selected Profile. |
| `os` | No | `darwin`, `linux`; empty means all supported operating systems. |
| `dependencies` | No | Entry-level Dependencies. |

For a `json-subset` target already trusted by matching Installation Metadata,
the recorded prior contribution enables a three-state update: new owned values
are added, unchanged retired values are removed, and target-only values are
preserved. A changed former contribution or incompatible scalar/object/array
value remains a Conflict. Every applied update creates a Backup Set. Legacy
metadata without prior-contribution evidence may establish a new baseline after
a safe additive or unchanged install, but never authorizes removal retroactively;
a compatible pre-existing target without matching metadata is still a Conflict.

For a `jsonc-subset` target, object keys use the same recursive three-state
ownership model, but scalars and arrays are atomic ordered values. Compatible
updates preserve target-only object keys plus untouched comments, trailing
commas, ordering, and formatting. A changed owned scalar or array is Drift for
a recorded target and a Conflict without trusted Installation Metadata.

For a `toml-subset` target, dots owns baseline values declared at root or in
ordinary tables. Scalars, arrays, and inline tables are atomic; target-only keys
and tables remain outside dots ownership. Exact prior-contribution evidence
allows compatible baseline additions, replacements, and removals without
rewriting unrelated comments or formatting. Changed owned values remain Drift
and are never overwritten. Uninstall subtracts only unchanged recorded values
and preserves external content.

For `seeded` ownership, dots stores the exact opaque baseline in Installation
Metadata. Missing state is seeded; live state still equal to the prior baseline
advances to the current baseline after a Backup Set; and locally evolved state
is preserved as aligned information with reason `seeded-local-evolution`.
Uninstall retains the physical runtime state and removes only its ownership
record.

For `marked-block` ownership, dots records one exact initial delimited block in
Installation Metadata. Blank lines and comments may precede it; executable
content may not. Updates replace only an unchanged prior contribution after a
Backup Set and preserve surrounding bytes. Duplicate, incomplete, moved, or
edited blocks remain Conflicts. Uninstall subtracts only the recorded block and
removes the regular-file container only when no external bytes remain.

The materialization choices below implement the
[Application-Writable Target decision](adr/0017-keep-application-writable-targets-outside-the-installed-repository.md).
Its [dated evidence inventory](application-writable-target-research.md)
distinguishes normal writers from explicit operator outputs, Ghostty's
conditional initializer, and entries read under ordinary use. Repository tests
lock that classification for every remaining symlink and exercise the confirmed
writers through an integrated Temporary Home lifecycle so ordinary application
writes cannot dirty the Installed Repository.

Current Managed Entries:

| Source | Target | Strategy | Tags | OS | Dependencies |
|--------|--------|----------|------|----|--------------|
| `configs/zsh/loader.zsh` | `~/.zshrc` | `copy` (`marked-block`) | `zsh` | `darwin`, `linux` | `zsh` |
| `configs/zsh/zshrc` | `~/.config/dots/zsh/zshrc` | `symlink` | `zsh` | `darwin`, `linux` | None |
| `configs/zsh/zimrc` | `~/.zimrc` | `symlink` | `zimfw` | `darwin`, `linux` | `zsh` |
| `configs/zsh/zshenv` | `~/.zshenv` | `symlink` | `zsh` | `darwin`, `linux` | `zsh` |
| `configs/git/loader.gitconfig` | `~/.gitconfig` | `copy` (`marked-block`) | `git` | `darwin`, `linux` | `git` |
| `configs/git/gitconfig` | `~/.config/dots/git/gitconfig` | `symlink` | `git` | `darwin`, `linux` | None |
| `configs/tuicr/config.toml` | `~/.config/tuicr/config.toml` | `copy` | `tuicr` | `darwin`, `linux` | `tuicr`; owns TOML subset |
| `configs/dots/theme.sh` | `~/.config/dots/theme.sh` | `symlink` | `tmux`, `neovim` | `darwin`, `linux` | None |
| `configs/dots/adaptive-theme` | `~/.config/dots/adaptive-theme` | `symlink` | `adaptive-theme` | `darwin`, `linux` | None |
| `configs/starship/starship.toml` | `~/.config/starship.toml` | `symlink` | `starship` | `darwin`, `linux` | `starship` |
| `configs/tmux/tmux.conf` | `~/.tmux.conf` | `symlink` | `tmux` | `darwin`, `linux` | `tmux` |
| `configs/herdr/config.toml` (`adaptive-theme` override: `configs/herdr/config-adaptive.toml`) | `~/.config/herdr/config.toml` | `copy` | `herdr` | `darwin` | `herdr`; owns TOML subset |
| `configs/zellij/config.kdl` (`adaptive-theme` override: `configs/zellij/config-adaptive.kdl`) | `~/.config/zellij/config.kdl` | `copy` | `zellij` | `darwin`, `linux` | `zellij`; whole-target ownership |
| `configs/zellij/layouts/default.kdl` | `~/.config/zellij/layouts/default.kdl` | `symlink` | `zellij` | `darwin`, `linux` | `zellij` |
| `configs/ghostty/config.ghostty` | `~/.config/ghostty/config.ghostty` | `symlink` | `ghostty` | `darwin`, `linux` | `ghostty` |
| `configs/ghostty/adaptive/adaptive-theme.ghostty` | `~/.config/ghostty/adaptive-theme.ghostty` | `symlink` | `adaptive-theme` | `darwin` | None |
| `configs/warp/settings.toml` | `~/.warp/settings.toml` | `copy` | `warp` | `darwin` | None |
| `configs/warp/keybindings.yaml` | `~/.warp/keybindings.yaml` | `copy` | `warp` | `darwin` | None |
| `configs/warp/settings.toml` | `~/.config/warp-terminal/settings.toml` | `copy` | `warp` | `linux` | `Warp` |
| `configs/warp/keybindings.yaml` | `~/.config/warp-terminal/keybindings.yaml` | `copy` | `warp` | `linux` | `Warp` |
| `configs/atuin/config.toml` | `~/.config/atuin/config.toml` | `copy` | `atuin` | `darwin`, `linux` | `atuin`; owns TOML subset |
| `configs/atuin/themes/catppuccin-mocha.toml` | `~/.config/atuin/themes/catppuccin-mocha.toml` | `symlink` | `atuin` | `darwin`, `linux` | `atuin` |
| `configs/bat/config` | `~/.config/bat/config` | `copy` | `bat` | `darwin`, `linux` | `bat`; whole-target ownership |
| `configs/nvim/lazy-lock.json` | `$XDG_STATE_HOME/nvim/lazy-lock.json` | `copy` (`seeded`) | `neovim` | `darwin`, `linux` | None |
| `configs/nvim/loader.lua` | `~/.config/nvim/init.lua` | `copy` | `neovim` | `darwin`, `linux` | `neovim` |
| `configs/nvim` | `~/.config/dots/nvim` | `symlink` | `neovim` | `darwin`, `linux` | None |
| `configs/zed/settings.json` | `~/.config/zed/settings.json` | `copy` (`jsonc-subset`) | `zed` | `darwin`, `linux` | `zed` |
| `configs/zed/keymap.json` | `~/.config/zed/keymap.json` | `copy` (`seeded`) | `zed` | `darwin`, `linux` | `zed` |
| `configs/zed/themes/catppuccin-blue.json` | `~/.config/zed/themes/catppuccin-blue.json` | `symlink` | `zed` | `darwin`, `linux` | `zed` |
| `configs/claude/settings.json` | `~/.claude/settings.json` | `copy` | `claude` | `darwin`, `linux` | None; owns JSON subset |
| `configs/claude/statusline-command.sh` | `~/.claude/statusline-command.sh` | `copy` | `claude` | `darwin`, `linux` | None |
| `configs/codex/config.toml` (`codegraph` override: `configs/codex/config-codegraph.toml`) | `~/.codex/config.toml` | `copy` | `codex` | `darwin`, `linux` | None; owns TOML subset; CodeGraph tag adds a Codex SessionStart hook |
| `configs/copilot/settings.json` | `~/.copilot/settings.json` | `copy` | `copilot` | `darwin`, `linux` | None; owns JSON subset |
| `configs/copilot/statusline-command.sh` | `~/.copilot/statusline-command.sh` | `copy` | `copilot` | `darwin`, `linux` | None |
| `configs/antigravity/settings.json` | `~/.gemini/antigravity-cli/settings.json` | `copy` | `antigravity` | `darwin`, `linux` | None; owns the broad Antigravity JSON baseline |
| `configs/antigravity/mobile-mcp-settings.json` | `~/.gemini/antigravity-cli/settings.json` | `copy` | `antigravity-dart-mcp` | `darwin`, `linux` | None; owns only the Dart/Flutter MCP JSON subset |
| `configs/vscode/settings.json` | `~/Library/Application Support/Code/User/settings.json` | `copy` | `vscode-mobile` | `darwin` | None; owns JSON subset enabling Dart MCP for GitHub Copilot in VS Code |
| `configs/vscode/settings.json` | `~/.config/Code/User/settings.json` | `copy` | `vscode-mobile` | `linux` | None; owns JSON subset enabling Dart MCP for GitHub Copilot in VS Code |
| `configs/opencode/opencode.json` | `~/.config/opencode/opencode.json` | `copy` | `opencode` | `darwin`, `linux` | None; owns only the native JSON baseline subset |
| `configs/opencode/mcp.json` | `~/.config/opencode/opencode.json` | `copy` | `opencode-chrome-devtools` | `darwin`, `linux` | `opencode`; contributes the Chrome DevTools JSON subset alongside the native baseline |

## Dependencies

Dependencies can be declared in tag-scoped Dependency Sets, Managed Entries,
and Provisioners. They are detection and installation guidance, not
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
| `darwin_app` | No | macOS `.app` bundle name that can satisfy the Dependency when its command is absent from `PATH`. Dots checks the selected user's `~/Applications` and the system `/Applications`; the value must be a bundle name such as `Ghostty.app`, not a path. Ignored outside macOS. |
| `manual` | No | Dependency-specific remediation for manual-only tools. Use it when generic package-manager guidance would hide required user choices, privileges, or verification steps. |
| `manual_debian` | No | Debian/Ubuntu-specific manual remediation. Prefer this over `manual` when instructions mention Debian/Ubuntu-only packages, commands, privileges, or verification. |
| `commands` | No | Multiple executable names that must all be present. Use this for manager-owned toolchains where both the manager and runtime commands matter. |
| `toolchain` | No | Built-in runtime bootstrap flow. Supported values: `node-lts-fnm`, `rust-stable-rustup`. These values map to fixed argv commands, not arbitrary shell hooks. |
| `brew` | No | Homebrew formula token. Mutually exclusive with `brew_cask`. |
| `linux_homebrew` | No | Allows Linux distro tiers to fall back to the Homebrew formula when the distro provider is absent or unavailable. Keep this opt-in for reviewed CLI/runtime tools only. |
| `user_local` | No | Linux-only opt-in for a reviewed User-Local Provider recipe. Requires `recipe`, `version`, and `checksum` or per-platform `checksums`; installs into `~/.local/bin` or `~/.local/opt/<tool>` without using system package managers. |
| `rolling_user_local` | No | macOS/Linux opt-in for a closed Rolling User-Local Provider recipe. Accepts only `recipe`; dots resolves the latest stable official version, immutable platform artifact, and digest before the action is installable. |
| `brew_cask` | No | Homebrew cask token. Renders as `brew install --cask <token>`. |
| `apt` | No | Debian/Ubuntu package name. |
| `dnf` | No | Fedora package name. |
| `pacman` | No | Arch package name. |
| `font_match` | No | Installed-font filename glob used for Font Dependencies. |
| `font_fallback_matches` | No | Additional filename globs that can satisfy the same Font Dependency. |


User-Local Providers are not system packages. They are reviewed, allowlisted Go recipes that write only to the selected home-owned environment and require explicit manifest policy. `dots` does not mutate shell startup files or `PATH`; after installation it re-runs normal Dependency probes, so `~/.local/bin` must already be visible in the current `PATH` for the Dependency to become present. Invalid `user_local` policy such as an unknown recipe, missing version, unsupported platform, or missing checksum is a manifest/planning error, not a silent fallback to Linuxbrew or manual guidance.

Rolling User-Local Providers use the separate `rolling_user_local` field. The manifest may select only a reviewed recipe and cannot declare a repository, URL, command, checksum, or installer script. The `codex` recipe resolves official stable `openai/codex` release metadata only when durable `codex` availability is absent, supports macOS/Linux on amd64/arm64, and requires the official release asset SHA-256 digest. The process-private Codex executable inside `ChatGPT.app/Contents/Resources` is not durable command availability and is skipped while probing `PATH`; a later terminal-visible Codex executable can still satisfy the Dependency. The `claude` recipe reads Anthropic's official stable channel and versioned integrity manifest, selects the macOS/Linux amd64/arm64 binary, verifies its published SHA-256 digest, and installs the raw executable without invoking Anthropic's installer.

The `opencode`, `antigravity`, and `copilot` recipes use the same closed GitHub release adapter for `anomalyco/opencode`, `google-antigravity/antigravity-cli`, and `github/copilot-cli`. Each selects exactly one allowlisted asset for macOS/Linux on amd64/arm64 and requires GitHub's official SHA-256 asset digest. OpenCode uses its root-level `opencode` executable from a macOS zip or Linux tar archive. Antigravity uses its root-level `antigravity` executable from a tar archive but exposes the installed command as `agy`; dots never runs the vendor installer. Copilot uses the root-level `copilot` executable from its tar archive. All archives retain the shared traversal and extraction limits.

Plans can therefore perform read-only network access, while a present command performs no release lookup or replacement. Resolution or verification failure stops a required installation before Managed Configuration changes. None of the recipes changes vendor auto-update settings.

`pnpm` is intentionally installed from the pinned standalone `@pnpm/linux-*` executable artifacts instead of through Corepack. Node remains part of the Core Development Baseline through `Node LTS (fnm)`, but the pnpm provider does not enable Corepack, create Corepack shims, run global `npm install -g`, or mutate shell configuration.

`neovim` uses the upstream multi-file Linux tarball through its User-Local Provider. The bundle is extracted under `~/.local/opt/nvim/<version>` and `~/.local/bin/nvim` points at the bundled executable.

Current dependency package coverage:

| Dependency | Detection | macOS/Homebrew | Debian/Ubuntu | Fedora | Arch |
|------------|-----------|----------------|---------------|--------|------|
| `Desktop Nerd Font` | Font globs `CascadiaCodeNF*` or `CaskaydiaCoveNerdFont*` | `brew install --cask font-cascadia-code-nf` | Manual | Manual | Manual |
| `CodexBar` | macOS `CodexBar.app` | `brew install --cask codexbar` | Not selected | Not selected | Not selected |
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
| `ghostty` | `ghostty` or macOS `Ghostty.app` | `brew install --cask ghostty` | Manual; Ubuntu guidance includes Ghostty upstream binary docs, the community `.deb` installer command, and notes `snap install ghostty --classic` requires sudo/password interactivity | Manual | Manual |
| `Warp` | `warp-terminal` | Manual | Manual | Manual | Manual |
| `atuin` | `atuin` | `atuin` | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual | User-local / Linuxbrew opt-in/manual |
| `bat` | `bat` | `bat` | User-local / Linuxbrew opt-in/manual | `bat` | `bat` |
| `neovim` | `nvim` | `neovim` | User-local / Linuxbrew opt-in/manual | `neovim` | `neovim` |
| `zed` | `zed` | `zed` | Manual | Manual | Manual |
| `jq` | `jq` | `jq` | `jq` | `jq` | `jq` |
| `OpenCode` | `opencode` | Rolling user-local | Rolling user-local | Rolling user-local | Rolling user-local |
| `Antigravity` | `agy` | Rolling user-local | Rolling user-local | Rolling user-local | Rolling user-local |
| `Copilot CLI` | `copilot` | Rolling user-local | Rolling user-local | Rolling user-local | Rolling user-local |
| `Claude Code` | `claude` | Rolling user-local | Rolling user-local | Rolling user-local | Rolling user-local |
| `Codex` | `codex` | Rolling user-local | Rolling user-local | Rolling user-local | Rolling user-local |
| `dart` | `dart` | Manual | Manual; Ubuntu guidance points to Flutter SDK installation because `dart mcp-server` ships with Dart/Flutter tooling, then asks users to verify `dart --version` | Manual | Manual |
| `curl` | `curl` | `curl` | `curl` | `curl` | `curl` |
| `npx` | `npx` | Manual | Manual | Manual | Manual |

Reviewed Linuxbrew opt-ins include CLI/runtime tools whose Homebrew formulas are
expected to work on Linuxbrew: Starship, the core runtimes/package tools, GitHub CLI,
Playwright CLI, lazygit, delta, fd, and atuin. Brew-only GUI apps such
as Ghostty and Zed remain manual on Linux.

Homebrew dependencies declared with a fully-qualified tap formula may require
explicit Homebrew Tap Trust on fresh macOS machines. `dots deps plan` and
`dots deps install --dry-run` surface the formula-level trust command, but
`dots` does not run trust commands automatically.

## Provisioners

Provisioners are a closed allowlist. They run after selected Managed Entries are
installed and only after their declared Dependencies are present. During execution,
`dots` threads the selected `--home`, sets `NPM_CONFIG_PREFIX` to `~/.local`, and
puts `~/.local/bin` first on `PATH` so npm-backed provisioners can install and
reuse user-local tools without requiring sudo in non-interactive runs.

| Field | Required | Supported values |
|-------|----------|------------------|
| `tool` | Yes | `claude`, `codex`, `codegraph`, `skills`, or `zimfw`. |
| `tags` | Yes | Non-empty strings matched against the selected Profile. |
| `os` | No | `darwin`, `linux`; empty means all supported operating systems. |
| `spec` | Yes | Tool-specific declaration. Each spec must speak exactly one tool dialect. |
| `dependencies` | No | Dependencies required before running the Provisioner. |

### `claude` spec

Supported shapes:

- `marketplace: <source>` renders a marketplace registration.
- `plugin: <name>` plus `from: <marketplace>` renders a plugin install.
- `mcp: <name>` plus `command: [...]` renders a stdio MCP server registration.

Claude specs must not mix shared CodeGraph/skills fields, skills-specific fields,
or multiple Claude shapes. `env` remains Codex-only.

### `codex` spec

Supported fields: `mcp`, `command`, and `env`.

Constraints:

- `mcp` is required.
- `command` must contain at least one non-empty argument.
- `env` keys must not be empty.
- Codex specs must not mix shared CodeGraph/skills fields or Claude fields.

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
- CodeGraph specs must not mix skill names, Claude plugin fields, MCP fields, or
  skills.sh-specific fields.

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
- Skills specs must not mix CodeGraph scalar fields, Claude fields, or Codex MCP
  fields.

### `zimfw` spec

Supported fields: `yes`.

`zimfw` renders one fixed non-interactive `zsh -c` invocation. It ensures
`~/.zim/zimfw.zsh` exists by downloading the latest ZimFW runtime when missing,
then runs `zimfw init -q` against the dots-managed `~/.zimrc`. The generated
runtime stays under `~/.zim/`; `~/.zimrc` remains the managed Source of Truth.

Constraints:

- `yes` is required and must be `true`.
- ZimFW specs must not mix CodeGraph/skills fields, Claude fields, MCP fields, or
  skills.sh fields.

Current Provisioners:

| Tool | Tags | OS | Rendered intent | Dependencies |
|------|------|----|-----------------|--------------|
| `zimfw` | `zimfw` | all | Install the ZimFW runtime under `~/.zim` when missing and run `zimfw init -q` using the dots-managed `~/.zimrc`. | `zsh`, `git`, `curl` |
| `skills` | `playwright` | all | Install `playwright-cli` from `microsoft/playwright-cli` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`, copying the skill and references into the agent skill roots. | `npx` |
| `skills` | `frontend-design` | all | Install `frontend-design` from `anthropics/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `vercel-web-skills` | all | Install `vercel-react-best-practices`, `vercel-composition-patterns`, `vercel-react-view-transitions`, and `web-design-guidelines` from `vercel-labs/agent-skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `dart-skills` | all | Install the upstream Dart skill package from `dart-lang/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `flutter-skills` | all | Install the upstream Flutter skill package from `flutter/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `skills` | `android-skills` | all | Install `android-cli` from `android/skills` globally for `codex`, `claude-code`, `antigravity`, `opencode`, and `github-copilot` through pinned `skills@1.5.12`. | `npx` |
| `claude` | `claude-dart-mcp` | `darwin`, `linux` | Add the Dart and Flutter MCP server using `claude mcp add --transport stdio dart -- dart mcp-server`; on Ubuntu, missing Dart points to Flutter SDK installation and `dart --version` verification before rerunning install. | `claude`, `dart` |
| `codex` | `codex-dart-mcp` | `darwin`, `linux` | Add the Dart and Flutter MCP server using `codex mcp add dart -- dart mcp-server --force-roots-fallback`; on Ubuntu, missing Dart points to Flutter SDK installation and `dart --version` verification before rerunning install. | `codex`, `dart` |
| `claude` | `claude-chrome-devtools` | `darwin`, `linux` | Register marketplace `ChromeDevTools/chrome-devtools-mcp`. | `claude` |
| `claude` | `claude-chrome-devtools` | `darwin`, `linux` | Install `chrome-devtools-mcp` from `chrome-devtools-plugins` with user scope. | `claude` |
| `codex` | `codex-chrome-devtools` | `darwin`, `linux` | Add MCP server `chrome-devtools` using `npx -y chrome-devtools-mcp@latest --no-performance-crux`. | `codex` |
| `codegraph` | `codegraph` | `darwin`, `linux` | Reuse `codegraph` when already on `PATH`; otherwise install it with the official curl bootstrap, then run `codegraph install --target codex,claude,antigravity,opencode --location global --yes` so CodeGraph configures MCP plus instructions for Codex, Claude Code, Antigravity, and OpenCode. Select with `--tag codegraph`. | `curl` |

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
Installed Selection. Removed surfaces are reported only; they are not
automatically deleted or uninstalled.

## Related docs

- [`CONTEXT.md`](../CONTEXT.md) defines the domain vocabulary.
- [`docs/scope.md`](scope.md) explains why the MVP Configuration Set is intentionally bounded.
- [`docs/adr/0003-claude-plugin-provisioner.md`](adr/0003-claude-plugin-provisioner.md) records the Claude plugin provisioner decision.
- [`docs/adr/0004-codex-mcp-provisioner.md`](adr/0004-codex-mcp-provisioner.md) records the Codex MCP provisioner decision.
- [`docs/adr/0005-opencode-mcp-config-overlay.md`](adr/0005-opencode-mcp-config-overlay.md) records the OpenCode config overlay decision.
- [`docs/adr/0007-hybrid-skill-provisioning.md`](adr/0007-hybrid-skill-provisioning.md) records the hybrid repo-owned/external skill ownership decision.
