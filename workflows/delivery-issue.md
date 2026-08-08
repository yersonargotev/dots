# delivery issue

Status: Active

## Purpose

Deliver one approved `dots` issue through implementation, evidence, independent
review, squash merge, and green CI on the integrated `main` commit without
requiring routine human checkpoints.

This workflow replaces the repository-specific `dots-pr-creation` and
`dots-pr-fast-path` workflows. Issue creation prepares its input; independent
review is composed within it. There is no fast path that bypasses the issue
contract.

## Implementation surface

Keep this file as the single normative workflow specification. Implement
`.agents/skills/delivery-issue/SKILL.md` as a thin adapter that validates exactly
one issue argument, reads this specification completely, and executes it. Its
frontmatter uses `disable-model-invocation: true` where supported and an argument
hint such as `<issue-number-or-url>`. Add `agents/openai.yaml` with the skill's
display metadata and explicit default invocation.

Remove the complete `.agents/skills/dots-pr-creation` and
`.agents/skills/dots-pr-fast-path` trees. Update `dots-issue-creation` to hand
approved work to `delivery-issue`, not a fast path. Refresh and validate any
tracked skill registry/cache artifacts, including the ignored-but-tracked
`.atl` files, without force-adding unrelated runtime state.

Migrate source/tests that name the removed `dots-development-loop.md`, including
`internal/agentinstructions/trigger_rules_test.go`, to the portable delegation
invariants in this specification and `docs/agents/delegation.md`. Do not retain
obsolete workflow names merely to satisfy tests.

Keep the repository-owned Agent Brief contract in
`docs/agents/agent-brief.md`. `dots-issue-creation` is the sole readiness
producer: it creates or updates a complete Agent Brief before adding
`ready-for-agent`. If it cannot produce the complete contract, it leaves the
issue in `needs-triage`. Triage-label documentation defines `ready-for-agent` as
both the label and exactly one valid Agent Brief, not merely a sufficiently
detailed issue body.

## Trigger

Start a Delivery Run only when the user explicitly invokes `delivery-issue` or
unambiguously asks to use it, with exactly one of:

- an issue number such as `42`;
- a hash-prefixed issue number such as `#42`; or
- a GitHub issue URL for the active `dots` repository.

Resolve short identifiers through `gh repo view`. Accept repository redirects
caused by a rename, but reject cross-repository URLs before making changes.
Reject missing or multiple issue references. Mere mention, diagnosis, review,
or discussion of an issue never triggers delivery. Set
`disable-model-invocation: true` in runtimes that support it.

## Vocabulary

- **Delivery Run**: one resumable execution for exactly one approved issue.
- **Agent Brief**: the authoritative implementation contract stored as exactly
  one issue comment headed `## Agent Brief`. It is workflow input, not a human
  checkpoint brief.
- **Delivery Result**: the final user-facing summary of the issue, pull request,
  integrated commit, post-merge CI, and cleanup state.
- **Actionable finding**: a confirmed problem involving the Agent Brief, a bug
  or regression, security, documented standards, missing coverage for changed
  behavior, or unjustified complexity.
- **Observation**: an optional suggestion, personal preference, or out-of-scope
  improvement. It is recorded but does not block delivery.

## Authorization and checkpoints

The explicit trigger plus successful admission authorizes the complete in-scope
workflow: isolated branch/worktree changes, safe validation, commits, push, PR
creation and updates, finding resolution, squash merge, and cleanup.

There is no routine human checkpoint. Report progress without asking for
confirmation. Stop only when the workflow encounters a material product or
architecture decision, missing external authority, or an operational blocker it
cannot resolve safely.

## Capability preflight

Before changing code, verify GitHub authentication and repository identity,
issue read access, safe branch/worktree creation, push and PR capability, the
required local toolchain, and an independent review capability. Record visible
merge permission without assuming that an unavailable branch-protection API
means protection is absent.

Read the current `AGENTS.md`, `.github/workflows/ci.yml`,
`.github/PULL_REQUEST_TEMPLATE.md`, issue/triage documentation, and any relevant
`CONTEXT.md` or ADR before work. Current repository safety and contribution
rules may add gates. If they materially contradict this workflow, report
`blocked` with the conflicting sources and leave the input issue's readiness
unchanged rather than choosing a rule silently.

If the agent cannot read the contract, create isolated work, push, or publish a
PR, stop before implementation. If only final merge authority is missing,
continue through a fully validated, reviewed, green PR, then present one late
checkpoint with the PR link and exact merge action required. Resume in the same
session after the user merges, or through the same issue invocation later, to
verify `main` CI and clean up.

## Delegation

The main agent owns admission, requirements decisions, GitHub mutations,
commits, integration, and final verification.

For non-trivial work, delegate bounded exploration or implementation when the
work has independent slices and non-overlapping file ownership. The main agent
inspects, integrates, and verifies all delegated results. For a small change or
one coherent module, work directly and record a concise skip reason in the
Delivery Result.

For non-trivial work, load the repository `delegation` skill when available and
run its Delegation Preflight before implementation. Follow
`docs/agents/delegation.md` for installed runtime artifacts and tool-level
permission conflicts. If a stricter agent runtime still requires explicit
permission despite this workflow's authorization, record
`tool-level permission required` and continue directly rather than silently
claiming delegation occurred.

Subagents never change labels, comment on issues, create or update PRs, merge,
or otherwise mutate external project state. Select their model or tier for the
job according to repository guidance rather than encoding one provider-specific
model in this workflow. Independent review remains mandatory whether or not
implementation work was delegated.

## Admission

Before creating a branch or changing code:

1. Verify that the issue is open, has exactly one category label (`bug` or
   `enhancement`), and has `ready-for-agent` as its only triage-state label.
2. Find exactly one comment headed `## Agent Brief`. Permit only the repository's
   required AI-triage disclaimer and blank lines before that first heading.
   Revisions edit the same comment in place; multiple matching comments make the
   contract ambiguous.
3. Verify non-empty `Category`, `Summary`, `Current behavior`, `Desired
   behavior`, `Key interfaces`, `Acceptance criteria`, and `Out of scope`.
   Explicit `None` or `Not applicable` is allowed where meaningful.
4. Semantically verify that the brief is internally consistent, contains no
   open questions, covers relevant failures and edge cases, and has concrete,
   independently verifiable acceptance criteria.
5. Map every acceptance criterion to automated evidence, manual evidence, or
   both. Verify that external dependencies and blocking issues are resolved.
6. Record the brief comment ID, `updatedAt`, and SHA-256 digest of the exact raw
   UTF-8 comment body returned by GitHub.

The brief's `Category` must match the issue category label. Normalize conflicting
triage-state labels by removing them, applying `needs-triage`, and reporting the
conflict; never guess which conflicting state was intended.

If the contract is missing, incomplete, contradictory, or ambiguous, post one
comment listing concrete gaps, replace `ready-for-agent` with `needs-triage`,
and stop before creating a branch or changing code.

Revalidate the issue state and recorded Agent Brief before modifying code,
before opening the PR, and immediately before merge. If authorization
disappears, preserve work and stop. If the brief changed, restart admission
against the new contract.

At each revalidation, inspect issue comments added after the recorded brief
snapshot. Comments do not change scope; only an in-place Agent Brief revision
does. Continue past compatible information, ignore requests for additional
scope, and return to triage when new evidence makes the brief incorrect, unsafe,
or materially incomplete.

When a contract failure returns the issue to triage, prefix its issue comment
with the AI-triage disclaimer required by the repository Agent Brief contract.

## Resume and isolation

Treat the issue identifier as the Delivery Run identity. An open PR that closes
the issue takes precedence over branch-name heuristics during discovery.

1. If the issue was already merged, verify its PR, squash commit, and the
   commit's presence in `main`, then return the existing Delivery Result. Include
   retained post-merge CI evidence when available. If retention removed it,
   report `historical CI evidence unavailable`; never reopen or create a new
   push merely to recreate historical evidence.
2. If exactly one open PR closes the issue, resume its branch at the first gate
   without current evidence.
3. Otherwise, resume one unambiguous delivery branch when it exists.
4. If no prior work exists, fetch and create a branch whose stable suffix is
   `issue-N-<slug>` from the current `origin/main`. Use the environment-required
   agent prefix (`codex/` in Codex); otherwise use `delivery/`.
5. Stop and request direction if multiple PRs or branches are plausible.

Never stash, overwrite, or combine unrelated changes. If the current workspace
is occupied, create an isolated git worktree for the Delivery Run.

When the matching PR belongs to another contributor, inspect it and run the
delivery gates without mutating its branch. It may be squash-merged if it passes
unchanged. Never push, rebase, force-push, or delete a contributor-owned branch.
If fixes are required, present one exceptional authority checkpoint containing
the findings, branch ownership, and available options. Do not create a competing
PR or close the contributor's PR automatically.

The workflow has no private state file. Derive resumable state from Git, the
issue, the PR, and GitHub Actions. Store the Agent Brief comment ID and digest in
the PR body. Before a PR exists, rerun admission and every local gate after a
restart. With a PR, trust a check or review only when it is tied to the exact
current head and base; repeat any gate whose evidence cannot be proved for that
snapshot.

## Implementation and local gates

Repeat this loop until it produces a reviewable candidate:

1. Implement the smallest complete change authorized by the Agent Brief.
2. Run focused checks while iterating.
3. Run the complete CI-equivalent suite:
   - `gofmt -l .`
   - `go vet ./...`
   - `go build ./...`
   - `go test ./...`
4. Fix every failure and restart the loop.

Any later code or artifact change invalidates earlier automated, manual, and
review evidence and restarts the complete gates.

The implementing agent may decide local, reversible matters within scope. If
implementation reveals a contradiction, public-contract change, material scope
expansion, or hard-to-reverse decision not authorized by the brief, post the
evidence and decision needed, replace `ready-for-agent` with `needs-triage`,
preserve safe work for resumption, and stop without opening a partial PR.

Delivery orchestrates rather than replaces narrower engineering skills. Load
and follow any applicable repository, language, framework, diagnosis, testing,
security, or review skill required by the issue and changed code.

## Manual verification

For CLI-observable changes, build a temporary real binary (for example,
`go build -o <tmp>/dots ./cmd/dots`) and invoke that same binary for every
applicable acceptance scenario, relevant failure behavior, output, and exit
code. Do not substitute `go run` for this manual gate. Any dotfiles behavior uses
explicit temporary home, source, and state roots; never read or modify the
operator's real configuration.

Documentation-, skill-, CI-, or metadata-only changes may mark CLI verification
`not applicable` with a reason and direct evidence for the changed artifact.

Record commands plus expected and observed results for the PR. Any finding
returns to implementation and restarts all local gates.

After automated and manual gates pass, stage only in-scope files, create a
Conventional Commit checkpoint, and require a clean working tree. Fetch
`origin/main`; if it advanced, rebase the committed branch, then repeat
automated and manual gates before review. Commit any conflict fix separately.
With an open PR, push rebased history only with `--force-with-lease`.

When work touches the skill registry, inspect the tracked
`.atl/skill-registry.md` and `.atl/.skill-registry.cache.json` explicitly even
though `.atl/` is ignored. Validate their Markdown/JSON content and force-add
only those tracked files when they are part of the authorized change. Never
force-add the whole ignored directory.

## Independent review

Review must be independent from the implementing context and cover both:

- **Spec**: conformance to the Agent Brief, acceptance criteria, and scope.
- **Standards**: conformance to `AGENTS.md`, dotfiles safety, tests, relevant
  architecture, and repository conventions.

Use a dedicated review skill when available. Otherwise use the active agent's
native independent-review capability. Self-review does not satisfy this gate;
absence of an independent review capability is an operational blocker.

Review the stable `origin/main...HEAD` commit range. Fix every actionable
finding, create a new Conventional Commit, then restart automated validation,
applicable manual verification, and independent review of the new `HEAD`.
Require a clean working tree at every review, before opening the PR, and before
merge. Observations do not block and stay outside the issue scope.

Do not create follow-up issues automatically for observations. Include each
observation in the Delivery Result with concise evidence and whether it merits
later triage. Create a separate issue only when the Agent Brief requires it or
the user explicitly requests it after delivery.

## Pull request and remote loop

Immediately before creating a PR, repeat discovery for any open PR that closes
the issue. Resume one unambiguous match instead of creating a duplicate; stop on
ambiguous concurrent work.

Open a non-draft PR against `main` only after all local gates pass. The PR must
follow the repository template, include `Closes #N`, use exactly one matching
`type:*` label, and record acceptance coverage, automated validation, manual
evidence or its not-applicable rationale, and independent review.

For a PR owned by the Delivery Run, append exactly one block delimited by
`<!-- delivery-issue:evidence:start -->` and
`<!-- delivery-issue:evidence:end -->`, with heading `## Delivery Evidence`.
Update that block in place after every fix or push. It records:

- Agent Brief comment URL/ID and SHA-256 digest;
- current head commit reviewed;
- acceptance-criterion coverage;
- automated validation commands/results;
- manual verification commands/results or not-applicable reason;
- independent Spec and Standards review results; and
- delegation slices/results or skip reason plus main-agent verification.

Do not edit a contributor-owned PR body to add this block. Recompute its local
gates and use native checks/reviews as retained evidence.

A normal Delivery Run closes exactly its one input issue. Related issues may be
linked with `Refs #N` but not closed. If an existing PR closes multiple issues,
stop at an authority checkpoint rather than integrating it automatically.
Post-merge correction and rollback PRs remain linked to the same input issue.

Use a Conventional Commit PR title because it becomes the squash commit subject.
Do not add `Co-Authored-By` trailers or other AI attribution to commits or the
PR.

Wait for CI and required repository rules. Resolve actionable remote review
findings. Every remote fix restarts all local gates and independent review before
push. Optional approvals do not block unless repository rules require them.

Immediately before merge, revalidate the issue and Agent Brief and require that
GitHub reports the PR mergeable with checks for the current head/base. Resolve
conflicts inside the Delivery Run and restart all gates; return material new
decisions to triage.

## Merge, post-merge CI, and cleanup

1. Squash merge without immediately deleting the delivery branch.
2. Verify that `Closes #N` closed the issue. If it did not, close the issue
   explicitly with a link to the merge. Preserve its category and
   `ready-for-agent` labels as workflow history; do not add a delivered label.
3. Wait for CI on the exact integrated commit on `main`.
4. If it passes, delete the remote delivery branch and safely remove the local
   delivery branch/worktree.
5. Fast-forward a local `main` only when that does not touch unrelated work. If
   it is occupied or dirty, fetch and verify `origin/main`, report local sync as
   pending, and do not block delivery.

If post-merge CI fails, do not report success or clean the delivery worktree.
Reopen the issue if needed and preserve `ready-for-agent`. Diagnose attribution
before acting:

- If a safe fix is covered by the same Agent Brief, create a correction branch
  linked to the issue and repeat the complete workflow.
- If the squash commit caused the failure but a safe fix requires a new product
  decision or scope, create a revert branch and PR linked to the issue. Validate
  and independently review the revert, squash-merge it, and require green
  post-revert CI on `main`. Then reopen the issue if needed, replace
  `ready-for-agent` with `needs-triage`, and comment with the evidence and
  decision required.
- Do not revert for infrastructure failures, flaky tests, or failures caused by
  another concurrent change; treat those as operational blockers with clear
  attribution evidence.

Never push a rollback directly to `main`. If an attributable broken merge cannot
be safely reverted, raise an urgent operational blocker and preserve all state.

## Blocked outcomes

Operational blockers do not invalidate issue readiness. Preserve
`ready-for-agent`, branch/worktree, and PR state. Retry reasonably transient
failures, then post one concise issue comment containing evidence, the last
completed gate, and the external action needed. Do not introduce another triage
label for ephemeral run state.

Do not retry deterministic code, test, or review failures without a change;
diagnose and fix them. Retry transient network or API operations at most three
total attempts with incremental waits. Rerun a CI job at most twice only when
logs indicate infrastructure failure or flakiness. If the same operational
blocker persists, report it as blocked. Successful recovery resets that
blocker's counter; loops that make implementation progress have no global cap.

Contract blockers return the issue to `needs-triage` as described in Admission
or Implementation.

## Evidence and output

Use the PR body for durable pre-merge evidence and GitHub's native reviews,
checks, merge record, and `main` CI run for the remaining audit trail. Do not add
routine progress or success comments to the issue.

For `complete` and `already-complete`, return a Delivery Result containing:

- issue and PR links;
- squash commit on `main`;
- automated validation result;
- manual verification result or not-applicable reason;
- independent review result;
- delegation result: slices used or skip reason, returned work accepted or
  rejected, and main-agent verification;
- post-merge `main` CI result;
- remote/local cleanup result, including any intentionally pending local sync;
  and
- release classification: `required` or `not required`, with a reason.

For `needs-triage` or `blocked`, include the issue link, concrete reason and
evidence, last completed gate, preserved branch/PR/worktree artifacts when any,
and the exact next action. Never emit placeholders for artifacts that do not
exist. `awaiting-authority` uses the Authority Brief defined below.

Every run reports exactly one outcome:

- `complete`: the issue is closed, its change is in `main`, post-merge CI is
  green, and cleanup is safe and complete or explicitly pending for an occupied
  local `main`.
- `already-complete`: an earlier merge was verified without new mutation;
  expired historical CI evidence is reported but does not reopen delivery.
- `needs-triage`: the contract is invalid or a new material decision is needed.
  A successful rollback uses this outcome and explicitly states that `main` was
  restored.
- `blocked`: an operational impediment persisted after its retry limit.
- `awaiting-authority`: all possible work is prepared, but a human must merge or
  decide how to handle contributor-owned work.

For `awaiting-authority`, present an Authority Brief containing what is ready,
why the agent lacks authority, exactly one requested decision or action, and
direct links to the relevant issue, PR, or asset.

`delivery-issue` does not publish a release. A `required` classification is an
event input for the separate, potentially batched `dots-release` workflow.
Classify release as required for CLI behavior, install/dependency behavior,
visible output, distributed configuration, operational user documentation, or
a user-usable bugfix. Classify pure internal tooling, tests, or agent-only
documentation as not required.

## Definition of done

A Delivery Run is complete only when the approved change is squash-merged into
`main`, CI is green for the exact integrated commit, the issue is closed, and
safe branch/worktree cleanup is complete or an occupied local `main` is reported
as intentionally pending synchronization.

## Implementation acceptance criteria

- `workflows/delivery-issue.md` is the sole workflow source of truth and no
  obsolete `dots-development-loop.md` remains.
- `delivery-issue` exists as a thin, explicit-only skill adapter with OpenAI
  interface metadata and exactly-one-issue argument handling.
- `dots-pr-creation` and `dots-pr-fast-path` are fully removed; active skills,
  docs, tests, and metadata contain no stale operational references to them.
- `dots-issue-creation` is the sole readiness producer and creates or updates
  exactly one valid Agent Brief before applying `ready-for-agent`.
- Agent Brief parsing supports only the required AI-triage disclaimer and blank
  lines before `## Agent Brief`, detects duplicates, and validates every
  required semantic field.
- The delivery adapter points to this spec instead of duplicating its runtime
  rules.
- Tests that referenced the removed workflow validate the new portable
  delegation contract and documentation.
- Tracked `.atl` registry/cache files are refreshed and valid; unrelated ignored
  runtime state is not staged.
- Markdown links, skill frontmatter, OpenAI YAML, JSON, and affected symlinks or
  registries pass their focused validation.
- `gofmt -l .`, `go vet ./...`, `go build ./...`, and `go test ./...` all pass.
- Because this implementation changes workflow/skills rather than CLI behavior,
  manual CLI verification is explicitly not applicable and direct artifact
  validation is recorded instead.
