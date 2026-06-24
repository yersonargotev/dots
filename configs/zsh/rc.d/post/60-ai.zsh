# AI / agent tooling hooks.
#
# Portable knobs only — never credentials. Tokens (e.g. ENGRAM_CLOUD_TOKEN)
# belong in ~/.zshrc.local, which is not managed by dots.

# Claude Code: disable flicker in fullscreen rendering.
export CLAUDE_CODE_NO_FLICKER=1

# Engram: opt into tool-search behavior.
export ENABLE_TOOL_SEARCH=true

# GitHub Copilot CLI: start interactive sessions in Autopilot mode by default.
if [[ -o interactive ]] && command -v copilot >/dev/null 2>&1; then
  copilot() {
    command copilot --mode autopilot "$@"
  }
fi
