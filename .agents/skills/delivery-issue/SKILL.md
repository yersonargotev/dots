---
name: delivery-issue
description: Delivers one approved dots GitHub issue through implementation, validation, independent review, squash merge, and green post-merge CI. Use only when the user explicitly invokes delivery-issue with one issue number or same-repository issue URL.
disable-model-invocation: true
argument-hint: "<issue-number-or-url>"
---

# Delivery Issue

Execute one complete Delivery Run for the `dots` repository.

## Input

Require exactly one explicit issue reference:

- `N`
- `#N`
- a GitHub issue URL for the active repository

Reject missing, multiple, or cross-repository references before mutation.

## Execution

1. Resolve the repository root with `git rev-parse --show-toplevel` and verify
   that `gh repo view` identifies the active `dots` repository.
2. Read `<repo-root>/workflows/delivery-issue.md` completely. It is the sole
   normative workflow specification.
3. Execute that workflow from capability preflight through exactly one reported
   outcome. Do not duplicate, weaken, or skip its admission, safety, review,
   merge, post-merge CI, rollback, or cleanup rules.
4. Use current repository instructions and narrower applicable skills when the
   workflow requires them.

If the workflow spec is missing or contradicts current repository rules, stop
with the workflow's `blocked` outcome. Mere mention or inspection of an issue is
not authorization to run this skill.
