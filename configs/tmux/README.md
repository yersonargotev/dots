# Tmux configuration

`configs/tmux/tmux.conf` is the repository-managed Tmux configuration installed
as `~/.tmux.conf` (symlink strategy, `core` tag, `darwin`+`linux`). It carries
the portable, auditable setup so a fresh machine gets a usable Tmux without the
maintainer's machine state.

## Prerequisites

- **`tmux`** — declared as the entry dependency; `dots` reports it across
  brew/apt/dnf/pacman but never installs it automatically.
- **TPM (Tmux Plugin Manager)** — the config *declares* plugins via `@plugin`
  lines and loads TPM only when `~/.tmux/plugins/tpm/tpm` exists, but it does
  not install TPM. Clone it once per machine, then press `prefix + I` (here
  `C-a I`) inside tmux to fetch the declared plugins:

  ```sh
  git clone https://github.com/tmux-plugins/tpm ~/.tmux/plugins/tpm
  ```

- **A Nerd Font** — the Catppuccin status modules use Nerd Font glyphs. Install
  and select a [Nerd Font](https://www.nerdfonts.com/) in your terminal for the
  status line to render correctly. This is a documented expectation, not a
  machine-specific path.

## Catppuccin light/dark behavior

The managed config uses Catppuccin Latte when macOS is in light appearance and
Catppuccin Mocha when macOS is in dark appearance. The selection is made when
Tmux sources this config with `defaults read -g AppleInterfaceStyle`: macOS
reports `Dark` only for dark mode, so any other value means Latte. Non-macOS
hosts fall back to Mocha. Re-source `~/.tmux.conf` or restart Tmux after
changing the system appearance.

Custom window-status formats use Catppuccin theme variables instead of fixed
Mocha hex values so the same status bar adapts to Latte or Mocha.

## Portability classification

The live `~/.tmux.conf` (a full Gentleman.Dots setup) was walked line by line
before adoption and bucketed into four categories:

| Category | Examples | Repository decision |
| --- | --- | --- |
| **Portable** | prefix remap `C-a`, vi copy-mode + clipboard binding, split/pane keymaps (`v`/`d`/`D`), window nav (`M-h`/`M-l`), pane resize (`M-H/J/K/L`), `mouse`, status position/lengths, base/pane index, terminal/key options (`default-terminal`, `terminal-features`, `extended-keys`, `escape-time`), the floating `scratch` popup, kill-other-sessions confirm, window-status formatting, the TPM `@plugin` declarations, the Catppuccin Latte/Mocha theme selection + status modules, and guarded `run` lines that load TPM/Catppuccin when their runtime files exist | Managed in `configs/tmux/tmux.conf`. |
| **Machine-specific** | `default-command`/`default-shell` hardcoding an absolute shell path (`/bin/zsh`); a status comment leaking `user@host` | **Excluded.** tmux inherits the login shell on its own; an absolute shell path is an installer concern. Put it in `~/.tmux.conf.local` if a host needs it. |
| **Generated** | everything under `~/.tmux/plugins/...` (downloaded plugin contents, TPM bootstrap output) | **Never committed.** Plugin *declarations* are versioned; downloaded *contents* are runtime state. The `run` lines that load them are config and stay. |
| **Private** | secrets, machine IDs, per-host overrides | **Excluded.** They live in the untracked `~/.tmux.conf.local`. |

## Local / private extension point

`~/.tmux.conf.local` is an optional, untracked file sourced **last** by a
guarded `if-shell` at the end of `tmux.conf`:

```tmux
if-shell "test -f ~/.tmux.conf.local" "source-file ~/.tmux.conf.local"
```

Because it is sourced after the portable defaults, anything you put there wins.
Use it for machine-specific tweaks that must never enter the repo — for example
an absolute `default-shell`, per-host keymaps, or a private status module:

```tmux
# ~/.tmux.conf.local (untracked, per-machine)
set -g default-shell "/opt/homebrew/bin/zsh"
```

The repository ships no `tmux.conf.local` and never tracks one; tmux silently
skips the `source-file` when the file is absent, so a fresh machine works
without it.

## Sandbox validation

Validate Tmux config changes with temporary directories — never the real
`$HOME` or `~/.tmux.conf` (per `AGENTS.md`):

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
  --home "$sandbox_home" \
  --source-root "$PWD" \
  --state-root "$sandbox_state"
```

The expected result is that `~/.tmux.conf` is installed/aligned inside
`$sandbox_home` as a symlink to the repository source. The maintainer's real
home directory must not be touched.
