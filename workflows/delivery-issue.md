# delivery issue

Status: Active

## Purpose

Process one approved `dots` issue without routine human checkpoints. A Tracking
Issue returns its executable frontier without mutation; a Delivery Unit proceeds
through implementation, evidence, independent review, squash merge, and green CI
on the integrated `main` commit.

The repository's issue-creation workflow and external planning skills may all
publish valid input. Delivery selects and snapshots a producer-neutral Delivery
Contract; independent review is composed within this workflow.

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
- **Delivery Contract**: the complete, normalized, and snapshotted scope that
  authorizes the run.
- **Contract Source**: the selected authoritative content: a historical Agent
  Brief, a standalone issue body, or a delivery ticket composed with its native
  parent specification and relationships.
- **Delivery Unit**: one reviewable implementation issue with a complete
  Delivery Contract.
- **Tracking Issue**: a complete parent specification implemented by native
  sub-issues rather than by its own branch and PR.
- **Execution Frontier**: ready Delivery Units without open native blockers.
- **Delivery Result**: the final user-facing summary of the issue, pull request,
  integrated commit, post-merge CI, and cleanup state.
- **Actionable finding**: a confirmed problem involving the Delivery Contract,
  a bug or regression, security, documented standards, missing coverage for
  changed behavior, or unjustified complexity.
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

## Admission

Before creating a branch or changing code:

1. Verify that the issue is open, has exactly one category label (`bug` or
   `enhancement`), and has `ready-for-agent` as its only triage-state label.
2. Read the issue body, all comments, and native `parent`, `subIssues`,
   `blockedBy`, and `blocking` relationships. Do not infer relationship state
   from body prose when GitHub exposes it natively.
3. Select exactly one Contract Source in this precedence:
   - one complete historical Agent Brief headed `## Agent Brief`;
   - otherwise, a complete standalone issue body when the issue has no native
     parent; or
   - otherwise, a delivery ticket body composed with its native parent
     specification and native relationships.
4. Treat a present Agent Brief that is duplicated, malformed, incomplete, or
   contradictory as ambiguous instead of silently falling through. Permit only
   the repository AI-triage disclaimer and blank lines before its heading. A
   child with a native parent is a composed ticket, not a standalone source.
   Complete historical Agent Brief issues remain deliverable without migration.
5. Semantically verify that the selected content determines a summary,
   current-state gap, desired behavior, relevant failures and edge cases, key
   interfaces, concrete acceptance criteria, and out-of-scope boundaries.
   Explicit `None` or `Not applicable` is allowed where meaningful.
6. Map every acceptance criterion to automated evidence, manual evidence, or
   both. Open native blockers do not invalidate completeness or remove
   `ready-for-agent`; they keep a Delivery Unit outside the Execution Frontier.
7. Snapshot every selected source body's node or comment ID, URL when available,
   `updatedAt`, exact raw UTF-8 body, and SHA-256 digest. Also snapshot category,
   triage/readiness labels, and all native relationships with issue identifiers,
   states, and update timestamps.

An issue-level `type:*` label is not part of admission; that label belongs to the
eventual PR. Normalize conflicting triage-state labels by removing them, applying
`needs-triage`, and reporting the conflict; never guess which state was intended.

If the contract is missing, incomplete, contradictory, or ambiguous, post one
comment listing concrete gaps, replace `ready-for-agent` with `needs-triage`,
and stop before creating a branch or changing code.

If the issue has native sub-issues and those children carry its implementation
outcomes, classify it as a Tracking Issue after validating the contract. Return
a non-mutating `tracking` Delivery Result listing child Delivery Units currently
in the Execution Frontier. Do not create a branch, commit, or PR.

For a Delivery Unit, stop without label mutation when any native `blockedBy`
issue is open. Report the blocker as an operational `blocked` outcome and retain
`ready-for-agent`; resume when native state places the unit in the Execution
Frontier.

Revalidate the complete snapshot before modifying code, before opening the PR,
and immediately before merge. Compare every source identity, body, `updatedAt`,
and digest; category; triage/readiness label; and every native relationship's
membership, issue identity, state, and `updatedAt`. Any difference restarts
admission. If readiness or authorization disappears, preserve work and stop;
conflicting states return to triage under the admission rules. The sole
exception is a `blockedBy` issue whose only semantic change is blocker state and
its consequent timestamp; that only recomputes frontier membership.

At each revalidation, inspect issue comments added after the snapshot. Comments
do not change scope. Continue past compatible information, ignore requests for
additional scope, and return to triage when new evidence makes the Delivery
Contract incorrect, unsafe, or materially incomplete.

When a contract failure returns the issue to triage, prefix its issue comment
with the repository's AI-triage disclaimer.

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
issue, the PR, and GitHub Actions. Store the complete Contract Source snapshot in
the PR body. Before a PR exists, rerun admission and every local gate after a
restart. With a PR, trust a check or review only when it is tied to the exact
current head, base, and Delivery Contract snapshot; repeat any gate whose
evidence cannot be proved for that state.

## Mutation safety gate

Before implementation begins, classify whether the change spans
managed filesystem mutation, persisted metadata or receipts, recovery or
rollback, or authority or identity that may change concurrently. For an
applicable change, read the reference completely:
[mutation-safety-gate.md](mutation-safety-gate.md). Complete its safety case.
Documentation-, skill-, CI-, or metadata-only changes with no mutation boundary
may record `not applicable` with direct evidence.

The gate passes only when every required part of the safety case is complete and
its independent design challenge has zero actionable findings. Repair local,
reversible gaps inside the gate. A material product or architecture decision not
authorized by the Delivery Contract returns `needs-triage`; an unavailable
required capability returns `blocked` under the existing outcome rules.

A later change that changes the approved mutation model invalidates the prior
gate evidence; repeat the gate before further implementation. The design
challenge is an early adversarial review. Final independent review remains
required.

## Implementation and local gates

Repeat this loop until it produces a reviewable candidate:

1. Implement the smallest complete change authorized by the Delivery Contract.
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
expansion, or hard-to-reverse decision not authorized by the Delivery Contract,
post the evidence and decision needed, replace `ready-for-agent` with
`needs-triage`, preserve safe work for resumption, and stop without opening a
partial PR.

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

- **Spec**: conformance to the Delivery Contract, acceptance criteria, and scope.
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
later triage. Create a separate issue only when the Delivery Contract requires
it or the user explicitly requests it after delivery.

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

- selected Contract Source kind and every source body ID, URL when available,
  `updatedAt`, and SHA-256 digest;
- snapshotted category, readiness, and native relationships;
- current head commit reviewed;
- mutation-safety result or its `not applicable` evidence;
- acceptance-criterion coverage;
- automated validation commands/results;
- manual verification commands/results or not-applicable reason;
- independent Spec and Standards review results.

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

Immediately before merge, revalidate the issue and complete Delivery Contract
snapshot and require that GitHub reports the PR mergeable with checks for the
current head/base. Resolve conflicts inside the Delivery Run and restart all
gates; return material new decisions to triage.

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

- If a safe fix is covered by the same Delivery Contract, create a correction
  branch linked to the issue and repeat the complete workflow.
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

An open native blocker is expected Delivery Contract state, not an operational
failure. Return `blocked` with the blocking issue links and the last completed
admission gate, create no branch or PR, preserve `ready-for-agent`, and do not
retry or post a routine issue comment. Resume after native relationship state
places the Delivery Unit in the Execution Frontier.

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
- post-merge `main` CI result;
- remote/local cleanup result, including any intentionally pending local sync;
  and
- release classification: `required` or `not required`, with a reason.

For `needs-triage` or `blocked`, include the issue link, concrete reason and
evidence, last completed gate, preserved branch/PR/worktree artifacts when any,
and the exact next action. Never emit placeholders for artifacts that do not
exist. `awaiting-authority` uses the Authority Brief defined below.

For `tracking`, include the Tracking Issue link and Contract Source snapshot,
the executable child Delivery Units with links, and confirmation that no branch,
commit, or PR was created.

Every run reports exactly one outcome:

- `tracking`: the input is a Tracking Issue; list executable child Delivery
  Units and confirm that no branch or PR mutation occurred.
- `complete`: the issue is closed, its change is in `main`, post-merge CI is
  green, and cleanup is safe and complete or explicitly pending for an occupied
  local `main`.
- `already-complete`: an earlier merge was verified without new mutation;
  expired historical CI evidence is reported but does not reopen delivery.
- `needs-triage`: the contract is invalid or a new material decision is needed.
  A successful rollback uses this outcome and explicitly states that `main` was
  restored.
- `blocked`: the Delivery Unit has an open native blocker, or an operational
  impediment persisted after its retry limit.
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

## Workflow contract

- Contract Source precedence is historical Agent Brief, standalone issue body,
  then delivery ticket plus native parent specification and relationships.
- Admission rejects incomplete, ambiguous, or contradictory sources before
  branch creation while preserving historical Agent Brief compatibility.
- `ready-for-agent` records specification completeness; native blockers control
  Execution Frontier membership without readiness-label churn.
- A Tracking Issue returns `tracking` without creating a branch or PR.
- The complete source, category, readiness, and relationship snapshot is
  revalidated before code mutation, PR creation, and merge.
- Applicable mutation work completes its safety gate before implementation and
  repeats it whenever the approved mutation model changes.
- Pull requests retain exactly one `type:*` label and durable Delivery Evidence.
- The adapter remains explicit-only and delegates all runtime rules to this
  normative workflow.
