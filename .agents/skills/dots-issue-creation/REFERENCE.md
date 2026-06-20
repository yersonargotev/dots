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

## Dependency notes for sliced issues

Add one of these sections when publishing multiple related issues:

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
