# Notes

## Project loops

- Canonical delivery trigger: the user explicitly invokes `delivery-issue` with an approved GitHub issue reference.
- A Delivery Run is one resumable execution that takes exactly one approved GitHub issue from its selected Delivery Contract to either a non-mutating `tracking` result or integration into `main` with green post-merge CI.
- A Delivery Contract selects a complete historical Agent Brief, a complete standalone issue body, or a delivery ticket composed with its native parent specification and relationships. The Agent Brief remains the highest-precedence historical Contract Source, not a mandatory comment for every issue.
- A Delivery Result is the final user-facing summary of the issue, pull request, integrated commit, post-merge CI, and cleanup state.

## Workflow conventions

- `delivery-issue` is explicitly triggered with `N`, `#N`, or a same-repository GitHub issue URL. Once admission succeeds, it has no routine human checkpoint and runs autonomously until complete or genuinely blocked.
- Delivery order is changes → automated validation → applicable manual verification → independent review → PR. A new Delivery Run never opens its PR before local review.
- Mandatory implementation review uses a dedicated review skill when available and otherwise the active agent's native independent-review capability. Review covers Standards and Spec, runs locally before PR creation, repeats after fixes, and reruns after meaningful PR/CI follow-up commits.
- A Delivery Result classifies whether its merged change requires a dots release. Release publication remains a separate, potentially batched workflow; pure internal tooling/tests/agent-only docs do not require release.
