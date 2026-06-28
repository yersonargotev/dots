# dots development loop

## Purpose

Turn a `dots` change intention into aligned design artifacts, triaged work,
implemented changes, reviewed PRs, merged code, and a dots release when the
change should ship.

## Trigger

Start this workflow when the user has a change intention for the `dots` project,
even when no GitHub issue exists yet.

## Scope

Use this workflow for changes to `dots` itself: CLI behavior, installation and
dependency behavior, generated agent instructions, documentation, release work,
and repository process changes.

Do not use this workflow for unrelated personal dotfile edits outside this
repository.

## Operating rules

- Keep the main agent responsible for requirements, decisions, GitHub state,
  commits, PRs, merges, releases, integration, and final verification.
- Keep diffs surgical. Every changed line must trace to the approved change
  intention or an explicitly accepted review finding.
- Use sandboxed `--home`, `--source-root`, `--state-root`, or temporary config
  paths for any dotfiles behavior validation. Never validate against the
  operator's real home configuration.
- Prefer the narrowest applicable skill for each phase; do not add ceremonies
  when the work is mechanical and already aligned.
- Push checkpoints right: do as much useful preparation as possible before
  asking the user for one decision.

## Phase 1: Align

Goal: turn the change intention into a shared, implementable direction.

1. Select the narrowest matching alignment path.
   - Use `loop-me` when designing or revising reusable workflow specs in
     `workflows/*.md`.
   - Use `grilling` when the plan or decision needs one-question-at-a-time
     stress testing and codebase exploration cannot answer the question.
   - Use `grill-with-docs` when the change needs shared understanding, domain
     sharpening, ADRs, issue creation, architecture notes, or workflow
     documentation.
   - Skip `grill-with-docs` only for mechanical or already-aligned work: small
     fixes, documentation adjustments that introduce no new decision, explicit
     review follow-ups, or existing issues already marked `ready-for-agent`.
   - Use `grill-with-docs` for any change touching domain language, CLI UX,
     security, installation behavior, release workflow, or architecture.
2. Inspect the smallest evidence needed: `CONTEXT.md`, relevant ADRs, issues,
   docs, source, tests, or command output. Use CodeGraph for source architecture,
   symbols, call flow, and impact analysis; use targeted shell reads for docs,
   manifests, configs, and scripts.
3. Evaluate Spark subagent delegation, but do not delegate by default.
   - Good Spark slices: independent codebase exploration, impact scans,
     test/log triage, or implementation over disjoint files/modules.
   - Bad Spark slices: tiny tasks, one coherent edit, review, real user
     configuration, GitHub/external-state mutation, or overlapping write scopes.
   - Spark may return findings, briefs, or local changes only. It must not open
     or edit issues, labels, comments, PRs, merges, releases, or any other
     external project state.
4. When the direction is aligned, present an Alignment Brief and wait for the
   user to approve moving to triage.

Alignment Brief format:

- Change intention and chosen alignment path.
- Decisions made and doubts closed.
- Links to ADRs, docs, issues, or notes created or changed.
- Risks or open constraints that implementation must respect.
- Delegation notes, if Spark was used: what was delegated, what came back, what
  the main agent accepted or rejected, and what the main agent verified directly.
- Decision requested: approve moving to triage.

## Phase 2: Triage

Goal: create or refine agent-ready GitHub issue scope.

1. Ensure there is at least one GitHub issue representing the work unless the
   user explicitly asks for read-only review or local-only exploration.
2. Apply the repository triage vocabulary from `docs/agents/triage-labels.md`.
3. Add enough research, acceptance criteria, and constraints for an implementer
   to proceed without asking follow-up questions.
4. Keep all GitHub mutation in the main agent. Spark may provide research or
   issue-scope suggestions, but the main agent writes external state.
5. Present a Triage Brief and wait for the user to approve moving to
   implementation.

Triage Brief format:

- Issue links, current labels, and readiness state.
- Exact implementation scope and explicit non-goals.
- Acceptance criteria and required validation.
- Risks, sandboxing needs, external-state hazards, and rollback notes.
- Delegation notes, if Spark was used, with the same accept/reject/verification
  shape as the Alignment Brief.
- Decision requested: approve moving to implementation.

## Phase 3: Implement

Goal: produce the smallest correct diff for the approved issue scope.

1. Work from the approved issue and brief. If implementation reveals a new
   product or architecture decision, stop and return to alignment.
2. Default path: local changes → local verification → local `$review` → PR.
3. Use PR-before-review only when early GitHub/CI feedback or external visibility
   is materially useful before local review is complete.
4. If Spark is used for implementation:
   - assign disjoint files or modules with explicit ownership;
   - require a changed-file list and concise handoff;
   - forbid reverting unrelated edits;
   - inspect, integrate, and verify the result in the main thread before any
     commit, PR, or external update.
5. For changes to Codex Spark delegation guidance itself:
   - keep the capability separately installable/removable through the
     `codex-spark-delegation` tag rather than coupling it to the default
     `agents` profile;
   - use `without-codex-spark-delegation` as the declarative cleanup tag;
   - remove only the
     `<!-- dots:codex-spark-delegation -->...<!-- /dots:codex-spark-delegation -->`
     block during cleanup;
   - preserve `dots:rules`, Engram, CodeGraph, Codex config, and the rest of the
     agent baseline;
   - if both `codex-spark-delegation` and `without-codex-spark-delegation` are
     selected, `without-*` wins because explicit exclusion expresses the desired
     final state.

## Phase 4: Review

Goal: close confirmed findings before PR readiness and after meaningful PR/CI
changes.

1. Use `$review` as the mandatory implementation review skill for this loop.
2. Run `$review` locally against the fixed point before opening or marking the PR
   ready, using the available axes: Standards and Spec.
3. Fix every confirmed finding and rerun `$review` until confirmed findings are
   closed.
4. After the PR exists, rerun `$review` when PR feedback, CI fixes, or meaningful
   follow-up commits change the diff.
5. Do not use Spark for review in this loop. Keep review centralized through
   `$review` and the main agent's verification.

## Phase 5: PR, merge, and release

Goal: land the reviewed change and publish it when users should receive it.

1. Open or update the PR using the repository PR template.
2. Include `Closes #<issue-number>`, exactly one `type:*` label, validation
   evidence, and the dotfiles safety checklist when config paths are involved.
3. Merge only after required checks and review expectations are satisfied.
4. Release `dots` after merging changes that affect CLI behavior,
   install/deps behavior, visible output, distributed configuration,
   operational user documentation, or a user-usable bugfix.
5. Do not release for purely internal tooling, tests, or agent-only
   documentation that does not change the user experience.

## Phase 6: Close

Goal: leave the user with the result, evidence, and any follow-up work.

Present a Release/Closure Brief:

- Merged PR links and merge commit.
- Tag and release links when applicable, or why no release was needed.
- Closed issues.
- Final validation evidence.
- User-facing change summary.
- Remaining follow-ups, if any.
- Final Delegation notes if Spark was used at any point: delegated slices,
  accepted findings or changes, rejected findings or changes, and final
  verification performed by the main agent.

## Required verification

Use focused checks while iterating, then match CI before declaring the workflow
complete for code changes:

```bash
gofmt -l .
go vet ./...
go build ./...
go test ./...
```

For docs-only workflow edits, a targeted Markdown/readability review and
`git diff --check` are sufficient unless the edit changes generated docs,
installer behavior, or command examples that must be executed.

## Done definition

This workflow run is done when:

- the change has passed the necessary alignment and triage checkpoints, or those
  checkpoints were explicitly unnecessary under the operating rules;
- the approved implementation scope is complete;
- confirmed `$review` findings are closed;
- required validation has passed;
- the PR is merged;
- a release exists when the release gate says one is required; and
- the user has received the Release/Closure Brief.
