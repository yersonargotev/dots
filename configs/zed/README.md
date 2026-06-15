# Zed configuration

`configs/zed/` is the repository-managed authored configuration for the
[Zed](https://zed.dev) editor, installed under `~/.config/zed/` with the
`symlink` strategy (`desktop` tag, `darwin`+`linux`). Managed files:

| Source | Target |
| --- | --- |
| `configs/zed/settings.json` | `~/.config/zed/settings.json` |
| `configs/zed/keymap.json` | `~/.config/zed/keymap.json` |
| `configs/zed/themes/catppuccin-blue.json` | `~/.config/zed/themes/catppuccin-blue.json` |

The Source of Truth is this repository. The installed files are links to the
tracked sources; runtime state, generated files, conversations, prompts, and
compiled extensions stay outside version control.

## Prerequisites

- **`zed`** — declared as an advisory dependency for the desktop profile.
- **CaskaydiaCove Nerd Font** — declared as an advisory dependency. Zed degrades
  to a fallback font when it is absent, so `dots install` does not fail when the
  font is missing.

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
| **Machine-specific** | `buffer_font_family` and font sizes. Zed has no user-level local include (no `settings.local.json`), so these stay in the shared `settings.json`; the font is declared as an advisory dependency instead of extracted. | Kept in `settings.json`; font handled via dependency declaration. |
| **Generated** | `conversations/`, `prompts/`, compiled `extensions/`, extension index/DB, caches, logs | Never committed (see `.gitignore`). |
| **Private** | secrets, authenticated state (Copilot auth lives outside the authored files), private paths, machine IDs | Excluded. |

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

The expected result is that the three Zed files are installed/aligned inside
`$sandbox_home` as symlinks to the repository sources. The maintainer's real home
directory must not be touched.
