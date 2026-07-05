---
name: delegation
description: >
  Delegate safe agent work slices. Trigger: non-trivial work needs exploration,
  test/log triage, separable implementation, review subagents, or a workflow asks
  for Delegation Preflight.
license: Apache-2.0
metadata:
  author: yersonargotev
  version: "1.0"
---

# Delegation

Use this skill when a task may benefit from a bounded subagent without handing off
requirements, decisions, external project state, integration, or final verification.

## Preflight

1. Decide whether the task is non-trivial.
2. Confirm the active surface has the needed delegation capability. For Codex
   Spark in dots, require the `dots:codex-spark-delegation` overlay plus
   `~/.codex/agents/dots-explorer.toml` and
   `~/.codex/agents/dots-worker.toml`.
3. Pick a safe slice or one skip reason: `tiny/mechanical`,
   `no independent slice`, `real user configuration`,
   `external state mutation`, `overlapping write scopes`, or
   `tool-level permission required`.
4. Keep the main agent responsible for requirements, decisions, GitHub/PR/release
   state, integration, and final verification.

## Slice routing

| Slice | Delegate to |
| --- | --- |
| Codebase exploration, impact scans, test/log triage | Fast read-only explorer. In Codex dots, use `dots-explorer` on Spark. |
| Separable implementation in disjoint files/modules | Worker with explicit file ownership and a changed-file list. In Codex dots, use `dots-worker` on Spark. |
| Review, architecture, security, or judgment-heavy work | The strongest appropriate model, or the model required by the selected review skill. |

Do not delegate work that mutates GitHub issues, PRs, labels, releases, package
managers, or real user configuration. Subagents may provide evidence or local
changes only.

## Worker contract

For write-heavy delegation, tell the worker:

- its exact file/module ownership;
- other agents may be editing, so it must not revert unrelated edits;
- to use sandboxed `--home`, `--source-root`, `--state-root`, or temp config paths
  for dotfiles behavior;
- to return changed files, tests run, risks, and a concise handoff.

## Integration and report

Inspect delegated evidence or changes before accepting them. Run the relevant
verification yourself. Report:

- delegated slice and agent surface;
- model/tier choice;
- accepted findings or changes;
- rejected findings or changes;
- main-agent verification;
- skip reason when no subagent was used.
