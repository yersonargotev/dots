# Zsh / Zim configuration

Portable, modular Zsh foundation managed by `dots`. This is the first
workstation configuration slice (issue #38, epic #37).

## Managed targets

`dots` installs three co-owned native loaders and three portable symlinks:

| Source                | Target       |
| --------------------- | ------------ |
| `configs/zsh/loader.zsh` | `~/.zshrc` (`copy`, marked block) |
| `configs/zsh/loader.zshenv` | `~/.zshenv` (`copy`, marked block) |
| `configs/zsh/loader.zimrc` | `~/.zimrc` (`copy`, marked block) |
| `configs/zsh/zshrc`   | `~/.config/dots/zsh/zshrc` |
| `configs/zsh/zimrc`   | `~/.config/dots/zsh/zimrc` |
| `configs/zsh/zshenv`  | `~/.config/dots/zsh/zshenv` |

The native `~/.zshrc`, `~/.zshenv` and `~/.zimrc` remain regular files. Installers
and personal additions can extend them without writing into the Installed Repository.
Each initial dots-owned block loads its portable symlink; dots preserves all
content outside that block. Proven legacy symlinks migrate through the existing
Source of Truth provenance and Backup Set checks. Unproven links, malformed or
edited blocks remain Conflicts; uninstall subtracts only the owned block.

## Layout

```
configs/zsh/
├── loader.zsh             # native marked block loading the portable entrypoint
├── loader.zshenv          # native environment loader
├── loader.zimrc           # native Zim module loader
├── zshrc                  # portable entrypoint: pre-init → Zim → post-init → local
├── zimrc                  # Zim module list
├── zshenv                 # environment for all shells (editor, cargo)
├── rc.d/
│   ├── pre/               # sourced BEFORE Zim loads its modules
│   │   ├── 00-history.zsh
│   │   ├── 10-input.zsh
│   │   └── 20-modules.zsh # module tuning read at load time
│   └── post/              # sourced AFTER Zim initializes
│       ├── 30-path.zsh    # portable PATH additions (guarded)
│       ├── 40-tools.zsh   # fzf/editor/zoxide/starship/atuin/fnm (guarded)
│       ├── 50-aliases.zsh # eza listings (guarded)
│       └── 60-ai.zsh      # Claude rendering knob (no secrets)
└── zshrc.local.example    # template for machine-specific values + secrets
```

Drop a new `*.zsh` file into `rc.d/pre/` or `rc.d/post/` to extend the config;
the numeric prefix controls load order.

## Interactive navigation

The managed widget ownership is intentionally non-overlapping:

- Tab presents grouped completion candidates through fzf-tab.
- Ctrl+T inserts a file or directory through fzf, with bat file previews.
- Alt+C changes directory through fzf, with eza directory previews.
- Ctrl+R and Up belong to Atuin; Ctrl+R begins in Git-workspace scope when
  available, while Up remains scoped to the current shell session.
- Ctrl+X Ctrl+E opens the current command with Zsh's standard
  `edit-command-line` widget. zoxide continues to provide `z` and `zi`.

fzf uses a bounded lower-screen Catppuccin presentation. All integrations are
guarded: if an explicitly skipped Dependency is absent, shell startup remains
usable and reports the unavailable feature without installing or downloading
anything.

## Local overrides and secrets

Anything machine-specific or secret goes in `~/.zshrc.local`, which `dots` never
manages. Start from the template:

```sh
cp configs/zsh/zshrc.local.example ~/.zshrc.local
```

This is where tokens, per-machine `PATH` entries, and
IDE shell integrations live.

## Zim runtime

dots provisions the Zim runtime with the existing curl bootstrap when the zimfw
Tag is selected. The portable `configs/zsh/zimrc` remains the Source of Truth
for project modules, loaded through `~/.zimrc`; personal modules may follow its
managed block. The generated runtime lives under `~/.zim/`. Missing runtime
files produce a warning and shell startup continues without modules.

If `~/.zim/` is missing, rerun `dots install --profile default` or another
profile that includes `core`, then restart your shell. The following are
intentionally **excluded** from the repository: `~/.zim/`, `.zcompdump*`,
`.zsh_history`, `.zsh_sessions/`, and any generated or backup files.

## Validation

Validate against a temporary home — never your real `$HOME`:

```sh
dots install --home /tmp/dots-home --source-root "$PWD" --state-root /tmp/dots-state
dots status  --home /tmp/dots-home --source-root "$PWD" --state-root /tmp/dots-state
```
