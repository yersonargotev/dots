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
[`agent-brief.md`](agent-brief.md). Otherwise the issue remains `needs-triage`.
