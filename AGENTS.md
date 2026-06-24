# Agent guide

Shared instructions for autonomous agents working in this repository. `CLAUDE.md`
is a symlink to this file, so Claude Code and any tool that reads `AGENTS.md`
(Codex, Cursor, OpenCode, Gemini CLI) follow the same guide.

`dots` is a safe Dotfiles CLI written in Go. Read [`CONTEXT.md`](CONTEXT.md) for
the domain glossary before writing PRDs, issues, plans, or architecture reviews,
and use its terms consistently (`Source of Truth`, `Install Manifest`,
`Managed Entry`, `Install Plan`, `Conflict`, `Backup Set`, `Drift`).


## Token discipline

Use the smallest evidence that proves the point. This repo has many generated,
vendored, and agent-support files, so broad reads waste context quickly.

- Prefer `rg`, `sed -n`, `git diff --stat`, and targeted file reads over `cat`
  or broad recursive output.
- Do not print full diffs by default. Start with `git diff --stat`, then inspect
  only the files or hunks that matter.
- Load long skill/docs references only when the workflow is stale, ambiguous,
  failing, or the current task needs that section.
- Use CodeGraph for source-code architecture, symbols, call flow, and impact
  analysis. For manifest, docs, config, and script changes, prefer `rg`, `sed`,
  targeted reads, and tests. Never use CodeGraph just because `.codegraph/`
  exists.
- Validate in two phases: focused package/file checks while iterating, then the
  full CI-equivalent suite before opening or marking a PR ready.

## Build, test, and verify

Match CI before pushing (`.github/workflows/ci.yml`):

```bash
gofmt -l .        # must print nothing; run `gofmt -w .` to fix
go vet ./...
go build ./...
go test ./...
```

Run a single package or test while iterating:

```bash
go test ./internal/cli/...
go test ./internal/cli/ -run TestEnvelopeGolden
```

## Driving the CLI as an agent

Prefer the machine-readable surface over scraping human text. The read-only
diagnostic commands emit a stable JSON envelope and semantic exit codes:

```bash
dots status --output json   # exit 0 aligned, 2 findings, 1 error
dots doctor --output json
dots plan   --output json
dots deps check --output json
```

Branch on the exit code, not on prose: `0` aligned, `2` divergence to act on,
`1` execution error. See [`docs/agents/output-contract.md`](docs/agents/output-contract.md)
for the envelope shape, field scope, and exit-code rules, and
[`docs/adr/0006-agent-output-contract.md`](docs/adr/0006-agent-output-contract.md)
for the rationale.

## Dotfiles validation safety

Never validate dotfiles behavior against the user's real home configuration. Do
not run `dots install` against the real `$HOME`, and do not write to live files
such as `~/.zshrc`, `~/.tmux.conf`, `~/.gitconfig`, or `~/.config/starship.toml`.
Use temporary directories plus explicit flags like `--home <tmp>`,
`--source-root "$PWD"`, and `--state-root <tmp>` where supported.

```bash
SANDBOX="$(mktemp -d)"
dots doctor --home "$SANDBOX"   # inspect without touching real config
```

`deps` is the exception: it manages system-global tools and package managers,
not `$HOME` files, so it has no `--source-root`/`--state-root` to sandbox and
those flags are not offered. The only honest sandbox knob is `--home` on
`deps check` and `deps plan`, which roots **font detection** at the environment
under test instead of the operator's real home:

```bash
SANDBOX="$(mktemp -d)"; mkdir -p "$SANDBOX/home"
dots deps check --file dots.yaml --home "$SANDBOX/home"
dots deps plan  --file dots.yaml --home "$SANDBOX/home"
```

`deps install` takes no `--home`: it would only relabel font detection while the
real install still hits the system, so its guardrails stay `--dry-run` and
confirmation instead.

## Contribution conventions

- **Issue first.** Issues and PRDs live in GitHub Issues for `yersonargotev/dots`.
  See [`docs/agents/issue-tracker.md`](docs/agents/issue-tracker.md). New issues
  use the templates under `.github/ISSUE_TEMPLATE/`.
- **Triage labels.** This repo uses `needs-triage`, `needs-info`, `ready-for-agent`,
  `ready-for-human`, and `wontfix`. See
  [`docs/agents/triage-labels.md`](docs/agents/triage-labels.md).
- **Conventional Commits.** Commit messages follow `type(scope): summary`
  (e.g. `feat(cli): ...`, `fix(doctor): ...`, `docs(adr): ...`).
- **Pull requests.** Use [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md).
  Include `Closes #<issue-number>`, exactly one `type:*` label, validation
  evidence, and the dotfiles safety checklist when config paths are involved.
- **Docs follow the same flow.** Documentation changes (`*.md`, `docs/`,
  including this file) go through issue + PR like code — never push docs directly
  to `main`. Open an issue, branch, then a PR labeled `type:docs` with
  `Closes #<issue-number>`; `main` is branch-protected and requires the
  `Test, vet and build` check on every PR.
- **Domain docs.** Single-context layout: `CONTEXT.md` at the root and ADRs under
  `docs/adr/`. Read the relevant ADR before changing architecture or workflow.
  See [`docs/agents/domain.md`](docs/agents/domain.md).
