# Agent guide

Shared instructions for autonomous agents working in this repository. `CLAUDE.md`
is a symlink to this file, so Claude Code and any tool that reads `AGENTS.md`
(Codex, Cursor, OpenCode, Gemini CLI) follow the same guide.

`dots` is a safe Dotfiles CLI written in Go. Read [`CONTEXT.md`](CONTEXT.md) for
the domain glossary before writing PRDs, issues, plans, or architecture reviews,
and use its terms consistently (`Source of Truth`, `Install Manifest`,
`Managed Entry`, `Install Plan`, `Conflict`, `Backup Set`, `Drift`).

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
- **Domain docs.** Single-context layout: `CONTEXT.md` at the root and ADRs under
  `docs/adr/`. Read the relevant ADR before changing architecture or workflow.
  See [`docs/agents/domain.md`](docs/agents/domain.md).
