# Triage Labels

This repository uses the default triage label vocabulary.

| Role | Label |
| --- | --- |
| Maintainer needs to evaluate | `needs-triage` |
| Waiting on reporter | `needs-info` |
| Fully specified with exactly one valid Agent Brief and ready for an agent | `ready-for-agent` |
| Needs human implementation | `ready-for-human` |
| Will not be actioned | `wontfix` |

## Agent-ready publishing

`dots-issue-creation` is the sole readiness producer. It applies the
`ready-for-agent` label only after creating or updating exactly one complete
Agent Brief that satisfies the repository-owned contract in
[`agent-brief.md`](agent-brief.md).

Use these four publication states for sliced work:

| Work state | Triage/readiness labels | Delivery status |
| --- | --- | --- |
| Unreviewed or incompletely specified | `needs-triage` | Not ready |
| Fully specified with one complete Agent Brief, but with an open native blocker | Neither `needs-triage` nor `ready-for-agent` | GitHub's native dependency state reports it as blocked |
| Fully specified, unblocked delivery frontier with one complete Agent Brief | `ready-for-agent` | Ready for `delivery-issue` |
| Sliced tracking parent | Neither `needs-triage` nor `ready-for-agent` | Tracking container, not a delivery unit |

Do not use `needs-triage` as a blocked-work label. When a blocker closes,
revalidate the dependent issue's native relationships and its single complete
Agent Brief, then move each newly unblocked delivery issue to
`ready-for-agent`. A PRD that becomes a sliced tracking parent must lose
`ready-for-agent`; only its current unblocked child frontier may receive it.
