# Adaptive theme audit

Issue #275 introduces an explicit `adaptive-theme` tag. The default profile/tag
sets continue to prefer Catppuccin Mocha or app dark defaults. Adaptive behavior
must either be behind the tag or intentionally documented as out of scope for the
slice.

## Implemented behind `adaptive-theme`

- **Shared marker/helper**: `configs/dots/theme.sh` is installed by `core` and
  reads `~/.config/dots/adaptive-theme`, which is installed only by the
  `adaptive-theme` tag. The helper returns Latte only for opt-in macOS light
  appearance; everything else returns Mocha.
- **tmux**: `configs/tmux/tmux.conf` sources the helper before setting
  `@catppuccin_flavor`. Missing helper/marker, macOS dark, Linux, unknown values,
  or missing `defaults` fall back to Mocha.
- **Ghostty**: default `configs/ghostty/config.ghostty` uses `theme =
  catppuccin-mocha`. On macOS, the tag installs `adaptive-theme.ghostty`, which uses
  Ghostty's native `theme = light:catppuccin-latte,dark:catppuccin-mocha` form.
- **Neovim**: `lua/plugins/colorscheme.lua` selects `catppuccin-latte` only when
  the marker exists and macOS light appearance is proven; otherwise it uses
  `catppuccin-mocha`.
- **Claude/Copilot statuslines**: copied statusline scripts source the helper
  and switch only their ANSI palettes. Claude's app-level `theme` remains
  `dark-ansi` because changing that setting dynamically would require rewriting
  the copied JSON settings file.

## Audited and left unchanged

- **Zellij**: default `configs/zellij/config.kdl` stays Mocha-only. Zellij has
  no safe dots-owned optional include seam in this symlinked config, and adding
  a second whole config would duplicate the Install Plan target surface, so the
  previous untagged light/dark switching was removed rather than moved.
- **Codex**: dots owns a TOML subset for status-line fields, not an app theme
  with a light/dark seam. No adaptive change was made.
- **OpenCode**: dots owns only the MCP overlay from ADR 0005. There is no
  dots-owned theme/status fragment to change without touching gentle-ai-owned
  global config, so it remains unchanged.
- **bat**: bat supports native `auto:system` and `--theme-light/--theme-dark`,
  but the managed config has no include seam. Enabling it in the default config
  would make adaptive behavior untagged, so bat remains Mocha for this slice.
- **Starship**: no native include mechanism; the managed config remains Mocha.
- **Atuin**: no native include mechanism for the managed config; the managed
  config remains Mocha.
- **Warp**: managed settings remain `theme = "dark"`; no safe optional include
  seam was introduced.
- **Zed**: already uses `"mode": "system"` with Catppuccin Mocha for dark and
  One Light for light. That existing app-native behavior is unchanged.
