# Warp configuration

`configs/warp/settings.toml` and `configs/warp/keybindings.yaml` are the
repository-managed Warp terminal preference slice. They are copied into stable
channel config paths for macOS and Linux through the `desktop` profile.

The Source of Truth is intentionally small: theme, terminal font family,
terminal font size, and authored keybindings. Warp writes changes from its UI
back into these live config files, so `dots` uses `copy` instead of `symlink` to
prevent live Warp writes from mutating the repository source. Warp stores broader
account, cloud, workspace, agent, session, cache, database, log, and generated
runtime state near these files; that state is excluded from version control.

## Managed paths

| Platform | Source | Target |
| --- | --- | --- |
| macOS | `configs/warp/settings.toml` | `~/.warp/settings.toml` |
| macOS | `configs/warp/keybindings.yaml` | `~/.warp/keybindings.yaml` |
| Linux | `configs/warp/settings.toml` | `~/.config/warp-terminal/settings.toml` |
| Linux | `configs/warp/keybindings.yaml` | `~/.config/warp-terminal/keybindings.yaml` |

Preview channel paths and Windows paths are out of scope for this slice.

## Portability classification

The live Warp configuration must be classified before adoption:

| Category | Examples | Repository decision |
| --- | --- | --- |
| **Portable** | selected built-in theme, terminal font family, terminal font size, intentional custom keybindings | Managed in `configs/warp/settings.toml` and `configs/warp/keybindings.yaml`. |
| **Machine-specific** | custom theme absolute paths, shell overrides, working directories, window sizing, display ergonomics, OS integrations | Excluded unless a stable cross-platform representation is proven. |
| **Account/cloud** | account state, Settings Sync state, cloud workspace state, authenticated agent or team state | Never committed. |
| **Generated/runtime** | sessions, history, logs, caches, databases, Codebase Context indexes, MCP logs, temporary files | Never committed. |
| **Private** | secrets, tokens, private paths, hostnames, machine IDs | Excluded. |

Warp's documentation classifies `settings.toml` and `keybindings.yaml` as
non-portable config because the complete live files can contain preferences that
should not move between machines. `dots` manages only the authored portable
subset above; do not adopt a full live Warp config without reviewing every key.
If Warp later writes additional live settings, treat them as Drift and classify
them before updating the repository source.

The macOS entries intentionally do not declare a command dependency: Warp's
Homebrew cask installs `Warp.app`, but does not install a stable `warp` binary on
`PATH` by default. Until `dots` supports application-bundle dependency probes,
macOS Warp presence is documented but not auto-detected.

## Sandbox validation

Validate Warp config changes with temporary directories only. Never run against
the maintainer's real `$HOME` or live Warp configuration.

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
  --profile desktop \
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
  --profile desktop \
  --home "$sandbox_home" \
  --source-root "$PWD" \
  --state-root "$sandbox_state"
```
