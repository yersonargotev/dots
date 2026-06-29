# eza configuration

eza is an **alias-only** slice (issue #45, epic #37). It has **no
repository-managed config file** — the portable footprint is guarded shell
aliases owned by the **Zsh slice**, while `dots.yaml` declares the `eza` binary
as part of the core tool baseline.

```zsh
# configs/zsh/rc.d/post/50-aliases.zsh
if command -v eza >/dev/null 2>&1; then
  alias ls='eza -a --icons=always --color=always --grid --group-directories-first'
  alias ll='eza -la --icons=always --color=always --group-directories-first --git'
  alias lt='eza -a --icons=always --color=always --tree --level=2 --group-directories-first'
fi
```

The `command -v eza` guard keeps the shell usable when eza is absent, so a fresh
machine without eza still starts cleanly and `ls` falls back to the system
binary. This slice manages only the **decision and its rationale** — there is no
file for `dots` to install.

## Why there is no managed config file

eza is configured two ways, neither of which produces a portable file worth
versioning here:

1. **CLI flags** — all behavior in use (`-a`, `--icons`, `--color`, `--grid`,
   `--group-directories-first`) is expressed inline in the alias above. The alias
   already lives in the Zsh slice, so there is nothing else to manage.
2. **`theme.yml`** — eza's only standalone config surface is a colors-only
   `theme.yml`, discovered via `EZA_CONFIG_DIR` (default `~/.config/eza/`). The
   Source of Truth machine **does not use one**: there is no `~/.config/eza/`
   directory, no `theme.yml`, and no `EZA_*` environment variables. Colors come
   from the inline `--color=always` flag plus the terminal palette.

Introducing a `theme.yml` that no machine actually runs would be **inventing
configuration**, which contradicts epic #37's principle of treating current
machine configuration as *input evidence, not source code to copy verbatim*. If
a custom eza theme is ever adopted on a real machine, that becomes its own
migration slice with real evidence behind it.

## Portability classification

The live eza usage was classified before adoption:

| Category | Examples | Repository decision |
| --- | --- | --- |
| **Portable** | the shared `ls`, `ll`, and `lt` alias flag sets | Owned by the Zsh slice in `configs/zsh/rc.d/post/50-aliases.zsh`, guarded by `command -v eza`. |
| **Machine-specific** | `EZA_CONFIG_DIR`, any per-host theme | None in use. Would belong in `~/.zshrc.local` (env var) if ever needed — never committed. |
| **Generated / private** | — | eza is stateless: no history, cache, tokens, or machine identity. Nothing to exclude. |

The alias set stays small and conventional by explicit maintainer decision:
`ls` keeps the grid default, `ll` adds long/all/git details without forcing grid
layout, and `lt` provides a shallow tree. More specialized views remain local
unless they become part of the shared workflow.

## Local machine overrides

If a machine ever needs eza-specific configuration, layer it without touching the
shared slice:

- A custom theme: set `EZA_CONFIG_DIR` and drop a `theme.yml` there, exporting the
  variable from `~/.zshrc.local` (never committed).
- Extra aliases: add them to `~/.zshrc.local`.

If a setting turns out to be portable for every machine, review it and promote it
into the appropriate shared slice instead of leaving it local.

## Sandbox validation

Because this slice adds **no** managed file, validation is a **negative
assertion**: prove that `dots` installs and reports alignment safely *without*
creating any eza artifact, and that the real `$HOME` is untouched (per
`AGENTS.md`). Use temporary directories plus explicit flags — never your real
`$HOME` or `~/.config/eza`:

```sh
sandbox_root="$(mktemp -d)"
sandbox_home="$sandbox_root/home"
sandbox_state="$sandbox_root/state"
mkdir -p "$sandbox_home" "$sandbox_state"

go run ./cmd/dots install \
  --profile default --yes \
  --home "$sandbox_home" --source-root "$PWD" --state-root "$sandbox_state"

go run ./cmd/dots status \
  --profile default \
  --home "$sandbox_home" --source-root "$PWD" --state-root "$sandbox_state"
```

Expected results:

- `install` and `status` run clean and report existing managed entries as aligned.
- **No** `~/.config/eza/` artifact is created inside `$sandbox_home` — eza has no
  managed config entry, only a dependency declaration and shell aliases.
- The maintainer's real home directory is never touched.

The alias-guard behavior is owned by the Zsh slice: sourcing
`configs/zsh/rc.d/post/50-aliases.zsh` defines eza aliases only when eza is
present and starts the shell without error when it is absent.
