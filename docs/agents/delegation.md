# Task-fit delegation across supported agents

This note separates the distributable delegation skill from native, surface-specific
agent artifacts. The `agents` profile installs the `delegation` skill as the
detailed procedure source, while supported global instruction files keep only
critical triggers and safety pointers. Native artifacts are only safe when the
agent's official format and dots ownership/cleanup boundary are both clear.

## Portable policy

The detailed procedure lives in the `delegation` skill installed from
`yersonargotev/dots/skills/delegation` by the `agents` profile or for Codex alone
by the `codex-delegation` profile. Delegate
non-trivial work only when there is an independent slice that can return a compact
summary without transferring requirements, decisions, external state, integration,
or final verification away from the main agent.

| Slice | Model/tier choice |
| --- | --- |
| Codebase exploration, impact scans, test/log triage | Fast/cost-effective capable model, read-only tools when the surface supports them. |
| Separable implementation over disjoint files/modules | Implementation-capable worker with explicit ownership and a changed-file list. |
| Review, architecture, security, or other high-judgment work | Strongest appropriate available model, or the model required by the selected skill. |

Record each Delegation Decision using the reporting checklist in the
`delegation` skill.

Delegation Preflight is required for non-trivial work. For Codex, that means
checking the model-neutral `dots:delegation` overlay plus both native custom
agents at `~/.codex/agents/dots-explorer.toml` and
`~/.codex/agents/dots-worker.toml`. The Codex overlay is portable global
guidance: installing the profile is standing user authorization for safe
bounded Codex delegation across repositories, not only for the dots repository.
Codex still only spawns subagents when the active prompt, workflow, or skill gives
a direct subagent/parallel-agent/delegation ask, so reusable workflows should say
that explicitly instead of relying on a dots-only workflow name. The skip reasons, including `tool-level permission required`, and reporting format live in the `delegation` skill.

## Surface inventory

| Surface | Native delegation support | dots-managed artifact for this slice |
| --- | --- | --- |
| Codex | Official subagent workflows, built-in `explorer`/`worker`, and custom TOML agents under `~/.codex/agents/` or `.codex/agents/`; custom agents can set `model`, reasoning effort, sandbox mode, MCP servers, and instructions. | `codex-delegation` installs the `delegation` skill only for Codex, a generic overlay in `~/.codex/AGENTS.md`, and dots-owned native agents at `~/.codex/agents/dots-explorer.toml` and `~/.codex/agents/dots-worker.toml`. Cleanup uses `without-codex-delegation`; the old Spark-named tags remain compatibility aliases. |
| Claude Code | Official Markdown subagents with YAML frontmatter under `.claude/agents/`, `~/.claude/agents/`, managed settings, `--agents`, or plugins; frontmatter supports `model`, tools, permission modes, and other controls. | Portable policy in `~/.claude/CLAUDE.md`. Native Claude subagent files are deferred until dots has a dedicated ownership and cleanup story for global `~/.claude/agents/`. |
| OpenCode | Official primary agents and subagents; custom agents can be JSON entries or Markdown files under `~/.config/opencode/agents/` or `.opencode/agents/`, including `mode`, `model`, `permission`, and prompt. | Portable policy in `~/.config/opencode/AGENTS.md`. Native OpenCode agent files are deferred because the existing OpenCode config boundary deliberately avoids touching gentle-ai-owned generated config. |
| VS Code Copilot / Copilot CLI | Official customization docs include VS Code Copilot instructions, skills, custom agents, language-model selection, and subagents; Copilot CLI documents a global custom-instructions file. Native VS Code/Copilot custom-agent artifacts still need a separate dots design before writing files. | Portable policy in VS Code Copilot prompt instruction files for macOS and Linux plus Copilot CLI `~/.copilot/copilot-instructions.md`. dots treats completion of the `vscode-copilot` gentle-ai provisioner as the ownership boundary for these Copilot portable-policy files only; no Copilot custom-agent artifact is added until the exact user-data target and cleanup semantics are designed. |
| Antigravity | Official docs expose Antigravity rules/workflows and a subagents area, but this repository has not yet confirmed a stable file format and ownership boundary equivalent to Claude/OpenCode agent files. | Portable policy in `~/.gemini/GEMINI.md` only. No Antigravity native delegation file is written until the official artifact format is confirmed. |

## Cleanup and ownership

The detailed portable policy lives in the installed `delegation` skill. The
existing `<!-- dots:rules -->` block is updated in place by
`dots install --profile agents` and stays compact: critical safety rules plus a
skill pointer. Codex delegation remains a separate
`<!-- dots:delegation -->` opt-in overlay that maps the skill's policy
to Codex explorer/worker usage instead of duplicating the full procedure. It must
remain repository-neutral because the same global Codex instructions are loaded in
non-dots projects. The same opt-in tag also writes two dots-owned native Codex
custom-agent files:
`~/.codex/agents/dots-explorer.toml` and `~/.codex/agents/dots-worker.toml`.
`--tag without-codex-delegation` removes the overlay plus those two dots-owned
files, while preserving `dots:rules`, Engram, CodeGraph, Codex config, user-owned
custom agents, and unrelated agent baseline content.
The legacy `codex-spark-delegation` and `without-codex-spark-delegation` tags
remain accepted aliases and migrate the old Spark-specific marker to the generic
overlay.
Future native per-agent artifacts must define their target path, install tag, ownership
mode, and removal semantics before they are added to `dots.yaml`.

## Sources checked

- OpenAI Codex subagents and custom agents: <https://developers.openai.com/codex/subagents>
- OpenAI Codex subagent concepts: <https://developers.openai.com/codex/concepts/subagents>
- Claude Code subagents: <https://code.claude.com/docs/en/sub-agents>
- OpenCode agents: <https://opencode.ai/docs/agents/>
- VS Code Copilot customization docs: <https://code.visualstudio.com/docs/copilot/copilot-customization>
- Google Antigravity subagents/rules docs: <https://antigravity.google/docs/subagents> and <https://antigravity.google/docs/rules-workflows>
