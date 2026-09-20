#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
config="$repo_root/configs/starship/starship.toml"
zimrc="$repo_root/configs/zsh/zimrc"

for command_name in git starship; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'required command not found: %s\n' "$command_name" >&2
    exit 1
  fi
done

sandbox_root="$(mktemp -d)"
sandbox_home="$sandbox_root/home"
remote="$sandbox_root/remote.git"
repo="$sandbox_root/repo"
peer="$sandbox_root/peer"
mkdir -p "$sandbox_home"

git init -q --bare "$remote"
git clone -q "$remote" "$repo"
git -C "$repo" switch -q -c main
git -C "$repo" config user.name "Prompt Test"
git -C "$repo" config user.email "prompt@example.invalid"
printf 'base\n' >"$repo/tracked"
git -C "$repo" add tracked
git -C "$repo" commit -qm base
git -C "$repo" push -qu origin main
git --git-dir="$remote" symbolic-ref HEAD refs/heads/main

render_prompt() {
  local status="$1"
  local width="$2"
  local path="${3:-$repo}"
  local duration="${4:-0}"
  TERM=xterm-256color \
    STARSHIP_SHELL=sh \
    STARSHIP_CONFIG="$config" \
    HOME="$sandbox_home" \
    starship prompt --path "$path" --status "$status" --terminal-width "$width" --cmd-duration "$duration" |
    sed $'s/\033\\[[0-9;]*m//g'
}

clean_success="$(render_prompt 0 100)"
clean_failure="$(render_prompt 1 52)"

# Compare literal, ANSI-stripped prompt fragments. Dynamic path, fill, and time
# content is deliberately outside these stable state assertions.
[[ "$clean_success" == *$' main'* ]]
[[ "$clean_success" == *$'\n❯ ' ]]
[[ "$clean_success" != *'+'* ]]
[[ "$clean_success" != *'!'* ]]
[[ "$clean_success" != *'?'* ]]
[[ "$clean_failure" == *$' main'* ]]
[[ "$clean_failure" == *$'\n❯ ' ]]
[[ "$(printf '%s' "$clean_success" | wc -l | tr -d ' ')" == "1" ]]
[[ "$(printf '%s' "$clean_failure" | wc -l | tr -d ' ')" == "1" ]]

runtime_repo="$sandbox_root/runtime"
mkdir -p "$runtime_repo"
printf '{}\n' >"$runtime_repo/package.json"
runtime_prompt="$(render_prompt 0 100 "$runtime_repo")"
[[ "$runtime_prompt" == *'node v'* ]]
[[ "$clean_success" != *'node v'* ]]

fast_prompt="$(render_prompt 0 100 "$repo" 499)"
slow_prompt="$(render_prompt 0 100 "$repo" 500)"
[[ "$fast_prompt" != *'took '* ]]
[[ "$slow_prompt" == *'took 500ms'* ]]

# Make the local branch diverge, then add one file in each requested worktree
# state so all compact Git indicators render together.
git clone -q "$remote" "$peer"
git -C "$peer" config user.name "Prompt Test"
git -C "$peer" config user.email "prompt@example.invalid"
printf 'remote\n' >"$peer/remote"
git -C "$peer" add remote
git -C "$peer" commit -qm remote
git -C "$peer" push -q origin main

printf 'ahead\n' >"$repo/ahead"
git -C "$repo" add ahead
git -C "$repo" commit -qm ahead
git -C "$repo" fetch -q origin
printf 'staged\n' >"$repo/staged"
git -C "$repo" add staged
printf 'changed\n' >>"$repo/tracked"
printf 'new\n' >"$repo/untracked"

dirty="$(render_prompt 0 100)"
[[ "$dirty" == *' main'* ]]
[[ "$dirty" == *'+1!1?1⇕⇡1⇣1'* ]]
[[ "$dirty" == *$'\n❯ ' ]]

for redundant_module in asciiship git-info duration-info; do
  if grep -Eq "^[[:space:]]*zmodule[[:space:]]+${redundant_module}([[:space:]]|$)" "$zimrc"; then
    printf 'redundant Zim prompt module remains: %s\n' "$redundant_module" >&2
    exit 1
  fi
done

for retained_module in completion zsh-users/zsh-syntax-highlighting zsh-users/zsh-history-substring-search zsh-users/zsh-autosuggestions; do
  grep -Eq "^[[:space:]]*zmodule[[:space:]]+${retained_module}([[:space:]]|$)" "$zimrc"
done

printf 'Starship clean, dirty, success, failure, and Zim ownership checks passed.\n'
