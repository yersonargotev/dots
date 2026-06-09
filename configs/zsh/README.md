# Zsh / Zim configuration

Portable, modular Zsh foundation managed by `dots`. This is the first
workstation configuration slice (issue #38, epic #37).

## Managed targets

`dots` symlinks exactly three files into your home directory:

| Source                | Target       |
| --------------------- | ------------ |
| `configs/zsh/zshrc`   | `~/.zshrc`   |
| `configs/zsh/zimrc`   | `~/.zimrc`   |
| `configs/zsh/zshenv`  | `~/.zshenv`  |

The `symlink` strategy is **required**: `~/.zshrc` resolves its own real
location to find the modular files under `rc.d/`. A copied `~/.zshrc` would not
be able to locate them.

## Layout

```
configs/zsh/
├── zshrc                  # entry point: pre-init → Zim → post-init → local
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
│       ├── 50-aliases.zsh # eza-backed ls (guarded)
│       └── 60-ai.zsh      # Claude/Engram knobs (no secrets)
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

This is where tokens (e.g. `ENGRAM_CLOUD_TOKEN`), per-machine `PATH` entries, and
IDE shell integrations live.

## Zim runtime

Installing Zim itself is out of scope for this slice. If Zim is not present,
the shell prints a non-destructive notice and runs without modules. To install
Zim:

```sh
curl -fsSL https://raw.githubusercontent.com/zimfw/install/master/install.zsh | zsh
```

Then restart your shell. The following are intentionally **excluded** from the
repository: `~/.zim/`, `.zcompdump*`, `.zsh_history`, `.zsh_sessions/`, and any
generated or backup files.

## Validation

Validate against a temporary home — never your real `$HOME`:

```sh
dots install --home /tmp/dots-home --source-root "$PWD" --state-root /tmp/dots-state
dots status  --home /tmp/dots-home --source-root "$PWD" --state-root /tmp/dots-state
```
