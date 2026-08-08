# Triage Labels

This repository uses the default triage label vocabulary.

| Role | Label |
| --- | --- |
| Maintainer needs to evaluate | `needs-triage` |
| Waiting on reporter | `needs-info` |
| Fully specified with exactly one valid Agent Brief and ready for an agent | `ready-for-agent` |
| Needs human implementation | `ready-for-human` |
| Will not be actioned | `wontfix` |

## PRD Publishing

PRD issues created by the `to-prd` workflow should receive the `ready-for-agent`
label only after `to-prd` creates or updates exactly one complete Agent Brief.
Otherwise they remain `needs-triage`.
