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
- `CONTEXT.md` when issue text touches domain concepts

## Repository conventions

- Treat this repository's issue, relationship, Agent Brief, and readiness rules
  as authoritative when composing with generic planning skills such as
  `to-spec` or `to-tickets`. Those skills may shape or slice the plan, but they
  must not choose repository labels or replace native GitHub relationships with
  prose.
- Use `needs-triage` for unreviewed work and `ready-for-agent` only for the
  current unblocked delivery frontier. Do not use either label to represent a
  native dependency state.
- `gh issue create --template ... --body-file ...` does not work. Use a template-shaped body file plus explicit labels.
- Inspect `gh issue create --help`, `gh issue edit --help`, and
  `gh issue view --help` before sliced publication. When the installed CLI
  exposes native hierarchy and dependency flags, using them is required.

## Delivery handoff

Every issue handed to `delivery-issue` requires exactly one complete Agent Brief
before it receives `ready-for-agent`. There is no delivery fast path around that
contract. Use the format in `../../../docs/agents/agent-brief.md`.

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
5. Create with `--body-file` and explicit category/type labels. Start ordinary
   unsliced issues in `needs-triage`; use the state transitions below for a
   sliced plan.
6. Create or update exactly one complete Agent Brief for every delivery issue.
   Then apply the readiness state only after relationships and briefs verify.
7. Verify each published issue's body, `parent`, `subIssues`, `blockedBy`,
   `blocking`, labels, and Agent Brief count. Every delivery child requires
   exactly one complete brief; a tracking parent must never have duplicate
   briefs. Stop and repair missing edges, unresolved placeholders, incorrect
   labels, or incorrect brief counts before reporting success.
8. Return issue URL(s), native relationship evidence, labels, Agent Brief
   evidence, and duplicate search results.

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

Agent-ready PRD or approved implementation issue:
```bash
gh issue create --title "feat(scope): concise outcome" \
  --body-file /tmp/dots-issue.md \
  --label enhancement --label needs-triage

gh issue comment <N> --body-file /tmp/dots-agent-brief.md
gh issue edit <N> --remove-label needs-triage --add-label ready-for-agent
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
- **Fully specified and blocked:** create or update its one complete Agent
  Brief, remove both `needs-triage` and `ready-for-agent`, and rely on the native
  blocked state while any `blockedBy` issue remains open.
- **Unblocked delivery frontier:** after verifying exactly one complete Agent
  Brief and no open blockers, remove `needs-triage` and apply
  `ready-for-agent`.
- **Tracking parent:** after slicing a PRD, remove `ready-for-agent` and
  `needs-triage`. The parent tracks the plan and is not a `delivery-issue` unit;
  its children carry delivery readiness. Retain at most one existing Agent
  Brief as historical context, but do not create another brief solely for the
  tracking parent.

When a blocker closes, re-read the dependent issue's native `blockedBy` state
and Agent Brief. Promote each newly unblocked, fully specified child to
`ready-for-agent`; leave descendants with open blockers label-free. There is no
automatic label transition, so repeat this check whenever the frontier moves.

### Publication verification

Use machine-readable output rather than body prose:

```bash
gh issue view <issue-number> \
  --json body,parent,subIssues,blockedBy,blocking,labels,comments \
  --jq '{body, parent, subIssues, blockedBy, blocking, labels: [.labels[].name], agentBriefCount: ([.comments[].body | select(test("(?m)^## Agent Brief$"))] | length)}'
```

Check both ends of the graph where applicable: the parent lists the child under
`subIssues`, the child reports `parent`, blockers and dependents agree through
`blockedBy`/`blocking`, labels match the four states above, every delivery child
has one Agent Brief, and a tracking parent has no duplicate briefs. Inspect
`body` for unresolved placeholders such as `<issue-number>`, `<N>`, or `TBD`.

See [REFERENCE.md](REFERENCE.md) for body templates and field guidance.
