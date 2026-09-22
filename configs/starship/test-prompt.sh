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

assert_prompt_contains() {
  local phase="$1"
  local prompt="$2"
  local expected="$3"
  if [[ "$prompt" != *"$expected"* ]]; then
    printf 'Starship assertion failed [%s]: expected substring %q\n' "$phase" "$expected" >&2
    printf 'ANSI-stripped prompt: %q\n' "$prompt" >&2
    exit 1
  fi
}

assert_prompt_excludes() {
  local phase="$1"
  local prompt="$2"
  local unexpected="$3"
  if [[ "$prompt" == *"$unexpected"* ]]; then
    printf 'Starship assertion failed [%s]: unexpected substring %q\n' "$phase" "$unexpected" >&2
    printf 'ANSI-stripped prompt: %q\n' "$prompt" >&2
    exit 1
  fi
}

assert_prompt_starts_with() {
  local phase="$1"
  local prompt="$2"
  local expected="$3"
  if [[ "$prompt" != "$expected"* ]]; then
    printf 'Starship assertion failed [%s]: expected prefix %q\n' "$phase" "$expected" >&2
    printf 'ANSI-stripped prompt: %q\n' "$prompt" >&2
    exit 1
  fi
}

assert_prompt_newlines() {
  local phase="$1"
  local prompt="$2"
  local expected="$3"
  local actual
  actual="$(printf '%s' "$prompt" | wc -l | tr -d ' ')"
  if [[ "$actual" != "$expected" ]]; then
    printf 'Starship assertion failed [%s]: expected %s newline(s), got %s\n' "$phase" "$expected" "$actual" >&2
    printf 'ANSI-stripped prompt: %q\n' "$prompt" >&2
    exit 1
  fi
}

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
assert_prompt_starts_with "clean success separation" "$clean_success" $'\n'
assert_prompt_starts_with "clean failure separation" "$clean_failure" $'\n'
assert_prompt_contains "clean success branch" "$clean_success" " main"
assert_prompt_contains "clean success marker" "$clean_success" $'\n❯ '
assert_prompt_excludes "clean success staged state" "$clean_success" "+"
assert_prompt_excludes "clean success modified state" "$clean_success" "!"
assert_prompt_excludes "clean success untracked state" "$clean_success" "?"
assert_prompt_contains "clean failure branch" "$clean_failure" " main"
assert_prompt_contains "clean failure marker" "$clean_failure" $'\n❯ '
assert_prompt_newlines "clean success shape" "$clean_success" 2
assert_prompt_newlines "clean failure shape" "$clean_failure" 2

runtime_repo="$sandbox_root/runtime"
mkdir -p "$runtime_repo"
printf '{}\n' >"$runtime_repo/package.json"
runtime_prompt="$(render_prompt 0 100 "$runtime_repo")"
assert_prompt_contains "runtime detection" "$runtime_prompt" "node v"
assert_prompt_excludes "runtime omission" "$clean_success" "node v"

fast_prompt="$(render_prompt 0 100 "$repo" 499)"
slow_prompt="$(render_prompt 0 100 "$repo" 500)"
assert_prompt_excludes "fast command duration" "$fast_prompt" "took "
assert_prompt_contains "slow command duration" "$slow_prompt" "took 500ms"

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
assert_prompt_contains "dirty branch" "$dirty" " main"
assert_prompt_contains "dirty Git state" "$dirty" "+1 !1 ?1 ⇕⇡1⇣1"
assert_prompt_contains "dirty marker" "$dirty" $'\n❯ '

for redundant_module in asciiship git-info duration-info; do
  if grep -Eq "^[[:space:]]*zmodule[[:space:]]+${redundant_module}([[:space:]]|$)" "$zimrc"; then
    printf 'redundant Zim prompt module remains: %s\n' "$redundant_module" >&2
    exit 1
  fi
done

for retained_module in completion zsh-users/zsh-syntax-highlighting zsh-users/zsh-history-substring-search zsh-users/zsh-autosuggestions; do
  if ! grep -Eq "^[[:space:]]*zmodule[[:space:]]+${retained_module}([[:space:]]|$)" "$zimrc"; then
    printf 'required Zim module missing: %s\n' "$retained_module" >&2
    exit 1
  fi
done

printf 'Starship rendering, spacing, and Zim ownership checks passed.\n'
