---
name: dots-issue-creation
description: Create one or more GitHub Issues for the dots repository using its real templates, labels, and agent-ready workflow. Use when creating dots issues, linking work before PRs, converting plans into GitHub issues, or publishing tracer-bullet implementation slices for yersonargotev/dots.
---

# Dots Issue Creation

Create actionable GitHub Issues for `yersonargotev/dots` without drifting from this repo's workflow.

## Ground truth

Read these when context is stale or the workflow may have changed:

- `.github/ISSUE_TEMPLATE/*.yml`
- `docs/agents/issue-tracker.md`
- `docs/agents/triage-labels.md`
- `docs/agents/delivery-contract.md`
- `CONTEXT.md` when issue text touches domain concepts

## Repository conventions

- This is the direct repository-specific publication workflow for real
  templates, duplicate detection, category/readiness labels, native
  relationships, and publication verification. It is not the exclusive
  readiness producer or a mandatory post-processor for external planning and
  triage skills.
- External `to-spec` or `to-tickets` results may go directly to
  `delivery-issue` when they already publish a complete Delivery Contract and
  native relationships. Never copy, modify, or vendor those skills.
- The supported external route is `to-spec` → `to-tickets` → `delivery-issue`;
  this skill is optional when that route already satisfied repository
  publication requirements.
- Use `needs-triage` for raw, ambiguous, rejected, or incomplete work. Apply
  `ready-for-agent` to every complete Delivery Contract; open native blockers
  determine Execution Frontier membership without readiness-label churn.
- `gh issue create --template ... --body-file ...` does not work. Use a template-shaped body file plus explicit labels.
- Inspect `gh issue create --help`, `gh issue edit --help`, and
  `gh issue view --help` before sliced publication. When the installed CLI
  exposes native hierarchy and dependency flags, using them is required.

## Delivery handoff

Every issue handed to `delivery-issue` needs one complete, unambiguous Delivery
Contract. This skill normally publishes a complete standalone body or a native
ticket-plus-parent graph. A complete historical Agent Brief remains compatible
but is not required for new work. Follow
`../../../docs/agents/delivery-contract.md`.

## Workflow

1. Verify repo and search duplicates.
   ```bash
   gh repo view --json nameWithOwner
   gh issue list --state all --search "<keywords>" --json number,title,state,labels,url --limit 20
   ```
2. Choose the shape:
   - Bug: broken behavior with reproducible evidence.
   - Feature: user-visible improvement or capability.
   - PRD / agent-ready work: implementation-ready outcome, requirements, and acceptance criteria.
   - Sliced plan: multiple PRD-style vertical slices.
3. Draft a body using the headings in [REFERENCE.md](REFERENCE.md).
4. For sliced work, map the hierarchy and every blocking edge before
   publication. Create blockers before dependents, attach every child to its
   tracking parent, and add every blocking edge with native GitHub
   relationships when supported.
5. Create with `--body-file` and an explicit `bug` or `enhancement` category
   label. Do not add issue-level `type:*` labels. Start raw or incomplete work in
   `needs-triage`; apply `ready-for-agent` only after completeness verifies.
6. For standalone work, make the issue body a complete Contract Source. For
   sliced work, make the parent specification and each ticket body complete in
   composition with their native relationships.
7. Verify each published issue's body, `parent`, `subIssues`, `blockedBy`,
   `blocking`, labels, and selected Contract Source. Stop and repair missing
   edges, unresolved placeholders, incomplete sources, or incorrect labels
   before reporting success.
8. Return issue URL(s), native relationship evidence, category/readiness labels,
   Contract Source evidence, and duplicate search results.

## CLI patterns

Bug:
```bash
gh issue create --title "fix(scope): concise bug" \
  --body-file /tmp/dots-issue.md \
  --label bug --label needs-triage
```

Feature:
```bash
gh issue create --title "feat(scope): concise feature" \
  --body-file /tmp/dots-issue.md \
  --label enhancement --label needs-triage
```

Agent-ready PRD or approved standalone implementation issue:
```bash
gh issue create --title "feat(scope): concise outcome" \
  --body-file /tmp/dots-issue.md \
  --label enhancement --label ready-for-agent
```

## Slicing rules

For multiple issues, publish blockers first. Each delivery issue should be
independently reviewable, demonstrable, and small enough for one PR. Use native
relationships whenever the installed CLI exposes them:

```bash
# Attach relationships during creation.
gh issue create --title "feat(scope): child" --body-file /tmp/child.md \
  --label enhancement --parent <parent-number> --blocked-by <blocker-number>

# Or attach relationships after issue numbers exist (choose one hierarchy direction).
gh issue edit <child-number> --parent <parent-number> \
  --add-blocked-by <blocker-number>
# Equivalent inverse when updating the parent instead of the child:
gh issue edit <parent-number> --add-sub-issue <child-number>
```

`--parent` and `--add-sub-issue` express the same hierarchy from opposite
directions; one verified native edge is sufficient. Likewise, use
`--blocked-by`/`--add-blocked-by` (or the inverse
`--blocking`/`--add-blocking`) for every dependency edge. Textual `Parent` or
`Blocked by` sections may mirror native relationships for readers, but are not
the source of truth. Use them as the only representation solely when the target
tracker or installed CLI does not expose native relationships, and record that
fallback explicitly.

### Readiness states

- **Unreviewed:** keep `needs-triage`; do not apply `ready-for-agent`.
- **Fully specified and blocked:** apply `ready-for-agent` and rely on the native
  blocked state to keep the Delivery Unit outside the Execution Frontier.
- **Unblocked delivery frontier:** after verifying a complete Delivery Contract
  and no open blockers, apply `ready-for-agent`; the issue is in the Execution
  Frontier.
- **Tracking Issue:** apply `ready-for-agent` after its complete specification
  and native sub-issues verify. Delivery returns `tracking`; its children carry
  implementation.

When a blocker closes, re-read the dependent issue's native `blockedBy` state
and Delivery Contract. The issue enters the Execution Frontier without a label
transition. Repeat relationship verification whenever the frontier moves.

### Publication verification

Use machine-readable output rather than body prose:

```bash
gh issue view <issue-number> \
  --json body,parent,subIssues,blockedBy,blocking,labels,comments \
  --jq '{body, parent, subIssues, blockedBy, blocking, labels: [.labels[].name], historicalAgentBriefs: [.comments[].body | select(test("(?m)^## Agent Brief$"))]}'
```

Check both ends of the graph where applicable: the parent lists the child under
`subIssues`, the child reports `parent`, blockers and dependents agree through
`blockedBy`/`blocking`, labels match the states above, and the selected standalone
or composed Contract Source is complete. Historical Agent Briefs are optional;
if present, their count and completeness must remain unambiguous. Inspect `body`
for unresolved placeholders such as `<issue-number>`, `<N>`, or `TBD`.

See [REFERENCE.md](REFERENCE.md) for body templates and field guidance.
