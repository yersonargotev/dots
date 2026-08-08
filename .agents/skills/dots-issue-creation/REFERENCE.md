# Dots Issue Body Reference

Use these headings with `gh issue create --body-file`. Keep bodies concise and actionable.

## Bug

```md
### Pre-flight checks
- [x] I searched existing issues and this is not a duplicate.
- [x] I can reproduce this without writing to my real home dotfiles.

### Bug description

### Steps to reproduce
1.

### Expected behavior

### Actual behavior

### Operating system
macOS | Linux | WSL | Other

### Shell
zsh | bash | fish | other

### Relevant logs
```

## Feature

```md
### Pre-flight checks
- [x] I searched existing issues and this is not a duplicate.
- [x] This is an actionable change, not a support question.

### Problem

### Proposed solution

### Affected area
CLI | Dotfiles configuration | Installation workflow | Documentation | CI / release | Agent workflow | Other

### Alternatives considered

### Additional context
```

## PRD / sliced work

```md
### Pre-flight checks
- [x] I searched existing issues and this is not a duplicate.
- [x] The scope is small enough for one reviewable work unit or clearly identifies slices.

### Desired outcome

### Current-state gap

### Requirements
- The workflow must ...

### Non-goals

### Acceptance criteria
- [ ] ...

### Validation notes
- `go test ./...`
- `go vet ./...`
- `go run ./cmd/dots manifest validate --file dots.yaml`
- Use temporary directories for home/config behavior.
```

## Optional relationship mirrors for sliced issues

Native GitHub `parent`/`subIssues` and `blockedBy`/`blocking` relationships are
the source of truth. Add these sections only as a readable mirror after native
relationships are created, or as an explicitly reported fallback when the
target tracker or installed CLI has no native relationship support.

```md
### Parent
- #123

### Blocked by
- None - can start immediately
```

```md
### Parent
- #123

### Blocked by
- #124
```

Never leave placeholder issue numbers in a published body. A text-only section
does not satisfy the sliced-work relationship contract when the supported
native flags are available.
