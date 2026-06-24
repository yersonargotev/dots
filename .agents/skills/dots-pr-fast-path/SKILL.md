---
name: dots-pr-fast-path
description: Compact issue-first PR workflow for small or already-implemented changes in yersonargotev/dots. Use when shipping a focused dots change needs duplicate search, issue linkage, validation, commit, PR, and CI with minimal context/output.
---

# Dots PR Fast Path

Ship a small dots change through issue, commit, PR, and CI without the full
ceremony. The leading word is **tight**: minimal context, enough evidence, no
shortcut around repo rules.

## Use when

- The change is small, focused, and already implemented or mechanical.
- One issue and one PR are enough.
- The user wants to ship the change, not design it.

## Escalate instead

Use `dots-issue-creation` and `dots-pr-creation` when requirements are unclear,
the diff is broad/risky, the change exceeds roughly 400 lines, or it touches
release automation, JSON output contracts, security-sensitive behavior, or
workflow docs you cannot verify quickly.

## Rules

- Keep output compact: `git diff --stat` first, then targeted hunks only.
- Do not run `dots install` against the real `$HOME`; use sandbox flags when
  validating dotfiles behavior.
- Do not touch unrelated edits.
- Every PR needs `Closes #N`, exactly one `type:*` label, validation evidence,
  Conventional Commit history, and no AI attribution.
- Focused checks while iterating; full CI-equivalent checks before ready PR unless
  the diff is docs/skills-only and the PR explains the skip.

## Workflow

1. Verify repo and state:
   ```bash
   gh repo view --json nameWithOwner
   git branch --show-current
   git status --short
   git diff --stat
   ```
2. Search duplicates and choose/create the issue:
   ```bash
   gh issue list --state all --search "<keywords>" --json number,title,state,labels,url --limit 10
   gh issue view <N> --json number,title,state,labels,url
   ```
   Create a compact issue only when no duplicate fits; add `ready-for-agent` only
   when the issue is implementation-ready.
3. Inspect ownership with targeted diffs:
   ```bash
   git diff --stat
   git diff -- <file-or-path>
   ```
4. Validate by risk:
   - Go behavior: focused package tests, then `gofmt -l .`, `go vet ./...`,
     `go build ./...`, `go test ./...` before ready PR.
   - Docs/skills only: validate frontmatter, links/symlinks, JSON/YAML, and
     registry/cache when touched; explain skipped Go checks.
   - Dotfiles behavior: validate only with temp `--home`, `--source-root`, or
     `--state-root` where supported.
5. Stage and commit intentionally:
   ```bash
   git add <files>
   git status --short
   git commit -m "type(scope): concise outcome"
   ```
6. Open and verify the PR:
   ```bash
   gh pr create --repo yersonargotev/dots --title "type(scope): concise outcome" --body-file /tmp/dots-pr.md --label type:feature
   gh pr view --json number,title,url,labels,closingIssuesReferences,isDraft,headRefName,baseRefName
   gh pr checks --watch
   ```

## Compact PR body

```md
## Summary
- ...

## Linked issue
Closes #N

## Validation
- [x] `command` — passed

## Dotfiles safety
- [x] Not applicable, or sandbox evidence here.
```

## Output

Return only issue URL, PR URL, branch, commit hash, validation result, and any
unrelated local changes left untouched.
