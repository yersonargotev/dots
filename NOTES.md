# Notes

## Project loops

- Canonical trigger: a `dots` development loop starts when the user has an intention of change for the `dots` project, before an issue necessarily exists.
- The loop may span multiple sessions and can produce ADRs, GitHub issues, PRs, reviews, merges, and dots releases.

## Workflow conventions

- Use `loop-me` when designing or revising reusable workflow specs in `workflows/*.md`.
- Use `grilling` when the plan or decision needs one-question-at-a-time stress testing and codebase exploration cannot answer the question.
- `grill-with-docs` is skipped only for mechanical or already-aligned work: small fixes, documentation adjustments with no new decision, explicit review follow-ups, or issues already marked `ready-for-agent`.
- Changes touching domain language, CLI UX, security, installation behavior, release workflow, or architecture go through `grill-with-docs`.
- Spark subagent delegation is evaluated in every phase of the dots development loop but is not mandatory. The main thread owns requirements, decisions, integration, and final verification.
- Good Spark slices: independent exploration, impact scans, test/log triage, or disjoint implementation ownership. Bad slices: tiny tasks, single coherent edits, review, real user configuration, or overlapping write scopes.
- Spark does not mutate GitHub or other external project state in the dots development loop. The main agent owns opening/editing issues, labels, comments, PRs, merges, releases, commits, and final integration.
- Codex Spark delegation should be opt-in and separately installable, not bundled into the default `agents` profile. Preferred tag name: `codex-spark-delegation`, selected with `dots install --profile agents --tag codex-spark-delegation`, so the user can stop installing it without removing the rest of the agent baseline.
- Codex Spark delegation needs explicit cleanup because omitting the install tag does not remove an already-written block. Preferred cleanup tag: `without-codex-spark-delegation`, removing only `<!-- dots:codex-spark-delegation -->...<!-- /dots:codex-spark-delegation -->` and preserving `dots:rules`, Engram, CodeGraph, Codex config, and the rest of the agent baseline.
- If both `codex-spark-delegation` and `without-codex-spark-delegation` are selected, `without-codex-spark-delegation` wins because explicit exclusion expresses the desired final state.

- After `grill-with-docs`, use a human-in-the-loop Alignment Brief before triage. The user would like more automation eventually, but wants to prove the loop first.
- After triage, use a human-in-the-loop Triage Brief before implementation. It includes issue links, state/labels, research, exact scope, risks, and acceptance criteria.
- Spark delegation does not add a new checkpoint. Existing briefs include Delegation notes when Spark was used: what was delegated, what came back, what the main agent accepted or rejected, and what the main agent verified directly.
- Implementation default is changes → local review → PR. PR-before-review is reserved for cases where early GitHub/CI feedback or external visibility materially helps.
- Mandatory implementation review uses the available `$review` skill only, not Spark. It reviews Standards and Spec axes. Run it locally before PR readiness, repeat after fixes, and rerun after meaningful PR/CI follow-up commits.
- A merged PR triggers a dots release when it affects CLI behavior, install/deps behavior, visible output, distributed configuration, operational user documentation, or a user-usable bugfix. Pure internal tooling/tests/agent-only docs do not trigger release.
- The loop closes with a Release/Closure Brief containing merged PRs, merge commit, tag/release links when applicable, closed issues, validation, user-facing change summary, and follow-ups.

## Pending implementation notes

- Implement `codex-spark-delegation` and `without-codex-spark-delegation` as separate selectable tags.
- Migrate Codex Spark delegation markers from `argote:subagent-delegation` to `dots:codex-spark-delegation`.
- Cleanup must remove both current `dots:codex-spark-delegation` markers and legacy `argote:subagent-delegation` markers while preserving `dots:rules`, Engram, CodeGraph, Codex config, and the rest of the agent baseline.
- Technical implementation direction: keep `dots:rules` on the automatic `agents` convergence path, move Codex Spark delegation into separate `ConvergeCodexSparkDelegation(home)` and `RemoveCodexSparkDelegation(home)` functions, and call them from install/update tag selection. `without-codex-spark-delegation` wins over `codex-spark-delegation`.
