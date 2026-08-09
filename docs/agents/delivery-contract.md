# Delivery Contracts

A Delivery Contract is the producer-neutral specification used by
`delivery-issue`. It preserves strict admission and revalidation without
requiring every planning workflow to synthesize a repository-specific comment.

## Contract Source precedence

Select exactly one source using this order:

1. A single complete historical Agent Brief comment headed `## Agent Brief`.
2. A complete standalone issue body when the issue has no native parent.
3. A delivery ticket body composed with its native parent specification and
   native hierarchy and dependency relationships.

A present Agent Brief that is duplicated, malformed, incomplete, or
contradictory makes the source ambiguous; do not silently fall through to the
issue body. A child issue with a native parent is evaluated as a composed
ticket, not as a standalone issue. Textual `Parent` or `Blocked by` sections
may aid readers but never replace relationships exposed by GitHub.

External planning and triage skills may publish complete standalone issues or
native ticket graphs directly. They do not need to call `dots-issue-creation`
or add an Agent Brief before delivery. Historical issues with one complete
Agent Brief remain valid without migration.

## Completeness

Every selected source must determine, without open questions:

- a concise summary and the current-state gap;
- the desired behavior and relevant failure or edge behavior;
- key interfaces or an explicit `Not applicable`;
- concrete, independently verifiable acceptance criteria; and
- explicit out-of-scope boundaries.

The issue must be open, have exactly one category label (`bug` or
`enhancement`), and have `ready-for-agent` as its only triage-state label.
`ready-for-agent` means that the specification is complete. It does not mean
that native blockers are closed, and no issue-level `type:*` label is required.
Pull requests still require exactly one matching `type:*` label.

Incomplete, contradictory, or ambiguous content returns to `needs-triage` with
concrete gaps before a branch is created. Open native blockers do not cause
readiness-label churn: the Delivery Unit remains specified but outside the
Execution Frontier.

## Tracking and execution

An issue with native sub-issues is a Tracking Issue when those children carry
its implementation outcomes. Delivery validates its specification and
relationships, then returns a non-mutating `tracking` result listing child
Delivery Units in the Execution Frontier. It creates no branch, commit, or pull
request for the Tracking Issue itself.

A Delivery Unit may execute only when it has a complete Delivery Contract and
no open issue in its native `blockedBy` relationships. Closing a blocker changes
frontier membership; it does not change specification completeness.

## Snapshot and revalidation

Admission records all inputs needed to prove that the contract did not change:

- each selected source body's GitHub node or comment identifier, URL when
  available, `updatedAt`, exact raw UTF-8 body, and SHA-256 digest;
- the category and triage/readiness labels; and
- native parent, sub-issue, blocked-by, and blocking relationships, including
  issue identifiers, state, and update timestamps.

Revalidate the snapshot immediately before code mutation, pull-request
creation, and merge. A changed source body or relationship restarts admission.
Compatible comments do not change scope. New contradictory evidence returns the
issue to triage; changed blocker state only recomputes the Execution Frontier.

## Source-specific guidance

The [Agent Brief contract](agent-brief.md) defines the historical comment shape.
The [issue tracker guidance](issue-tracker.md) defines native relationships and
the direct planning path. The [triage labels](triage-labels.md) define readiness
independently from blocker state. The delivery workflow remains the normative
runtime specification.
