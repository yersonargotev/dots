# Neovim configuration

The managed LazyVim/Catppuccin setup defaults to `catppuccin-mocha`. When the
`adaptive-theme` tag has installed `~/.config/dots/adaptive-theme`, Neovim checks
macOS appearance at startup and selects `catppuccin-latte` only when light mode is
proven. macOS dark mode, Linux/non-macOS, missing `defaults`, unknown values, and
tag absence keep Mocha.

The check happens inside `lua/plugins/colorscheme.lua`; no real `$HOME` files are
needed to validate it beyond a temporary marker path such as
`DOTS_ADAPTIVE_THEME_MARKER=/tmp/.../adaptive-theme`.

`~/.config/nvim/init.lua` is a regular loader that reads this Managed
Configuration through `~/.config/dots/nvim`. lazy.nvim writes its lockfile to
`stdpath("state")/lazy-lock.json`, which resolves beneath `$XDG_STATE_HOME`
(normally `~/.local/state/nvim/lazy-lock.json`). dots seeds that runtime file,
advances it only while it still matches the recorded baseline, and preserves
local plugin revisions and the lockfile itself during uninstall.
