# Notes

## Project loops

- Canonical trigger: a `dots` development loop starts when the user has an intention of change for the `dots` project, before an issue necessarily exists.
- The loop may span multiple sessions and can produce ADRs, GitHub issues, PRs, reviews, merges, and dots releases.

## Workflow conventions

- Use `loop-me` when designing or revising reusable workflow specs in `workflows/*.md`.
- Use `grilling` when the plan or decision needs one-question-at-a-time stress testing and codebase exploration cannot answer the question.
- `grill-with-docs` is skipped only for mechanical or already-aligned work: small fixes, documentation adjustments with no new decision, explicit review follow-ups, or issues already marked `ready-for-agent`.
- Changes touching domain language, CLI UX, security, installation behavior, release workflow, or architecture go through `grill-with-docs`.
- Codex subagent delegation is evaluated in every phase of the dots development loop and is the default for non-trivial work. The main thread owns requirements, decisions, integration, external project state, and final verification.
- Good delegation slices: independent exploration, impact scans, test/log triage, or disjoint implementation ownership. Good opt-outs: tiny mechanical tasks, single coherent edits with no independent research value, real user configuration, GitHub/external-state mutation, or overlapping write scopes.
- Pick the model/tier by delegated job: GPT-5.6 Sol with low reasoning is preferred for bounded exploration, triage, and separable implementation; review, architecture, security, or other judgment-heavy slices use the strongest appropriate available model, or the model the selected skill requires.
- Skill-owned subagents are allowed when the selected skill requires them, including `$review` Standards/Spec subagents; keep review subagents on the selected skill's strongest appropriate model, and keep final finding triage, fixes, and verification in the main agent.
- Delegation subagents do not mutate GitHub or other external project state in the dots development loop. The main agent owns opening/editing issues, labels, comments, PRs, merges, releases, commits, and final integration.
- Codex delegation is opt-in through the narrow `codex-delegation` profile, not bundled into the broader `agents` baseline. The profile installs only the portable skill for Codex and the generic `dots:delegation` overlay in AGENTS; GPT-5.6 Sol with low reasoning is the current model choice rather than the capability identity.
- Codex delegation has explicit cleanup through `without-codex-delegation`, removing only `<!-- dots:delegation -->...<!-- /dots:delegation -->` while preserving `dots:rules`, Engram, CodeGraph, Codex config, and user-owned agents.
- The legacy `codex-spark-delegation` and `without-codex-spark-delegation` tags remain compatibility aliases. If install and cleanup tags are selected together, cleanup wins because explicit exclusion expresses the desired final state.

- After `grill-with-docs`, use a human-in-the-loop Alignment Brief before triage. The user would like more automation eventually, but wants to prove the loop first.
- After triage, use a human-in-the-loop Triage Brief before implementation. It includes issue links, state/labels, research, exact scope, risks, and acceptance criteria.
- Delegation uses an explicit Delegation Decision in existing briefs: what was delegated or why it was skipped, which model/tier was chosen, what came back, what the main agent accepted or rejected, and what the main agent verified directly.
- Implementation default is changes → local review → PR. PR-before-review is reserved for cases where early GitHub/CI feedback or external visibility materially helps.
- Mandatory implementation review uses the available `$review` skill. `$review` may launch its Standards and Spec subagents; use its Standards and Spec agents rather than an ad-hoc delegation review, and keep their model choice aligned with the selected skill. Run it locally before PR readiness, repeat after fixes, and rerun after meaningful PR/CI follow-up commits.
- A merged PR triggers a dots release when it affects CLI behavior, install/deps behavior, visible output, distributed configuration, operational user documentation, or a user-usable bugfix. Pure internal tooling/tests/agent-only docs do not trigger release.
- The loop closes with a Release/Closure Brief containing merged PRs, merge commit, tag/release links when applicable, closed issues, validation, user-facing change summary, and follow-ups.
