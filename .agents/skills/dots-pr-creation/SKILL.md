---
name: dots-pr-creation
description: Create pull requests for the dots repository using its issue-first workflow, labels, template, validation, and dotfiles safety rules. Use when opening, preparing, or reviewing a PR for yersonargotev/dots, especially after implementing an issue or before running branch-pr.
---

# Dots PR Creation

Create PRs for `yersonargotev/dots` without importing generic workflow mistakes.

## Ground truth

Read these when stale or before opening a PR:

- `AGENTS.md` for contribution rules and validation commands
- `.github/PULL_REQUEST_TEMPLATE.md` for required body sections
- `docs/agents/issue-tracker.md` and `docs/agents/triage-labels.md` for issue/label workflow
- `docs/agents/output-contract.md` when the PR touches JSON output or exit codes
- `CONTEXT.md` when the PR touches domain concepts

## What this corrects

- Generic `branch-pr` may mention `status:approved`; dots uses `ready-for-agent` for agent-ready issues.
- `.atl/` is gitignored, so registry/cache changes may need explicit `git add -f` or index updates.
- Every PR must link an issue with `Closes #N`, use exactly one `type:*` label, fill the PR template, use conventional commits, and include no AI attribution.

## Fast path

Use `dots-pr-fast-path` for small, already-implemented changes. Return to this
full workflow when the change is broad, risky, lacks a clear issue, touches
release/output-contract behavior, or the fast path finds stale workflow evidence.

## Workflow

1. Verify repo, branch, issue, and working tree without reverting others' edits.
   ```bash
   gh repo view --json nameWithOwner
   git branch --show-current
   git status --short
   gh issue view <N> --json number,title,state,labels,url
   ```
2. Confirm the linked issue is open and agent-ready when appropriate: `ready-for-agent`, not `status:approved`.
3. Review changes and ownership scope.
   ```bash
   git diff --stat
   git diff -- . ':(exclude).atl/*'
   git diff -- .atl/skill-registry.md .atl/.skill-registry.cache.json
   ```
4. Validate locally unless clearly not applicable. Match CI:
   ```bash
   gofmt -l .
   go vet ./...
   go build ./...
   go test ./...
   ```
   For docs/skill-only PRs, still validate JSON/YAML/symlinks and state why Go validation was skipped or run.
5. Stage intentionally. Include gitignored `.atl` files only when they are part of the change.
   ```bash
   git add <files>
   git add -f .atl/skill-registry.md .atl/.skill-registry.cache.json
   git status --short
   ```
6. Commit with a conventional commit, no `Co-Authored-By` or AI attribution.
7. Create the PR with the filled template body and exactly one `type:*` label.
   ```bash
   gh pr create --repo yersonargotev/dots \
     --title "feat(scope): concise outcome" \
     --body-file /tmp/dots-pr.md \
     --label type:feature
   ```
8. Verify the opened PR.
   ```bash
   gh pr view --json number,title,url,body,labels,closingIssuesReferences,headRefName,baseRefName
   ```

## Output contract

Return:

- PR URL or “not created” reason
- linked issue (`Closes #N`) and issue readiness evidence
- exactly one `type:*` label
- validation commands with results or explicit skip rationale
- staged/committed files, including any force-added `.atl` files
- warnings about unrelated local edits left untouched

See [REFERENCE.md](REFERENCE.md) for PR body, label, validation, and safety checklists.
