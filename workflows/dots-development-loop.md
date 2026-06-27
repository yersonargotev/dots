# dots development loop

## Purpose

Turn a `dots` change intention into aligned design artifacts, triaged work, implemented changes, reviewed PRs, merged code, and a dots release when appropriate.

## Trigger

The workflow starts when the user has an intention of change for the `dots` project, before an issue necessarily exists.

## Current shape

1. Align with `grill-with-docs` when the change needs shared understanding, domain sharpening, ADRs, or issue creation.
   - Skip `grill-with-docs` only for mechanical or already-aligned work: small fixes, documentation adjustments that introduce no new decision, explicit review follow-ups, or existing issues already marked `ready-for-agent`.
   - Use `grill-with-docs` for any change touching domain language, CLI UX, security, installation behavior, release workflow, or architecture.
2. Present an Alignment Brief after `grill-with-docs` and wait for the user to approve moving into triage.
   - The brief includes links to ADRs and issues created or changed, decisions made, doubts closed, and the single decision: approve moving to triage.
   - This checkpoint is intentionally human-in-the-loop for now, even though the desired long-term direction is more automation after the loop proves itself.
3. Triage the resulting issue or issues, including research and further alignment when necessary.
4. Present a Triage Brief and wait for the user to approve moving into implementation.
   - The brief includes issue links, current state and labels, relevant research, exact implementation scope, risks, acceptance criteria, and the single decision: approve moving to implementation.
5. Implement the issue or issues using the repository skills and workflow documentation.
   - Default path: changes → local review → PR.
   - Use PR-before-review only when early GitHub/CI feedback is materially useful or the work needs external visibility before local review is complete.
6. Review the implementation with `$review` and loop back into implementation for every confirmed finding.
   - `$review` is the mandatory review skill for this loop.
   - Run it locally against the fixed point before opening or marking the PR ready, using the two available axes: Standards and Spec.
   - If review finds issues, fix them and run `$review` again until the confirmed findings are closed.
   - After the PR exists, run `$review` again when PR feedback, CI fixes, or meaningful follow-up commits change the diff.
7. Merge the PR and release `dots` when the change should ship.
   - Release after merging changes that affect CLI behavior, install/deps behavior, visible output, distributed configuration, operational user documentation, or a user-usable bugfix.
   - Do not release for purely internal tooling, tests, or agent-only documentation that does not change the user experience.
8. Present a Release/Closure Brief to close the workflow.
   - The brief includes merged PR links, merge commit, tag and release links when applicable, closed issues, final validation evidence, what changed for the user, and any remaining follow-ups.

## Open questions

None.
