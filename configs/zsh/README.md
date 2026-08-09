# Zsh / Zim configuration

Portable, modular Zsh foundation managed by `dots`. This is the first
workstation configuration slice (issue #38, epic #37).

## Managed targets

`dots` installs a co-owned native loader and three portable symlinks:

| Source                | Target       |
| --------------------- | ------------ |
| `configs/zsh/loader.zsh` | `~/.zshrc` (`copy`, marked block) |
| `configs/zsh/zshrc`   | `~/.config/dots/zsh/zshrc` |
| `configs/zsh/zimrc`   | `~/.zimrc`   |
| `configs/zsh/zshenv`  | `~/.zshenv`  |

The native `~/.zshrc` remains a regular file so third-party installers can
append their own shell content without writing into the Installed Repository.
Its initial dots-owned block loads the portable symlink; dots preserves all
content outside that block.

## Layout

```
configs/zsh/
├── loader.zsh             # native marked block loading the portable entrypoint
├── zshrc                  # portable entrypoint: pre-init → Zim → post-init → local
├── zimrc                  # Zim module list
├── zshenv                 # environment for all shells (editor, cargo, foundry)
├── rc.d/
│   ├── pre/               # sourced BEFORE Zim loads its modules
│   │   ├── 00-history.zsh
│   │   ├── 10-input.zsh
│   │   └── 20-modules.zsh # module tuning read at load time
│   └── post/              # sourced AFTER Zim initializes
│       ├── 30-path.zsh    # portable PATH additions (guarded)
│       ├── 40-tools.zsh   # zoxide/starship/atuin/fnm (guarded)
│       ├── 50-aliases.zsh # eza listings (guarded)
│       └── 60-ai.zsh      # Claude/Copilot knobs (no secrets)
└── zshrc.local.example    # template for machine-specific values + secrets
```

Drop a new `*.zsh` file into `rc.d/pre/` or `rc.d/post/` to extend the config;
the numeric prefix controls load order.

## Local overrides and secrets

Anything machine-specific or secret goes in `~/.zshrc.local`, which `dots` never
manages. Start from the template:

```sh
cp configs/zsh/zshrc.local.example ~/.zshrc.local
```

This is where tokens, per-machine `PATH` entries, and
IDE shell integrations live.

## Zim runtime

dots provisions the Zim runtime as part of the core install. `~/.zimrc` remains
the Source of Truth for module selection, and the generated runtime lives under
`~/.zim/`.

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
