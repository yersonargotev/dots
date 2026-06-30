# Ghostty configuration

`configs/ghostty/config.ghostty` is the repository-managed Ghostty terminal
configuration installed as `~/.config/ghostty/config.ghostty` (symlink strategy,
`desktop` tag, `darwin`+`linux`). It uses Ghostty's current canonical filename.

The Source of Truth is this repository. The installed file is a link to the
tracked source; runtime state, generated files, and machine-local overrides stay
outside version control.

## Prerequisites

- **`ghostty`** — declared as an advisory dependency for the desktop profile.
- **Desktop Nerd Font** — declared on the `desktop` profile as shared desktop
  infrastructure. Ghostty consumes this shared requirement for the managed
  `font-family` baseline (`Cascadia Code NF`). The primary macOS package is the
  Homebrew cask `font-cascadia-code-nf`, detected through `CascadiaCodeNF*` font
  files. Compatible current Nerd Fonts files such as
  `CaskaydiaCoveNerdFont*` also satisfy the dependency.

`dots install` does not install packages. `dots deps plan` and the explicit
`dots deps install` workflow can report/use Homebrew guidance where available;
Linux package mappings are intentionally omitted until package names are
verified for each distro.

## Portability classification

The live `~/.config/ghostty/config` file was classified before adoption:

| Category | Examples | Repository decision |
| --- | --- | --- |
| **Portable** | font family, font size, Catppuccin Mocha fallback theme, optional `adaptive-theme` native light/dark include, intentional keybindings for terminal workflow and Zellij/tmux forwarding | Managed in `configs/ghostty/config.ghostty`. |
| **Machine-specific** | window dimensions, opacity/blur, window padding ergonomics, explicit shell/command paths, initial working directories, OS integrations, display/GPU/host-dependent behavior | Excluded from the shared file; document deliberate host-specific exceptions in `configs/ghostty/config.local.ghostty.example`. |
| **Generated** | logs, caches, sessions, backups, temporary files, generated state, local shaders | Never committed. |
| **Private** | secrets, authenticated state, private paths, hostnames, machine IDs | Excluded. |


## Adaptive theme opt-in

Fresh installs keep the Mocha theme from `config.ghostty`. When the user selects
`--tag adaptive-theme`, dots also installs
`~/.config/ghostty/adaptive-theme.ghostty`, and the shared config includes it with
`config-file = ?adaptive-theme.ghostty`. That fragment uses Ghostty's native
`theme = light:catppuccin-latte,dark:catppuccin-mocha` syntax, so Ghostty follows
the desktop appearance while keeping Mocha as the dark fallback. The optional
include is absent unless the tag is selected, so default desktop installs do not
change theme behavior.

## Legacy config migration

Ghostty still loads the legacy `~/.config/ghostty/config` file for compatibility.
Because Ghostty loads both `config.ghostty` and legacy `config`, an existing
legacy file can override the repository-managed Source of Truth after install.

Before switching a real workstation to this slice, classify the legacy file, move
portable settings into `configs/ghostty/config.ghostty`, move machine-specific
settings into `~/.config/ghostty/config.local.ghostty`, then archive or remove
the legacy `~/.config/ghostty/config` file. Do not leave duplicated font, theme/color,
or keybinding settings in the legacy file because they can override the managed
Source of Truth. Do the same review for macOS
Application Support Ghostty config files if they exist.

Do not delete the legacy file blindly: it is user state until it has been
classified. The sandbox validation below starts from an empty temporary home, so
it proves safe installation and alignment, not cleanup of a real workstation's
legacy file.

## Local machine overrides

Ghostty supports split configuration with `config-file`. The shared file uses an
optional local include:

```ghostty
config-file = ?config.local.ghostty
```

The `?` prefix means a fresh install remains aligned even when the local file is
absent. When a machine needs private settings, copy the versioned example from the
repository into the Ghostty config directory manually:

```sh
mkdir -p ~/.config/ghostty
cp ~/.local/share/dots/configs/ghostty/config.local.ghostty.example \
  ~/.config/ghostty/config.local.ghostty
```

If you are working from a development checkout instead of the default installed
repository, replace `~/.local/share/dots` with that checkout path. Then edit
`~/.config/ghostty/config.local.ghostty` locally. Do not commit that file unless
the setting has been reviewed and reclassified as portable.

## Sandbox validation

Validate Ghostty config changes with temporary directories — never the real
`$HOME` or `~/.config/ghostty` (per `AGENTS.md`):

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
  --tag adaptive-theme \
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

The expected result is that `~/.config/ghostty/config.ghostty` is installed/aligned inside `$sandbox_home` as a symlink to the repository source; add `--tag adaptive-theme` on macOS to also install `~/.config/ghostty/adaptive-theme.ghostty`. Linux keeps the Mocha fallback because the adaptive fragment is Darwin-only.
The maintainer's real home directory must not be touched.
