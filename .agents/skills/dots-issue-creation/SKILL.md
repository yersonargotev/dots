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

## What this corrects

- `to-issues` is useful for splitting plans into vertical tracer-bullet slices.
- The generic `issue-creation` skill is only a rough reference here: this repo uses `needs-triage` and `ready-for-agent`, not `status:needs-review` / `status:approved`.
- `gh issue create --template ... --body-file ...` does not work. Use a template-shaped body file plus explicit labels.

## Fast path

Use `dots-pr-fast-path` when a small change is already implemented and only
needs the standard issue + commit + PR path. Use this full issue workflow for
planning, multiple slices, unclear requirements, or duplicate-heavy triage.

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
4. Create with `--body-file` and explicit labels.
5. Add `ready-for-agent` only when the issue is sufficiently specified for an agent to implement without extra product context.
6. Return issue URL(s), labels, duplicate search result, and dependency notes.

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
  --label needs-triage --label ready-for-agent
```

## Slicing rules

For multiple issues, publish blockers first and reference real issue numbers in dependent issues. Each issue should be independently reviewable, demoable, and small enough for one PR unless the body explicitly calls out slices.

See [REFERENCE.md](REFERENCE.md) for body templates and field guidance.
