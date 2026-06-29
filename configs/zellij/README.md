# Zellij configuration

`configs/zellij/config.kdl` is the repository-managed Zellij configuration
installed as `~/.config/zellij/config.kdl` (symlink strategy, `core` tag,
`darwin`+`linux`). `configs/zellij/layouts/default.kdl` is installed as
`~/.config/zellij/layouts/default.kdl` and selected by `default_layout "default"`.

The Zellij UX intentionally follows the repository-managed tmux configuration
where Zellij has practical equivalents: `C-a` is the primary multiplexer prefix,
common pane/tab actions keep tmux-like keys, and the status bar uses a
Catppuccin `zjstatus` layout at the top of the screen. `Ctrl+g` remains as a
compatibility entry point for the classic Zellij command flow.

The Source of Truth is this repository. The installed files are links to these
tracked sources; runtime state and downloaded plugins stay outside version
control.

## Prerequisites

- **`zellij`** — declared as the entry dependency; `dots` reports it across
  brew/apt/dnf/pacman but never installs it automatically.
- **`nvim`** — used by `scrollback_editor "nvim"`. It is an editor preference,
  not a managed Zellij dependency in this slice.
- **`zjstatus`** — optional/manual prerequisite for the default layout status
  bar. Place the plugin binary at `~/.config/zellij/plugins/zjstatus.wasm` on
  machines that want the tmux-like status layout exactly. The `.wasm` binary is
  intentionally not committed, downloaded, or provisioned by this repository;
  it is a future vendoring candidate only if the repository gets a deliberate
  plugin-binary policy.

## Portability classification

The live `~/.config/zellij` directory was classified before adoption:

| Category | Examples | Repository decision |
| --- | --- | --- |
| **Portable** | locked-by-default mode, tmux-like `C-a` prefix, `Ctrl+g` compatibility command flow, pane/tab/resize/move/scroll keybindings, Alt navigation shortcuts, Catppuccin Mocha theme, rounded pane frames, mouse mode, scroll buffer size, Neovim scrollback editor, built-in plugin aliases, default layout selection, and the tmux-like personal `zjstatus` status bar layout | Managed in `configs/zellij/config.kdl` and `configs/zellij/layouts/default.kdl`. |
| **Machine-specific** | absolute user paths, hostnames, machine IDs, per-host overrides | **Excluded.** Use a separate config file or config directory through real Zellij mechanisms when a host needs divergence. |
| **Generated** | downloaded `.wasm` plugin binaries, `plugexit`, plugin-manager output, caches, logs, sessions, serialized runtime state, backup files such as `config.kdl.bak-*` | **Never committed.** Runtime files live under the user's Zellij data/config locations. |
| **Private** | secrets, tokens, local-only paths, private command wiring | **Excluded.** Keep them outside the managed files. |

## Behavior carried by the portable config

- Zellij starts in `default_mode "locked"` so most terminal applications receive
  normal bindings. The deliberate exception is direct tmux-style navigation:
  `Ctrl+h/j/k/l` moves between Zellij panes without a prefix.
- `C-a` is the primary tmux-like multiplexer prefix. From locked or normal mode
  it enters Zellij's `tmux` mode for one command, then most commands return to
  locked mode.
- `Ctrl+g` remains as the compatibility entry point for the classic Zellij
  command flow: from locked mode it enters normal mode, and from active command
  modes it returns to locked mode.
- The UI uses the built-in `catppuccin-mocha` theme, rounded pane frames,
  `mouse_mode true`, and `scroll_buffer_size 10000`.
- Scrollback opens in Neovim via `scrollback_editor "nvim"`.
- The default layout includes a top `zjstatus` status bar with an intentional
  tmux-like left/center/right shape: mode and session on the left, tabs in the
  center, and git branch plus `America/Bogota` time on the right. That timezone
  is portable because it is an explicit preference, not host detection.

## Tmux-like keybinding map

These bindings intentionally mirror `configs/tmux/tmux.conf` where Zellij has a
matching concept. Zellij is still modal, so this is practical parity rather than
exact emulation.

| tmux habit | Zellij binding | Zellij action | Notes |
| --- | --- | --- | --- |
| Prefix | `C-a` | enter `tmux` mode | Primary path for tmux-like commands. |
| Classic Zellij command flow | `Ctrl+g` | enter/leave normal command mode | Compatibility path for existing Zellij muscle memory. |
| Horizontal/right split | `C-a v` or `C-a %` | `NewPane "right"` | `v` matches this repo's tmux config; `%` matches stock tmux. |
| Vertical/down split | `C-a d` or `C-a "` | `NewPane "down"` | `d` matches this repo's tmux config; `"` matches stock tmux. |
| Detach | `C-a D` | `Detach` | Matches this repo's tmux `D` binding. |
| New tab/window | `C-a c` | `NewTab` | Zellij tabs are the closest equivalent to tmux windows. |
| Rename tab/window | `C-a ,` | rename tab | Mirrors tmux rename-window habit. |
| Previous/next tab | `C-a p` / `C-a n` | previous/next tab | Zellij tab navigation equivalent. |
| Direct previous/next tab | `Cmd+h` / `Cmd+l` when forwarded as `Super+h` / `Super+l` | previous/next tab | Mirrors the maintainer's direct tmux window navigation habit. Some terminals or macOS defaults may reserve Cmd keys before Zellij sees them. |
| Focus pane | `C-a h/j/k/l` | move focus | Zellij pane focus equivalent. |
| Direct focus pane | `Ctrl+h/j/k/l` | move focus | No-prefix navigation equivalent to the tmux/vim-tmux-navigator habit. |
| Resize pane | `C-a H/J/K/L` | resize focused pane | Zellij resize actions use directional growth/shrink semantics. |
| Fullscreen/zoom | `C-a z` | toggle fullscreen | Closest equivalent to tmux zoom. |
| Close focused pane | `C-a x` | close focus | Zellij close equivalent. |
| Scrollback | `C-a [` | scroll mode | Mirrors tmux copy-mode entry habit. |
| Tab selection | `C-a 1` … `C-a 9` | go to tab | Uses Zellij's 1-based tab selection. |

Unavoidable differences: Zellij tabs are not tmux windows, Zellij modes remain
visible in the status bar, and plugin binaries such as `zjstatus.wasm` are
runtime files rather than TPM-style plugin declarations managed by this repo.

## Local/private overrides

Zellij does **not** have a native `config.local.kdl` include mechanism. Do not
invent one: Zellij will not read it automatically.

Use real Zellij mechanisms instead:

```sh
# Point at one private config file.
ZELLIJ_CONFIG_FILE="$HOME/.config/zellij-private/config.kdl" zellij
zellij --config "$HOME/.config/zellij-private/config.kdl"

# Point at a private config directory containing config.kdl and layouts/.
ZELLIJ_CONFIG_DIR="$HOME/.config/zellij-private" zellij
zellij --config-dir "$HOME/.config/zellij-private"
```

Those private files are machine-local. They must not be committed to this
repository unless they are reclassified as portable.

## Sandbox validation

Validate Zellij config changes with temporary directories — never the real
`$HOME` or `~/.config/zellij` (per `AGENTS.md`):

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

The expected result is that both `~/.config/zellij/config.kdl` and
`~/.config/zellij/layouts/default.kdl` are installed/aligned inside
`$sandbox_home` as symlinks to repository sources. The maintainer's real home
directory must not be touched.
