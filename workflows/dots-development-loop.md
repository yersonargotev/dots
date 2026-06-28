# dots development loop

## Purpose

Turn a `dots` change intention into aligned design artifacts, triaged work, implemented changes, reviewed PRs, merged code, and a dots release when appropriate.

## Trigger

The workflow starts when the user has an intention of change for the `dots` project, before an issue necessarily exists.

## Current shape

1. Align before implementation using the narrowest matching skill.
   - Use `loop-me` when designing or revising reusable workflow specs in `workflows/*.md`.
   - Use `grilling` when the plan or decision needs one-question-at-a-time stress testing and codebase exploration cannot answer the question.
   - Use `grill-with-docs` when the change needs shared understanding, domain sharpening, ADRs, issue creation, architecture notes, or workflow documentation.
   - Skip `grill-with-docs` only for mechanical or already-aligned work: small fixes, documentation adjustments that introduce no new decision, explicit review follow-ups, or existing issues already marked `ready-for-agent`.
   - Use `grill-with-docs` for any change touching domain language, CLI UX, security, installation behavior, release workflow, or architecture.
   - Evaluate Spark subagent delegation in every phase, but do not delegate by default. Delegate only when a bounded slice can return compact findings or changes without blurring ownership.
   - Spark does not mutate GitHub or other external project state. It produces findings, briefs, or local changes for the main agent to inspect.
   - Codex Spark delegation is an opt-in installable capability, not part of the default `agents` baseline. It should be selected with a dedicated tag such as `codex-spark-delegation` so the user can stop installing it independently from the rest of the agents profile.
   - Codex Spark delegation also needs a dedicated cleanup path. Removing the install tag from future runs is not enough because an existing `<!-- dots:codex-spark-delegation -->` block remains in `~/.codex/AGENTS.md` until explicitly removed.
2. Present an Alignment Brief after alignment and wait for the user to approve moving into triage.
   - The brief includes links to ADRs and issues created or changed, decisions made, doubts closed, and the single decision: approve moving to triage.
   - This checkpoint is intentionally human-in-the-loop for now, even though the desired long-term direction is more automation after the loop proves itself.
   - If Spark was used during alignment, include Delegation notes: what was delegated, what came back, what the main agent accepted or rejected, and what the main agent verified directly.
3. Triage the resulting issue or issues, including research and further alignment when necessary.
   - The main agent owns all GitHub actions: opening or editing issues, applying labels, posting comments, opening PRs, merging, and releasing.
4. Present a Triage Brief and wait for the user to approve moving into implementation.
   - The brief includes issue links, current state and labels, relevant research, exact implementation scope, risks, acceptance criteria, and the single decision: approve moving to implementation.
   - If Spark was used during triage, include Delegation notes with the same accept/reject/verification shape.
5. Implement the issue or issues using the repository skills and workflow documentation.
   - Default path: changes → local review → PR.
   - Use PR-before-review only when early GitHub/CI feedback is materially useful or the work needs external visibility before local review is complete.
   - Delegate implementation to Spark only for disjoint files or modules with explicit ownership, required changed-file reporting, and no permission to revert unrelated edits.
   - Spark workers may prepare local patches, but the main agent inspects and integrates the result before any commit, PR, or external update.
   - If a change is needed to the Codex Spark delegation guidance itself, prefer making it separately installable/removable through the `codex-spark-delegation` tag rather than coupling it to the `agents` profile.
   - Preferred cleanup shape: a declarative tag, `without-codex-spark-delegation`, removes only the `<!-- dots:codex-spark-delegation -->...<!-- /dots:codex-spark-delegation -->` block. It must not remove `dots:rules`, Engram, CodeGraph, Codex config, or any other agent baseline.
   - If both `codex-spark-delegation` and `without-codex-spark-delegation` are selected, the `without-*` tag wins because explicit exclusion expresses the desired final state.
6. Review the implementation with `$review` and loop back into implementation for every confirmed finding.
   - `$review` is the mandatory review skill for this loop.
   - Run it locally against the fixed point before opening or marking the PR ready, using the two available axes: Standards and Spec.
   - If review finds issues, fix them and run `$review` again until the confirmed findings are closed.
   - After the PR exists, run `$review` again when PR feedback, CI fixes, or meaningful follow-up commits change the diff.
   - Do not use Spark for review in this loop. Keep review centralized through `$review` and the main agent's verification.
7. Merge the PR and release `dots` when the change should ship.
   - Release after merging changes that affect CLI behavior, install/deps behavior, visible output, distributed configuration, operational user documentation, or a user-usable bugfix.
   - Do not release for purely internal tooling, tests, or agent-only documentation that does not change the user experience.
8. Present a Release/Closure Brief to close the workflow.
   - The brief includes merged PR links, merge commit, tag and release links when applicable, closed issues, final validation evidence, what changed for the user, and any remaining follow-ups.
   - If Spark was used at any point in the workflow, include final Delegation notes covering the delegated slices, accepted findings or changes, rejected findings or changes, and final verification performed by the main agent.
