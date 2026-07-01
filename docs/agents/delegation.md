# Task-fit delegation across supported agents

This note separates the portable dots delegation policy from native, surface-specific
agent artifacts. The portable policy is installed by the `agents` profile into the
supported global instruction files. Native artifacts are only safe when the agent's
official format and dots ownership/cleanup boundary are both clear.

## Portable policy

Delegate non-trivial work only when there is an independent slice that can return a
compact summary without transferring requirements, decisions, external state,
integration, or final verification away from the main agent.

| Slice | Model/tier choice |
| --- | --- |
| Codebase exploration, impact scans, test/log triage | Fast/cost-effective capable model, read-only tools when the surface supports them. |
| Separable implementation over disjoint files/modules | Implementation-capable worker with explicit ownership and a changed-file list. |
| Review, architecture, security, or other high-judgment work | Strongest appropriate available model, or the model required by the selected skill. |

Record each Delegation Decision in workflow briefs and PR summaries: delegated slice
or skip reason, selected agent surface, selected model/tier, accepted/rejected
findings or changes, and main-agent verification.

Skip delegation when the task is tiny/mechanical, lacks an independent slice, touches
real user configuration, would mutate GitHub/PR/release/external state, or would
create overlapping write scopes.

## Surface inventory

| Surface | Native delegation support | dots-managed artifact for this slice |
| --- | --- | --- |
| Codex | Official subagent workflows, built-in `explorer`/`worker`, and custom TOML agents under `~/.codex/agents/` or `.codex/agents/`; custom agents can set `model`, reasoning effort, sandbox mode, MCP servers, and instructions. | Portable policy in `~/.codex/AGENTS.md`; Codex Spark guidance remains opt-in/removable through `codex-spark-delegation` and `without-codex-spark-delegation`. No new Codex custom-agent files are added to the `agents` profile. |
| Claude Code | Official Markdown subagents with YAML frontmatter under `.claude/agents/`, `~/.claude/agents/`, managed settings, `--agents`, or plugins; frontmatter supports `model`, tools, permission modes, and other controls. | Portable policy in `~/.claude/CLAUDE.md`. Native Claude subagent files are deferred until dots has a dedicated ownership and cleanup story for global `~/.claude/agents/`. |
| OpenCode | Official primary agents and subagents; custom agents can be JSON entries or Markdown files under `~/.config/opencode/agents/` or `.opencode/agents/`, including `mode`, `model`, `permission`, and prompt. | Portable policy in `~/.config/opencode/AGENTS.md`. Native OpenCode agent files are deferred because the existing OpenCode config boundary deliberately avoids touching gentle-ai-owned generated config. |
| VS Code Copilot | Official customization docs include instructions, skills, custom agents, language-model selection, and subagents, but user/profile storage and ownership need a separate dots design before writing files. | Portable policy in VS Code Copilot prompt instruction files for macOS and Linux. No Copilot custom-agent artifact is added until the exact user-data target and cleanup semantics are designed. |
| Antigravity | Official docs expose Antigravity rules/workflows and a subagents area, but this repository has not yet confirmed a stable file format and ownership boundary equivalent to Claude/OpenCode agent files. | Portable policy in `~/.gemini/GEMINI.md` only. No Antigravity native delegation file is written until the official artifact format is confirmed. |

## Cleanup and ownership

The portable policy lives inside the existing `<!-- dots:rules -->` block and is
updated in place by `dots install --profile agents`. The Codex Spark block remains a
separate `<!-- dots:codex-spark-delegation -->` opt-in overlay that maps the portable
policy to Codex explorer/worker usage instead of duplicating the global rules. It is
the only delegation artifact with explicit cleanup through
`--tag without-codex-spark-delegation`.
Future native per-agent artifacts must define their target path, install tag, ownership
mode, and removal semantics before they are added to `dots.yaml`.

## Sources checked

- OpenAI Codex subagents and custom agents: <https://developers.openai.com/codex/subagents>
- OpenAI Codex subagent concepts: <https://developers.openai.com/codex/concepts/subagents>
- Claude Code subagents: <https://code.claude.com/docs/en/sub-agents>
- OpenCode agents: <https://opencode.ai/docs/agents/>
- VS Code Copilot customization docs: <https://code.visualstudio.com/docs/copilot/copilot-customization>
- Google Antigravity subagents/rules docs: <https://antigravity.google/docs/subagents> and <https://antigravity.google/docs/rules-workflows>
