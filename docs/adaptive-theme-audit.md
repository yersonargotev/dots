# Adaptive theme audit

Issue #275 introduces an explicit `adaptive-theme` tag. The Core Profile and
its atomic capabilities continue to prefer Catppuccin Mocha or app dark defaults. Adaptive behavior
must either be behind the tag or intentionally documented as out of scope for the
slice.

## Implemented behind `adaptive-theme`
- **Shared marker/helper**: `configs/dots/theme.sh` is selected by its `neovim`
  consumer and reads `~/.config/dots/adaptive-theme`, which is
  installed only by the `adaptive-theme` tag. The helper returns Latte only for
  opt-in macOS light appearance; everything else returns Mocha.
- **Ghostty**: default `configs/ghostty/config.ghostty` uses `theme =
  Catppuccin Mocha`. On macOS, the tag installs `adaptive-theme.ghostty`, which uses
  Ghostty's native `theme = light:Catppuccin Latte,dark:Catppuccin Mocha` form.
- **Neovim**: `lua/plugins/colorscheme.lua` selects `catppuccin-latte` only when
  the marker exists and macOS light appearance is proven; otherwise it uses
  `catppuccin-mocha`.
- **Herdr**: default `configs/herdr/config.toml` keeps Herdr on dark
  `catppuccin`. On macOS, the `adaptive-theme` tag selects
  `configs/herdr/config-adaptive.toml` for the same target, enabling Herdr's
  native `theme.auto_switch` with `catppuccin-latte` for light appearance and
  `catppuccin` for dark appearance. Herdr has no dots-owned include seam, so a
  manifest test keeps non-theme sections synchronized between both Herdr files.
- **Claude status line**: the copied status line script sources the helper
  and switches only its ANSI palette. Claude's app-level `theme` is `auto` so
  Claude can use its own light/dark support without dots rewriting the copied
  JSON settings file.

## Audited and left unchanged

- **Codex**: dots owns a TOML subset for status-line fields, not an app theme
  with a light/dark seam. No adaptive change was made.
- **bat**: bat supports native `auto:system` and `--theme-light/--theme-dark`,
  but the managed config has no include seam. Enabling it in the default config
  would make adaptive behavior untagged, so bat remains Mocha for this slice.
- **Starship**: no native include mechanism; the managed config remains Mocha.
- **Atuin**: no native include mechanism for the managed config; the managed
  config remains Mocha.
