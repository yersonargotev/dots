# bat configuration

bat is a **single-file** slice (issue #46, epic #37). The entire portable
footprint is one managed config file:

```
# configs/bat/config
--theme="Catppuccin Mocha"
```

`dots` symlinks it to `~/.config/bat/config`. There is **no managed theme file**
and **no managed cache** — the only thing this slice owns is the theme selection,
and the theme it selects ships **inside bat itself**.

## Why the theme is selected but not vendored

`Catppuccin Mocha` is a **built-in bat theme** as of **bat 0.25**. On the
Source-of-Truth machine (bat 0.26.1), `--theme="Catppuccin Mocha"` resolves to
the built-in theme with no custom theme file present — verified against an empty
config directory:

```sh
echo test | BAT_CONFIG_DIR="$(mktemp -d)" bat --theme="Catppuccin Mocha" -p -l txt
# renders with Catppuccin Mocha colors, exit 0
```

So the config line is portable on its own: any machine with bat ≥ 0.25 renders it
identically without shipping a single byte of theme data.

## Why there is no managed theme file or cache

The live machine still carries leftover state from before bat bundled
Catppuccin. It is **intentionally excluded** from the repo:

| Live artifact | Category | Repository decision |
| --- | --- | --- |
| `~/.config/bat/config` (`--theme="Catppuccin Mocha"`) | **Portable / source-of-truth** | Managed as `configs/bat/config`, symlinked via `dots.yaml`. |
| `~/.config/bat/themes/Catppuccin Mocha.tmTheme` (~66 KB) | **Downloaded / redundant** | **Excluded.** The theme is built-in since bat 0.25, so the file is dead weight, not source-owned. Versioning it would be copying machine state verbatim — exactly what epic #37 rejects. |
| `~/.cache/bat/` (`syntaxes.bin`, `themes.bin`, `metadata.yaml`) | **Generated** (`bat cache --build`) | **Excluded.** Regenerable, and only existed to register the redundant custom theme. With the built-in theme there is nothing to build. |
| Shell alias (`cat`→`bat`), `BAT_THEME` / `MANPAGER` / `BAT_*` env | **Not in use** | Nothing to migrate. If ever adopted, they belong to the Zsh slice or a future slice — never invented here. |

Contrast with the **atuin** slice, which *does* vendor `catppuccin-mocha.toml`:
atuin's Catppuccin theme is **not** built-in, so the file is genuine
source-of-truth there. bat is the opposite case — same palette, different
ownership, because the evidence differs.

## Version floor and fallback

This slice relies on bat's **built-in** Catppuccin themes, available from
**bat ≥ 0.25**. The `dots.yaml` entry declares a `bat` dependency
(`brew` / `apt` / `dnf` / `pacman`), and the runtime `command: bat` guard keeps
install/status safe when bat is absent.

On an **older bat** (some distro packages still ship < 0.25, e.g. older Ubuntu
LTS), the config degrades **gracefully**: bat parses `--theme="Catppuccin Mocha"`,
finds no matching theme, and falls back to its default theme. The shell and
`bat` stay usable — there is **no error and no broken startup**. The slice does
not hard-pin a minimum version; a graceful fallback is preferred over install
friction.

## Local machine overrides

Layer machine-specific bat settings without touching this slice:

- A different theme on one host: set it in a local, uncommitted bat config, or
  export `BAT_THEME` from `~/.zshrc.local`.
- A genuinely custom theme: drop the `.tmTheme` into `~/.config/bat/themes/`,
  run `bat cache --build`, and keep both out of the repo (the cache is generated;
  the theme is host-local) unless it becomes portable for every machine — in
  which case promote it into a dedicated slice with real evidence behind it.

The redundant `~/.config/bat/themes/Catppuccin Mocha.tmTheme` already on the live
machine can be removed manually at your convenience; this slice deliberately does
**not** touch the real `$HOME`.

## Sandbox validation

Validate against temporary directories with explicit flags — never the real
`$HOME` or `~/.config/bat` (per `AGENTS.md`):

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

- `install` and `status` run clean and report the bat entry as **aligned**.
- `$sandbox_home/.config/bat/config` is a symlink into the repo and resolves the
  **built-in** Catppuccin Mocha theme:
  `echo test | BAT_CONFIG_DIR="$sandbox_home/.config/bat" bat -p -l txt` renders
  with Catppuccin colors and exits 0.
- **No** `~/.config/bat/themes/` and **no** `~/.cache/bat/` artifact is created
  by `dots` — there is no such entry in `dots.yaml`.
- The maintainer's real home directory is never touched.
