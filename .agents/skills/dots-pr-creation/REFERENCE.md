# Dots PR Creation Reference

## PR body checklist

Use `.github/PULL_REQUEST_TEMPLATE.md` as source of truth. A complete PR body should include:

- `Closes #N` near the top or in the template's issue section.
- Summary of the user-visible or agent-workflow outcome.
- Validation evidence with exact commands and pass/fail/skip status.
- Dotfiles safety note when config paths, install behavior, home-directory behavior, or dependency installation are involved.
- Risk/rollback notes for workflow, release, or output-contract changes.

Minimal body shape when using `--body-file`:

```md
## Summary
- ...

## Linked issue
Closes #N

## Validation
- [ ] `gofmt -l .`
- [ ] `go vet ./...`
- [ ] `go build ./...`
- [ ] `go test ./...`

## Dotfiles safety
- [ ] Did not run `dots install` against the real `$HOME`.
- [ ] Used temporary directories and explicit `--home`, `--source-root`, or `--state-root` where applicable.
- [ ] Not applicable: explain why.
```

Prefer the real template over this minimal shape whenever available.

## Labels

Exactly one `type:*` label must be on the PR. Choose by dominant change:

- `type:feature` for new user-facing or agent workflow capability.
- `type:bug` for bug fixes.
- `type:docs` for documentation-only changes.
- `type:refactor` for code restructuring without behavior change.
- `type:chore` for maintenance that is not user-facing.
- `type:breaking-change` for breaking behavior or workflow changes.

Do not add generic status labels copied from other repos. For issue readiness, dots uses `ready-for-agent` on issues.

## Validation rules

Default CI-equivalent validation:

```bash
gofmt -l .
go vet ./...
go build ./...
go test ./...
```

For docs/skill-only changes, lightweight validation may be enough before opening a draft, but the PR must clearly say what was skipped and why. Useful checks:

```bash
test -f .agents/skills/<skill>/SKILL.md
python3 - <<'PY'
import json, pathlib
json.load(open('.atl/.skill-registry.cache.json'))
for p in pathlib.Path('.agents/skills').glob('*/agents/openai.yaml'):
    print(p)
PY
test "$(readlink .claude/skills/<skill>)" = "../../.agents/skills/<skill>"
grep -n '| `<skill>` |' .atl/skill-registry.md
```

If YAML tooling is installed, validate metadata with `python3 -c 'import yaml,sys; yaml.safe_load(open(sys.argv[1]))' <file>` or the repo's preferred validator.

## Safety guardrails

- Never run `dots install` against the real `$HOME`.
- Prefer machine-readable CLI output when validating behavior: `--output json` and exit codes `0`, `1`, `2` per `docs/agents/output-contract.md`.
- Do not revert, overwrite, or restage unrelated work in the tree.
- Do not force-push unless the user explicitly asks and understands the impact.
- Do not open a PR without `Closes #N` unless the user explicitly confirms an exception.
