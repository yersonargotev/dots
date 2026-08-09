# Triage Labels

This repository uses the default triage label vocabulary.

| Role | Label |
| --- | --- |
| Maintainer needs to evaluate | `needs-triage` |
| Waiting on reporter | `needs-info` |
| Complete Delivery Contract, ready for execution when native blockers allow | `ready-for-agent` |
| Needs human implementation | `ready-for-human` |
| Will not be actioned | `wontfix` |

## Agent-ready publishing

Any approved workflow may apply `ready-for-agent` after verifying a complete
Delivery Contract. The Contract Source may be a historical Agent Brief, a
standalone issue body, or a ticket composed with its native parent
specification and relationships. See
[`delivery-contract.md`](delivery-contract.md).
In short, `ready-for-agent` means that the specification is complete; native
blockers determine whether a Delivery Unit can execute now.

Use these four publication states for sliced work:

| Work state | Triage/readiness labels | Delivery status |
| --- | --- | --- |
| Unreviewed or incompletely specified | `needs-triage` | Not ready |
| Fully specified Delivery Unit with an open native blocker | `ready-for-agent` | Outside the Execution Frontier until the blocker closes |
| Fully specified, unblocked Delivery Unit | `ready-for-agent` | In the Execution Frontier for `delivery-issue` |
| Complete Tracking Issue with native sub-issues | `ready-for-agent` | Read-only `tracking` result; children carry implementation |

Do not use `needs-triage` as a blocked-work label. Open blockers do not cause
readiness-label churn. When a blocker closes, revalidate the Delivery Contract
and native relationships; the newly unblocked Delivery Unit enters the
Execution Frontier without a readiness-label transition. Invoking delivery on
a Tracking Issue reports its executable child frontier without mutation.
