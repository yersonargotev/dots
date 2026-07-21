# AI / agent tooling hooks.
#
# Portable knobs only — never credentials. Tokens (e.g. ENGRAM_CLOUD_TOKEN)
# belong in ~/.zshrc.local, which is not managed by dots.

# Claude Code: disable flicker in fullscreen rendering.
export CLAUDE_CODE_NO_FLICKER=1

# Engram: opt into tool-search behavior.
export ENABLE_TOOL_SEARCH=true

# Claude Code harness backed by GPT-5.6 Sol through the local CLIProxyAPI.
if command -v claude >/dev/null 2>&1 && command -v cliproxyapi >/dev/null 2>&1; then
  alias claudex='ANTHROPIC_BASE_URL=http://127.0.0.1:8317 ANTHROPIC_AUTH_TOKEN=sk-dummy CLAUDE_CODE_SUBAGENT_MODEL=gpt-5.6-sol CLAUDE_CODE_ALWAYS_ENABLE_EFFORT=1 CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY=3 ENABLE_TOOL_SEARCH=false claude --model gpt-5.6-sol'
fi

# GitHub Copilot CLI: start interactive sessions in Autopilot mode by default.
if [[ -o interactive ]] && command -v copilot >/dev/null 2>&1; then
  copilot() {
    command copilot --mode autopilot "$@"
  }
fi
