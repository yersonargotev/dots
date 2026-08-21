# Zed configuration

`configs/zed/` is the repository-managed authored configuration for the
[Zed](https://zed.dev) editor (`zed` Tag, `darwin`+`linux`). Zed settings
are a regular co-owned JSONC target; keymaps and the authored theme remain
repository-owned symlinks:

| Source | Target | Strategy / ownership |
| --- | --- | --- |
| `configs/zed/settings.json` | `~/.config/zed/settings.json` | `copy` / `jsonc-subset` |
| `configs/zed/keymap.json` | `~/.config/zed/keymap.json` | `symlink` / whole target |
| `configs/zed/themes/catppuccin-blue.json` | `~/.config/zed/themes/catppuccin-blue.json` | `symlink` / whole target |

The Source of Truth owns the portable settings baseline. Zed may add object
keys to its regular target without writing through to the Installed Repository.
Dots preserves those keys, comments, trailing commas, and untouched formatting;
owned scalars and arrays remain atomic ordered values. Runtime state, generated
files, conversations, prompts, and compiled extensions stay outside version
control.

## Prerequisites

- **`zed`** — declared as an advisory dependency for the atomic `zed` Tag.
- **Desktop Nerd Font** — selected internally by the `ghostty`, `warp`, and
  `zed` Tags, so any one of those desktop slices can satisfy the same shared font
  requirement without selecting Zed. The primary macOS package is the Homebrew
  cask `font-cascadia-code-nf`, detected through `CascadiaCodeNF*` font files.
  Compatible installed files such as `CaskaydiaCoveNerdFont*` also satisfy the
  dependency. This does not manage VS Code configuration.

`dots install` does not install packages. `dots deps plan` and the explicit
`dots deps install` workflow can report/use Homebrew guidance where available;
Linux package mappings are intentionally omitted until package names are verified
for each distro.

## Self-provisioned extensions

`settings.json` declares `auto_install_extensions` so a fresh Zed install pulls
the extensions the config depends on instead of vendoring extension binaries:

```jsonc
"auto_install_extensions": {
  "astro": true,
  "catppuccin": true,
  "catppuccin-icons": true,
  "git-firefly": true,
  "html": true,
  "scss": true
}
```

The custom theme `Catppuccin Mocha (blue)` referenced by the dark theme is the
exception: it is vendored as `themes/catppuccin-blue.json` because it is authored
material, not a downloadable extension. Without it the dark theme breaks.

## Portability classification

The live `~/.config/zed/` directory was classified before adoption:

| Category | Examples | Repository decision |
| --- | --- | --- |
| **Portable** | panel docks, `vim_mode`, `theme` (system/One Light/Catppuccin Mocha (blue)), `icon_theme`, `ui_font_family`, Copilot/agent preferences, declared extensions, the two intentional keybindings, the custom Catppuccin blue theme | Managed in `configs/zed/`. |
| **Machine-specific** | `buffer_font_family` and font sizes. Zed has no user-level local include (no `settings.local.json`), so these stay in the shared `settings.json`; the Nerd Font is modeled as a shared desktop advisory dependency instead of Zed-owned state. | Kept in `settings.json`; font handled by the shared dependency declaration selected by each desktop application Tag. |
| **Generated** | `conversations/`, `prompts/`, compiled `extensions/`, extension index/DB, caches, logs | Never committed (see `.gitignore`). |
| **Private** | secrets, authenticated state (Copilot auth lives outside the authored files), private paths, machine IDs | Excluded. |


## Resolving existing local Zed files

If `dots status --profile desktop` reports a Zed target as `conflict`, the
profile is only partially managed. A compatible regular `settings.json` can be
reconciled only when Installation Metadata proves its prior dots contribution;
untrusted or incompatible content remains a safety stop. Choose the resolution
explicitly:

| Choice | Effect | Tradeoff |
| --- | --- | --- |
| **Skip / keep local** | Leaves the existing `~/.config/zed/...` file untouched for this run. | Safest when you are unsure, but Zed remains outside the completed managed state. |
| **Replace** | Creates a Backup Set, then replaces the local target with the selected managed form. | Fastest path to repository ownership; review backups before deleting anything permanently. |
| **Adopt** | For supported regular-file conflicts only: copies the existing local target into `configs/zed/...`; symlink entries then install their link. | Preserves your current local content, but it becomes shared Source of Truth and must be reviewed for machine-specific or private data. |
| **Diff** | Shows target versus source before choosing skip, replace, or adopt. | Read-only preview; it does not resolve the conflict by itself. |

Non-interactive `dots install --profile desktop --yes` and `dots update --yes`
remain conservative: unresolved conflicts are skipped by default and local Zed
files are not overwritten or adopted automatically.

### Sandbox rehearsal

Rehearse conflict handling with temporary roots first. Pass every root explicitly
so no real Zed configuration is read or written:

```bash
sandbox_root="$(mktemp -d)"
sandbox_home="$sandbox_root/home"
sandbox_state="$sandbox_root/state"
mkdir -p "$sandbox_home/.config/zed/themes" "$sandbox_state"
printf '{"local": true}\n' > "$sandbox_home/.config/zed/settings.json"
printf '[]\n' > "$sandbox_home/.config/zed/keymap.json"
printf '{"theme": "local"}\n' > "$sandbox_home/.config/zed/themes/catppuccin-blue.json"

go run ./cmd/dots install \
  --profile desktop \
  --home "$sandbox_home" \
  --source-root "$PWD" \
  --state-root "$sandbox_state"
# or, without the TUI:
go run ./cmd/dots install \
  --profile desktop \
  --no-tui \
  --home "$sandbox_home" \
  --source-root "$PWD" \
  --state-root "$sandbox_state"
```

For each conflicting Zed target:

- choose **diff** when you need to compare the local file with `configs/zed/...`;
- choose **skip** when the local file should stay machine-owned for now;
- choose **replace** when the repository version should win;
- choose **adopt** only after checking the local file contains portable, safe
  configuration worth committing to this repository;
- choose **skip** or **replace** for unsupported cases such as directories or
  existing symlink leaves.

### Real migration

Run against your real home only when you are ready to make the migration decision:

```bash
dots install --profile desktop
# or, without the TUI:
dots install --profile desktop --no-tui
```

Replacement backups are recorded as Backup Sets under the state root, defaulting
to `~/.local/state/dots/backups/`. Inspect and restore them with:

```bash
dots backups list
dots backups restore <set> --dry-run
dots backups restore <set>
```

See [`docs/backups.md`](../../docs/backups.md) for restore behavior, safety
backups, provenance checks, and sandbox restore flags.

## Sandbox validation

Validate Zed config changes with temporary directories — never the real `$HOME`
or `~/.config/zed` (per `AGENTS.md`):

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

The expected result is that `settings.json` is an aligned regular co-owned file,
while keymap and theme are symlinks to the repository sources. The maintainer's
real home directory must not be touched.
