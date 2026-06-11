# Zellij configuration

`configs/zellij/config.kdl` is the repository-managed Zellij configuration
installed as `~/.config/zellij/config.kdl` (symlink strategy, `core` tag,
`darwin`+`linux`). `configs/zellij/layouts/default.kdl` is installed as
`~/.config/zellij/layouts/default.kdl` and selected by `default_layout "default"`.

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
  machines that want this layout exactly. The `.wasm` binary is intentionally
  not committed; it is a future vendoring candidate only if the repository gets
  a deliberate plugin-binary policy.

## Portability classification

The live `~/.config/zellij` directory was classified before adoption:

| Category | Examples | Repository decision |
| --- | --- | --- |
| **Portable** | locked-by-default mode, `Ctrl+g` command flow, pane/tab/resize/move/scroll keybindings, Alt navigation shortcuts, Catppuccin Mocha theme, rounded pane frames, mouse mode, scroll buffer size, Neovim scrollback editor, built-in plugin aliases, default layout selection, and the personal status bar layout | Managed in `configs/zellij/config.kdl` and `configs/zellij/layouts/default.kdl`. |
| **Machine-specific** | absolute user paths, hostnames, machine IDs, per-host overrides | **Excluded.** Use a separate config file or config directory through real Zellij mechanisms when a host needs divergence. |
| **Generated** | downloaded `.wasm` plugin binaries, `plugexit`, plugin-manager output, caches, logs, sessions, serialized runtime state, backup files such as `config.kdl.bak-*` | **Never committed.** Runtime files live under the user's Zellij data/config locations. |
| **Private** | secrets, tokens, local-only paths, private command wiring | **Excluded.** Keep them outside the managed files. |

## Behavior carried by the portable config

- Zellij starts in `default_mode "locked"` so Neovim/LazyVim receives normal
  bindings without terminal multiplexer interference.
- `Ctrl+g` moves from locked mode into Zellij normal mode; `Ctrl+g`, `Esc`, or
  completing most commands returns to locked mode.
- The UI uses the built-in `catppuccin-mocha` theme, rounded pane frames,
  `mouse_mode true`, and `scroll_buffer_size 10000`.
- Scrollback opens in Neovim via `scrollback_editor "nvim"`.
- The default layout includes a `zjstatus` status bar with the intentional
  personal timezone preference `America/Bogota`. That timezone is portable
  because it is an explicit preference, not host detection.

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
