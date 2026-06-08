# v1 Scope and Deferred Work

v1 proves that the Dotfiles CLI can install repository-owned configuration safely and handle missing Dependencies with explicit guardrails before the project expands into update orchestration or larger application configuration. These boundaries protect installer correctness, preserve the Source of Truth, and prevent Machine-Specific Configuration from leaking into shared Managed Configuration.

## Quick review path

1. Use **v1 includes** to confirm what the first usable release must support.
2. Use **deferred work** to reject scope creep during v1 implementation.
3. Use **why these boundaries exist** when deciding whether a new request belongs in v1 or later.

## v1 includes

| Area | v1 boundary |
|------|-------------|
| Safe installation | The Dotfiles CLI owns installation through an Install Manifest, computes an Install Plan before changing files, supports dry-run behavior, detects Conflicts, and keeps non-interactive behavior conservative. |
| Conflict safety | Conflict Resolution supports explicit choices such as `skip`, `replace`, `adopt`, and `diff`; replacement requires a Backup Set first, and adoption must be explicit to avoid contaminating the Source of Truth. |
| Status | `dots status` reports Dotfiles Status, including whether Managed Entries are installed, missing, conflicting, skipped, drifted, or unsupported. |
| Diagnostics | `dots doctor` reports platform, dependency, secret, and configuration concerns without pretending that guardrails are a complete audit. |
| Dependency management | `dots deps check`, `dots deps plan`, and `dots deps install` detect missing Dependencies, show OS-aware guidance, preview installable actions, and execute package-manager commands only after `--yes` or interactive confirmation. Manual-only Dependencies remain manual. |
| Backups list | `dots backups list` exposes Backup Sets and Backup Metadata so preserved files can be audited after installation. |
| Release artifacts | GitHub Releases publish platform-specific Release Artifacts for macOS amd64/arm64 and Linux amd64/arm64. |
| Bootstrapper support | The Bootstrapper downloads the matching Release Artifact, performs Checksum Verification, installs or locates `dots`, and delegates setup to the Dotfiles CLI. |
| Homebrew Distribution | The release workflow generates a tap formula from the same Release Artifacts and checksum manifest, then publishes it to `yersonargotev/homebrew-tap`. |
| MVP Configuration Set | v1 proves the installer with zsh, git, starship, and tmux before migrating larger application configurations. |

## Deferred to v1.1 or later

| Deferred item | Earliest target | Why it is deferred |
|---------------|-----------------|--------------------|
| Advanced dependency orchestration | Later than v1 | Rollback, version constraints, repository/tap/index refresh, reinstall, and upgrade behavior introduce package-manager state decisions beyond the guarded v1 install flow. |
| `dots update` | v1.1 — **shipped** | Updating the Installed Repository introduces Git state, local changes, versioning, and post-update conflict handling. Delivered in v1.1; see [`docs/update.md`](update.md). |
| Neovim and larger application configurations | Later than v1 | Larger app configs introduce plugin, language-server, and dependency complexity that distracts from proving installer correctness. |
| Windows support | Later than v1 | v1 focuses on macOS and Linux Supported Platforms only. Windows needs separate path, shell, package, and platform behavior decisions. |
| Official WSL support | Later than v1 | WSL has mixed Windows/Linux filesystem and dependency concerns that should not be treated as generic Linux during the first release. |
| NixOS specialization | Later than v1 | NixOS requires specialized package and system-configuration assumptions that do not fit the v1 advisory Dependency Plan. |
| Alpine/musl specialization | Later than v1 | Alpine and musl introduce distribution and binary compatibility concerns outside the initial Linux amd64/arm64 release target. |
| Automatic backup restore | v1.1 — **shipped** | v1 made Backup Sets visible and reliable via `dots backups list`; with Backup Metadata proven, `dots backups restore` returns targets to a preserved Backup Set, refuses sets from another machine without `--force`, supports `--dry-run`, and backs up overwritten targets first. Delivered in v1.1; see [`docs/backups.md`](backups.md). |
| Full visual TUI snapshot testing | Later than v1 | v1 tests TUI state transitions, but installer correctness matters more than pixel-perfect terminal rendering. |

## Why these boundaries exist

### Installer safety comes before convenience

The Bootstrapper is intentionally thin. It downloads, verifies, and launches the Dotfiles CLI; it does not duplicate manifest parsing, conflict handling, backup behavior, or configuration installation. That keeps the risky filesystem logic inside testable Go code instead of spreading it across shell entrypoints.

### Source of Truth integrity matters more than speed

The repository-owned Source of Truth must not absorb accidental local state. Adoption exists for explicit migrations only. Machine-Specific Configuration belongs in Local Extension Points, ignored files, or external secret stores, not in shared Managed Configuration.

### Dependency installation stays behind explicit consent

A Dependency Plan helps the user understand missing tools before any package-manager command runs. `dots deps install` uses the same preview, asks for confirmation by default, and executes direct argv-shaped package-manager actions only for installable Dependencies. It does not bypass `sudo`, does not execute manual guidance, and does not claim rollback, version constraints, reinstall, or upgrade behavior.

### Distribution starts with verifiable artifacts

v1 distribution starts with GitHub Release Artifacts plus Checksum Verification because every install path can verify exactly what it downloads. Homebrew Distribution now uses that same artifact contract: the formula is generated from `checksums.txt`, selects the correct macOS/Linux amd64/arm64 binary, and delegates all setup behavior to the installed `dots` CLI.

### The MVP Configuration Set keeps the feedback loop tight

zsh, git, starship, and tmux are enough to prove symlink, template, copy, Local Extension Point, dependency, status, backup, and conflict behavior. Pulling Neovim or larger apps into v1 would blur whether failures come from installer design or application-specific complexity.

## Alignment references

- [PRD: Dotfiles CLI and bootstrapper](https://github.com/yersonargotev/dots/issues/1) — product scope, user stories, implementation decisions, testing decisions, and out-of-scope items.
- [`CONTEXT.md`](../CONTEXT.md) — canonical vocabulary for Source of Truth, Managed Entry, Install Plan, Backup Set, Drift, Homebrew Distribution, Deferred Configuration, and related terms.
- [`docs/adr/0001-bootstrap-with-go-cli.md`](adr/0001-bootstrap-with-go-cli.md) — architectural decision record for the Bootstrapper, Go CLI, manifest contract, backups, status, dependencies, supported platforms, and distribution sequence.
- [`docs/release.md`](release.md) — release workflow and Checksum Verification contract for Bootstrapper-ready artifacts.

## Scope checklist

Use this checklist when reviewing v1 issues or PRs:

- [ ] The change strengthens safe installation, status, diagnostics, guarded dependency management, backups, release artifacts, checksum Bootstrapper support, or Homebrew Distribution.
- [ ] The change does not add advanced dependency orchestration, larger Deferred Configuration, or unsupported platform behavior to v1.
- [ ] The change preserves Source of Truth integrity and does not make Machine-Specific Configuration part of shared Managed Configuration.
- [ ] The change uses the project vocabulary from `CONTEXT.md` and the tradeoffs recorded in the ADR.
