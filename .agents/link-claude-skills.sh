#!/usr/bin/env bash
#
# link-claude-skills.sh — expose THIS repo's skills to Claude Code, repo-local only.
#
# Project scope: Claude discovers skills under <repo>/.claude/skills/. We mirror
# each authored skill in .agents/skills/ with a relative symlink so the repo is
# the single source of truth and the whole thing is committable + portable.
#
#   <repo>/.claude/skills/<name> -> ../../.agents/skills/<name>
#
# No global state is touched: nothing under ~/.agents or ~/.claude is modified.
# Re-run after adding or removing a skill.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$REPO_ROOT/.agents/skills"
DST="$REPO_ROOT/.claude/skills"

mkdir -p "$DST"

# Prune dangling/stale links from a previous run.
for link in "$DST"/*; do
  [ -e "$link" ] || [ -L "$link" ] || continue
  name="$(basename "$link")"
  if [ -L "$link" ] && [ ! -e "$SRC/$name" ]; then
    rm -f "$link"
    echo "✗ pruned (skill removed): $name"
  fi
done

linked=0
for dir in "$SRC"/*/; do
  name="$(basename "$dir")"
  ln -sfn "../../.agents/skills/$name" "$DST/$name"
  linked=$((linked + 1))
done

echo "Done. $linked skills linked into .claude/skills/ (repo-local)."
