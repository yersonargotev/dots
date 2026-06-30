# Neovim configuration

The managed LazyVim/Catppuccin setup defaults to `catppuccin-mocha`. When the
`adaptive-theme` tag has installed `~/.config/dots/adaptive-theme`, Neovim checks
macOS appearance at startup and selects `catppuccin-latte` only when light mode is
proven. macOS dark mode, Linux/non-macOS, missing `defaults`, unknown values, and
tag absence keep Mocha.

The check happens inside `lua/plugins/colorscheme.lua`; no real `$HOME` files are
needed to validate it beyond a temporary marker path such as
`DOTS_ADAPTIVE_THEME_MARKER=/tmp/.../adaptive-theme`.
