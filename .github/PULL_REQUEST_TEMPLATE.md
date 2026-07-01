Closes #<issue-number>

## PR Type

Check exactly one and add the matching `type:*` label to the PR.

- [ ] Bug fix (`type:bug`)
- [ ] New feature (`type:feature`)
- [ ] Documentation only (`type:docs`)
- [ ] Code refactoring (`type:refactor`)
- [ ] Maintenance/tooling (`type:chore`)
- [ ] Breaking change (`type:breaking-change`)

## Summary

-
-
-

## Changes

| File | Change |
|------|--------|
| `path/to/file` | What changed and why. |

## Test Plan

- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] `gofmt -l .`
- [ ] `go run ./cmd/dots manifest validate --file dots.yaml`
- [ ] Sandboxed plan only when dotfiles behavior changes: `go run ./cmd/dots plan --file dots.yaml --profile core --source-root "$PWD" --home <tmp>`

## Dotfiles Safety

- [ ] I did not run `dots install` against the real home directory.
- [ ] I did not write to real local configuration such as `~/.zshrc`, `~/.tmux.conf`, `~/.gitconfig`, or `~/.config/starship.toml`.
- [ ] Any CLI validation that targets home/config paths used temporary directories and explicit flags such as `--home <tmp>`, `--source-root "$PWD"`, and `--state-root <tmp>` where supported.

## Contributor Checklist

- [ ] Linked an approved / `ready-for-agent` issue with `Closes #<issue-number>`.
- [ ] Added exactly one `type:*` label.
- [ ] Kept the PR scoped to one reviewable work unit.
- [ ] Updated docs or agent instructions when workflow behavior changed.
- [ ] Used conventional commit format.
- [ ] Did not add `Co-Authored-By` or AI attribution trailers.
