# Atuin configuration

`configs/atuin/config.toml` is the repository-managed Atuin configuration
materialized as a regular `~/.config/atuin/config.toml` file (`copy` strategy,
TOML Subset Ownership, `atuin` tag, `darwin`+`linux`). The portable Catppuccin
Mocha theme it references remains a dots-owned symlink at
`~/.config/atuin/themes/catppuccin-mocha.toml`.

The Source of Truth owns only the baseline scalar and array values declared in
the tracked config. Atuin may add other keys or tables to the regular live file
without modifying the Installed Repository or creating Drift. Shell history,
sync/auth state, machine identity, generated files, and machine-local overrides
stay outside version control.

## Prerequisites

- **`atuin`** — declared as an advisory dependency for the `atuin` Tag.
  `dots install` does not install packages. `dots deps plan` and the explicit
  `dots deps install` workflow can report/use Homebrew guidance where available;
  Linux package mappings are intentionally omitted until package names are
  verified for each distro.

The shell integration is **not** part of this slice. It is a guarded hook owned
by Zsh in `configs/zsh/rc.d/post/40-tools.zsh`:

```zsh
[[ -r "${HOME}/.atuin/bin/env" ]] && source "${HOME}/.atuin/bin/env"
command -v atuin >/dev/null 2>&1 && eval "$(atuin init zsh)"
```

The `command -v atuin` guard keeps the shell usable when Atuin is absent, so a
fresh machine without Atuin still starts cleanly. This slice manages only the
declarative config and theme — never the `init` hook.

## Portability classification

The live `~/.config/atuin/config.toml` was classified before adoption:

| Category | Examples | Repository decision |
| --- | --- | --- |
| **Portable** | `enter_accept`, `filter_mode_shell_up_key_binding`, `[sync] records`, `[theme] name`, the custom `themes/catppuccin-mocha.toml` palette | Managed in `configs/atuin/config.toml` and `configs/atuin/themes/catppuccin-mocha.toml`. |
| **Machine-specific** | `db_path`, `key_path`, `session_path`, `data_dir`, `sync_address`, `auto_sync`, `timezone`, daemon socket/port | Excluded from the shared file. Set per machine via `ATUIN_*` env vars in `~/.zshrc.local`. |
| **Generated** | `history.db` (+ `-shm`/`-wal`), `records.db`, `last_version_check_time`, `latest_version`, `atuin-receipt.json`, daemon socket | Never committed. Live in `~/.local/share/atuin/`, which dots never manages. |
| **Private** | encryption `key`, auth `session` token, `host_id`, sync server credentials | Excluded. Live in `~/.local/share/atuin/` and are created/managed by Atuin itself. |

### Why history, sync tokens, and auth state are safe

Atuin stores its sensitive and generated state in a separate **data directory**
(`~/.local/share/atuin/`, or `$XDG_DATA_HOME/atuin`), not in the config
directory:

- **History database** — `~/.local/share/atuin/history.db`
- **Encryption key** — `~/.local/share/atuin/key`
- **Auth session token** — `~/.local/share/atuin/session`
- **Sync records** — `~/.local/share/atuin/records.db`
- **Machine identity** — `~/.local/share/atuin/host_id`

`dots` materializes the co-owned config and symlinks only the static theme into
`~/.config/atuin/`; it never reads, writes, or links anything in the data
directory. The
`configs/atuin/.gitignore` additionally guards the config directory against any
of these files being committed if a machine relocates them there via
`ATUIN_CONFIG_DIR` or a custom `data_dir`.

## Local machine overrides

Atuin has no `include` or local-override file mechanism. Configuration is layered
as: built-in defaults → `config.toml` → `ATUIN_*` environment variables (highest
priority). Nested keys use a double-underscore separator.

Set machine-specific values in `~/.zshrc.local` (not managed by dots):

```sh
# Examples — adjust per machine, never commit these.
export ATUIN_SYNC_ADDRESS="https://api.atuin.sh"
export ATUIN_TIMEZONE="local"
export ATUIN_SEARCH__MODE="fuzzy"
```

Verify the effective, merged value of any setting with:

```sh
atuin config get <key> --resolved
```

If a setting turns out to be portable for every machine, review it and move it
into `configs/atuin/config.toml` instead of leaving it in `~/.zshrc.local`.

## Sandbox validation

Validate Atuin config changes with temporary directories — never the real
`$HOME`, `~/.config/atuin`, or `~/.local/share/atuin` (per `AGENTS.md`):

```bash
sandbox_root="$(mktemp -d)"
sandbox_home="$sandbox_root/home"
sandbox_state="$sandbox_root/state"
sandbox_cache="$sandbox_root/cache"
sandbox_gopath="$sandbox_root/gopath"
sandbox_modcache="$sandbox_root/modcache"
mkdir -p "$sandbox_home" "$sandbox_state" "$sandbox_cache" "$sandbox_gopath" "$sandbox_modcache"

HOME="$sandbox_home" \
GOCACHE="$sandbox_cache" \
GOPATH="$sandbox_gopath" \
GOMODCACHE="$sandbox_modcache" \
XDG_CACHE_HOME="$sandbox_cache" \
go run ./cmd/dots install \
  --profile default \
  --yes \
  --home "$sandbox_home" \
  --source-root "$PWD" \
  --state-root "$sandbox_state"

HOME="$sandbox_home" \
GOCACHE="$sandbox_cache" \
GOPATH="$sandbox_gopath" \
GOMODCACHE="$sandbox_modcache" \
XDG_CACHE_HOME="$sandbox_cache" \
go run ./cmd/dots status \
  --profile default \
  --home "$sandbox_home" \
  --source-root "$PWD" \
  --state-root "$sandbox_state"
```

The expected result is that `~/.config/atuin/config.toml` and
`~/.config/atuin/themes/catppuccin-mocha.toml` are installed/aligned inside
`$sandbox_home`: the config is a regular co-owned TOML target and the static
theme is a symlink to the repository source. The maintainer's real home directory
must not be touched.
