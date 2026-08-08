# Notes

## Project loops

- Canonical delivery trigger: the user explicitly invokes `delivery-issue` with an approved GitHub issue reference.
- A Delivery Run is one resumable execution that takes exactly one approved GitHub issue from its authoritative Agent Brief through integration into `main` and green post-merge CI.
- An Agent Brief is the single authoritative implementation contract stored as one GitHub issue comment headed `## Agent Brief`; it is an input contract, not a human checkpoint brief.
- A Delivery Result is the final user-facing summary of the issue, pull request, integrated commit, post-merge CI, and cleanup state.

## Workflow conventions

- `delivery-issue` is explicitly triggered with `N`, `#N`, or a same-repository GitHub issue URL. Once admission succeeds, it has no routine human checkpoint and runs autonomously until complete or genuinely blocked.
- Codex subagent delegation is evaluated during non-trivial Delivery Runs. The main thread owns requirements, decisions, integration, external delivery state, and final verification.
- Good delegation slices: independent exploration, impact scans, test/log triage, or disjoint implementation ownership. Good opt-outs: tiny mechanical tasks, single coherent edits with no independent research value, real user configuration, GitHub/external-state mutation, or overlapping write scopes.
- Pick the model/tier by delegated job: GPT-5.6 Sol with low reasoning is preferred for bounded exploration, triage, and separable implementation; review, architecture, security, or other judgment-heavy slices use the strongest appropriate available model, or the model the selected skill requires.
- Skill-owned subagents are allowed when the selected skill requires them, including `$review` Standards/Spec subagents; keep review subagents on the selected skill's strongest appropriate model, and keep final finding triage, fixes, and verification in the main agent.
- Delegation subagents do not mutate GitHub or other external project state in a Delivery Run. The main agent owns issue labels/comments, PRs, merges, commits, and final integration.
- Codex delegation is opt-in through the narrow `codex-delegation` profile, not bundled into the broader `agents` baseline. The profile installs only the portable skill for Codex, the generic `dots:delegation` overlay, and the dots-owned native explorer/worker agents; GPT-5.6 Sol with low reasoning is the current model choice rather than the capability identity.
- Codex delegation has explicit cleanup through `without-codex-delegation`, removing only `<!-- dots:delegation -->...<!-- /dots:delegation -->` and the two dots-owned native agents while preserving `dots:rules`, CodeGraph, Codex config, and user-owned agents.
- The legacy `codex-spark-delegation` and `without-codex-spark-delegation` tags remain compatibility aliases. If install and cleanup tags are selected together, cleanup wins because explicit exclusion expresses the desired final state.

- Delivery uses an explicit Delegation Decision in its final result: what was delegated or why it was skipped, which model/tier was chosen, what came back, what the main agent accepted or rejected, and what the main agent verified directly.
- Delivery order is changes → automated validation → applicable manual verification → independent review → PR. A new Delivery Run never opens its PR before local review.
- Mandatory implementation review uses a dedicated review skill when available and otherwise the active agent's native independent-review capability. Review covers Standards and Spec, runs locally before PR creation, repeats after fixes, and reruns after meaningful PR/CI follow-up commits.
- A Delivery Result classifies whether its merged change requires a dots release. Release publication remains a separate, potentially batched workflow; pure internal tooling/tests/agent-only docs do not require release.
