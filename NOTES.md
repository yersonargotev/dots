# Notes

## Project loops

- Canonical trigger: a `dots` development loop starts when the user has an intention of change for the `dots` project, before an issue necessarily exists.
- The loop may span multiple sessions and can produce ADRs, GitHub issues, PRs, reviews, merges, and dots releases.

## Workflow conventions

- Use `loop-me` when designing or revising reusable workflow specs in `workflows/*.md`.
- Use `grilling` when the plan or decision needs one-question-at-a-time stress testing and codebase exploration cannot answer the question.
- `grill-with-docs` is skipped only for mechanical or already-aligned work: small fixes, documentation adjustments with no new decision, explicit review follow-ups, or issues already marked `ready-for-agent`.
- Changes touching domain language, CLI UX, security, installation behavior, release workflow, or architecture go through `grill-with-docs`.

- After `grill-with-docs`, use a human-in-the-loop Alignment Brief before triage. The user would like more automation eventually, but wants to prove the loop first.
- After triage, use a human-in-the-loop Triage Brief before implementation. It includes issue links, state/labels, research, exact scope, risks, and acceptance criteria.
- Implementation default is changes → local review → PR. PR-before-review is reserved for cases where early GitHub/CI feedback or external visibility materially helps.
- Mandatory implementation review uses the available `$review` skill only. It reviews Standards and Spec axes. Run it locally before PR readiness, repeat after fixes, and rerun after meaningful PR/CI follow-up commits.
- A merged PR triggers a dots release when it affects CLI behavior, install/deps behavior, visible output, distributed configuration, operational user documentation, or a user-usable bugfix. Pure internal tooling/tests/agent-only docs do not trigger release.
- The loop closes with a Release/Closure Brief containing merged PRs, merge commit, tag/release links when applicable, closed issues, validation, user-facing change summary, and follow-ups.
