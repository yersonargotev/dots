#!/usr/bin/env bash
# GitHub Copilot CLI statusLine — plain text + Catppuccin Mocha palette

input=$(cat)

field() {
  jq -r "$1 // empty" <<<"$input" 2>/dev/null
}

model=$(field '.model.displayName // .model.display_name // .model.name // .model // .session.model')
effort=$(field '.effortLevel // .effort_level // .model.effortLevel // .model.effort_level')
cwd=$(field '.cwd // .currentWorkingDirectory // .workspace.currentDirectory // .workspace.root')
branch=$(field '.git.branch // .branch // .workspace.git.branch')
ctx_pct=$(field '.contextWindow.usedPercentage // .context_window.used_percentage // .context.usedPercentage')
quota=$(field '.quota.remainingPercentage // .quota.remaining_percentage // .usage.remainingPercentage')
agent=$(field '.agent.name // .agent // .customAgent.name')
yolo=$(field '.yolo // .allowAllTools // .permissions.allowAll')
sandbox=$(field '.sandbox.enabled // .sandbox')

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
printed=0

print_part() {
  if [ "$printed" -eq 1 ]; then
    printf "$sep"
  fi
  printed=1
  printf "%b%s%b" "$1" "$2" "$reset"
}

if [ -n "$model" ]; then
  if [ -n "$effort" ]; then
    print_part "$subtext" "$model:$effort"
  else
    print_part "$subtext" "$model"
  fi
fi

if [ -n "$agent" ]; then
  print_part "$mauve" "$agent"
fi

if [ -n "$branch" ]; then
  print_part "$green" "branch: $branch"
elif [ -n "$cwd" ]; then
  print_part "$green" "$(basename "$cwd")"
fi

if [ -n "$ctx_pct" ]; then
  ctx_fmt=$(awk -v n="$ctx_pct" 'BEGIN { printf "%.1f%%", n }')
  print_part "$yellow" "ctx: $ctx_fmt"
fi

if [ -n "$quota" ]; then
  quota_fmt=$(awk -v n="$quota" 'BEGIN { printf "%.0f%% left", n }')
  print_part "$blue" "$quota_fmt"
fi

case "$sandbox" in
true|enabled) print_part "$green" "sandbox" ;;
false|disabled) print_part "$red" "no sandbox" ;;
esac

case "$yolo" in
true) print_part "$pink" "yolo" ;;
esac

if [ "$printed" -eq 0 ]; then
  print_part "$subtext" "copilot"
fi

printf "\n"
exit 0
