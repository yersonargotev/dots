#!/usr/bin/env bash
# Claude Code statusLine — plain text + Catppuccin Mocha palette

input=$(cat)

model=$(echo "$input" | jq -r '.model.display_name // ""')
used_pct=$(echo "$input" | jq -r '.context_window.used_percentage // empty')
total_tokens=$(echo "$input" | jq -r '.context_window.total_input_tokens // empty')
five_h=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // empty')
five_h_resets=$(echo "$input" | jq -r '.rate_limits.five_hour.resets_at // empty')
seven_d=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // empty')
seven_d_resets=$(echo "$input" | jq -r '.rate_limits.seven_day.resets_at // empty')
vim_mode=$(echo "$input" | jq -r '.vim.mode // ""')
perm_mode=$(echo "$input" | jq -r '.permission_mode // .permissionMode // ""')

# Catppuccin Mocha palette
blue='\033[38;2;137;180;250m'
mauve='\033[38;2;203;166;247m'
green='\033[38;2;166;227;161m'
yellow='\033[38;2;249;226;175m'
subtext='\033[38;2;166;173;200m'
overlay='\033[38;2;108;112;134m'
red='\033[38;2;243;139;168m'
pink='\033[38;2;245;194;231m'
reset='\033[0m'

sep="${overlay} | ${reset}"

# --- Build output ---

# Model first
if [ -n "$model" ]; then
  printf "${subtext}%s${reset}" "$model"
fi

# Vim mode
if [ -n "$vim_mode" ]; then
  printf "$sep"
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
bypassPermissions)
  printf "${sep}${pink}bypass${reset}"
  ;;
acceptEdits)
  printf "${sep}${green}accept edits${reset}"
  ;;
plan)
  printf "${sep}${yellow}plan${reset}"
  ;;
esac

# Context used
if [ -n "$used_pct" ]; then
  ctx_pct=$(printf '%.1f' "$used_pct")
  if [ -n "$total_tokens" ]; then
    ctx_k=$(echo "$total_tokens" | awk '{printf "%.1fk", $1 / 1000}')
    printf "${sep}${yellow}ctx: ${ctx_k} ${ctx_pct}%%${reset}"
  else
    printf "${sep}${yellow}ctx: ${ctx_pct}%%${reset}"
  fi
fi

# 5-hour rate limit
if [ -n "$five_h" ]; then
  printf "$sep"
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
  printf "$sep"
  if [ -n "$seven_d_resets" ]; then
    now=$(date +%s)
    diff_secs=$(( seven_d_resets - now ))
    [ "$diff_secs" -lt 0 ] && diff_secs=0
    days_left=$(( (diff_secs + 86399) / 86400 ))
    seven_d_left=$(echo "$seven_d" | awk '{printf "%.0f", 100 - $1}')
    printf "${blue}${days_left}d ${seven_d_left}%% left${reset} ${overlay}> %s${reset}" "$(date -r "$seven_d_resets" +%d/%m 2>/dev/null || date -d "@$seven_d_resets" +%d/%m 2>/dev/null)"
  else
    seven_d_left=$(echo "$seven_d" | awk '{printf "%.0f", 100 - $1}')
    printf "${blue}7d ${seven_d_left}%% left${reset}"
  fi
fi

printf "\n"
exit 0
