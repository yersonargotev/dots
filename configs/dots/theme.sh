#!/usr/bin/env sh
# Shared dots adaptive-theme detection and Catppuccin palette helpers for
# shell-readable managed config. Opt-in is selected by installing
# ~/.config/dots/adaptive-theme through the manifest tag of the same name.
# Mocha/dark remains the safe fallback.

dots_adaptive_theme_marker=${DOTS_ADAPTIVE_THEME_MARKER:-"$HOME/.config/dots/adaptive-theme"}

dots_adaptive_theme_enabled() {
  [ -f "$dots_adaptive_theme_marker" ]
}

dots_macos_light_appearance() {
  [ "$(uname -s 2>/dev/null)" = "Darwin" ] || return 1
  command -v defaults >/dev/null 2>&1 || return 1
  case "$(defaults read -g AppleInterfaceStyle 2>/dev/null)" in
    "") return 0 ;;
    *) return 1 ;;
  esac
}

dots_catppuccin_flavor() {
  if dots_adaptive_theme_enabled && dots_macos_light_appearance; then
    printf '%s\n' latte
  else
    printf '%s\n' mocha
  fi
}

dots_catppuccin_ansi_palette() {
  case "${1:-$(dots_catppuccin_flavor)}" in
    latte)
      cat <<'PALETTE'
blue='\033[38;2;30;102;245m'
mauve='\033[38;2;136;57;239m'
green='\033[38;2;64;160;43m'
yellow='\033[38;2;223;142;29m'
subtext='\033[38;2;92;95;119m'
overlay='\033[38;2;156;160;176m'
red='\033[38;2;210;15;57m'
pink='\033[38;2;234;118;203m'
PALETTE
      ;;
    *)
      cat <<'PALETTE'
blue='\033[38;2;137;180;250m'
mauve='\033[38;2;203;166;247m'
green='\033[38;2;166;227;161m'
yellow='\033[38;2;249;226;175m'
subtext='\033[38;2;166;173;200m'
overlay='\033[38;2;108;112;134m'
red='\033[38;2;243;139;168m'
pink='\033[38;2;245;194;231m'
PALETTE
      ;;
  esac
}

dots_apply_catppuccin_ansi_palette() {
  eval "$(dots_catppuccin_ansi_palette "$@")"
  reset='\033[0m'
}
