#!/usr/bin/env bash
# Claude Code statusLine — plain text + dots-managed palette

input=$(cat)

model=$(echo "$input" | jq -r '.model.display_name // ""')
effort=$(echo "$input" | jq -r '.effort.level // ""')
used_pct=$(echo "$input" | jq -r '.context_window.used_percentage // empty')
total_tokens=$(echo "$input" | jq -r '.context_window.total_input_tokens // empty')
five_h=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // empty')
five_h_resets=$(echo "$input" | jq -r '.rate_limits.five_hour.resets_at // empty')
seven_d=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // empty')
seven_d_resets=$(echo "$input" | jq -r '.rate_limits.seven_day.resets_at // empty')
vim_mode=$(echo "$input" | jq -r '.vim.mode // ""')
perm_mode=$(echo "$input" | jq -r '.permission_mode // .permissionMode // ""')

# This status line always uses Catppuccin, independently of the app theme.
if [ -r "$HOME/.config/dots/theme.sh" ]; then
  # shellcheck source=/dev/null
  . "$HOME/.config/dots/theme.sh"
  dots_apply_catppuccin_ansi_palette
else
  blue='\033[38;2;137;180;250m'
  mauve='\033[38;2;203;166;247m'
  green='\033[38;2;166;227;161m'
  yellow='\033[38;2;249;226;175m'
  subtext='\033[38;2;166;173;200m'
  overlay='\033[38;2;108;112;134m'
  red='\033[38;2;243;139;168m'
  pink='\033[38;2;245;194;231m'
  reset='\033[0m'
fi

sep=''
separator() {
  printf '%b' "$sep"
  sep="${overlay} | ${reset}"
}

# --- Build output ---

# Model first
if [ -n "$model" ]; then
  separator
  printf "${subtext}%s${reset}" "$model"
  if [ -n "$effort" ]; then
    printf " ${mauve}(%s)${reset}" "$effort"
  fi
fi

# Vim mode
if [ -n "$vim_mode" ]; then
  separator
  case "$vim_mode" in
  NORMAL) printf "${red}N${reset}" ;;
  INSERT) printf "${pink}I${reset}" ;;
  VISUAL) printf "${mauve}V${reset}" ;;
  REPLACE) printf "${yellow}R${reset}" ;;
  *) printf "${subtext}%s${reset}" "$vim_mode" ;;
  esac
fi

# Permission mode
case "$perm_mode" in
default)
  separator
  printf "${subtext}default${reset}"
  ;;
auto)
  separator
  printf "${green}auto${reset}"
  ;;
bypassPermissions)
  separator
  printf "${pink}bypass${reset}"
  ;;
acceptEdits)
  separator
  printf "${green}accept edits${reset}"
  ;;
plan)
  separator
  printf "${yellow}plan${reset}"
  ;;
esac

# Context fields can arrive independently, including valid zero values.
if [ -n "$used_pct" ] || [ -n "$total_tokens" ]; then
  separator
  printf "${yellow}ctx:"
  if [ -n "$total_tokens" ]; then
    ctx_k=$(echo "$total_tokens" | awk '{printf "%.1fk", $1 / 1000}')
    printf ' %s' "$ctx_k"
  fi
  if [ -n "$used_pct" ]; then
    printf ' %.1f%%' "$used_pct"
  fi
  printf '%b' "$reset"
fi

# 5-hour rate limit
if [ -n "$five_h" ]; then
  separator
  if [ -n "$five_h_resets" ]; then
    now=$(date +%s)
    diff_secs=$(( five_h_resets - now ))
    [ "$diff_secs" -lt 0 ] && diff_secs=0
    hours_left=$(( diff_secs / 3600 ))
    minutes_left=$(( (diff_secs % 3600) / 60 ))
    five_h_left=$(echo "$five_h" | awk '{printf "%.0f", 100 - $1}')
    printf "${blue}${hours_left}h:${minutes_left}m ${five_h_left}%% left${reset} ${overlay}> %s${reset}" "$(date -r "$five_h_resets" +%H:%M 2>/dev/null || date -d "@$five_h_resets" +%H:%M 2>/dev/null)"
  else
    five_h_left=$(echo "$five_h" | awk '{printf "%.0f", 100 - $1}')
    printf "${blue}5h ${five_h_left}%% left${reset}"
  fi
fi

# 7-day rate limit
if [ -n "$seven_d" ]; then
  separator
  if [ -n "$seven_d_resets" ]; then
    now=$(date +%s)
    diff_secs=$(( seven_d_resets - now ))
    [ "$diff_secs" -lt 0 ] && diff_secs=0
    days_left=$(( diff_secs / 86400 ))
    hours_left=$(( (diff_secs % 86400) / 3600 ))
    if [ "$days_left" -gt 0 ]; then
      countdown="${days_left}d ${hours_left}h"
    else
      countdown="${hours_left}h"
    fi
    seven_d_left=$(echo "$seven_d" | awk '{printf "%.0f", 100 - $1}')
    printf "${blue}%s ${seven_d_left}%% left${reset} ${overlay}> %s${reset}" "$countdown" "$(date -r "$seven_d_resets" +%d/%m 2>/dev/null || date -d "@$seven_d_resets" +%d/%m 2>/dev/null)"
  else
    seven_d_left=$(echo "$seven_d" | awk '{printf "%.0f", 100 - $1}')
    printf "${blue}7d ${seven_d_left}%% left${reset}"
  fi
fi

printf "\n"
exit 0
