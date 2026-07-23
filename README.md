# dots

`dots` is a safe Dotfiles CLI for installing repository-owned shell, git, starship, and tmux configuration. It treats this repository as the Source of Truth, reads an Install Manifest, shows an Install Plan before changing files, and preserves user-owned configuration with Backup Sets instead of overwriting blindly.

The Bootstrapper in [`scripts/install.sh`](scripts/install.sh) installs the released `dots` binary, verifies the matching SHA-256 checksum, and then delegates setup to the tested Go CLI. That keeps install behavior in one place instead of duplicating dangerous filesystem logic in shell.

## Install

### Bootstrapper

Install the latest published Release Artifact and verify it against the published checksum manifest:

```bash
curl -fsSL https://raw.githubusercontent.com/yersonargotev/dots/main/scripts/install.sh | bash
```

The Bootstrapper:

1. Detects the current Supported Platform.
2. Downloads the matching GitHub Release Artifact and `checksums.txt` from the latest release by default.
3. Performs Checksum Verification before installing the binary to `~/.local/bin/dots`.
4. Clones the Source of Truth to `~/.local/share/dots` when the default Installed Repository is missing or empty, using `DOTS_VERSION` as the Git ref when pinned.
5. Runs `dots install` so the Dotfiles CLI owns the Install Plan and file changes.

Set `DOTS_VERSION=v0.x.y` only when you intentionally need to pin a specific release; the default Source of Truth clone uses that tag too. Use `DOTS_REPOSITORY_REF=<ref>` only when the binary release and Source of Truth ref must differ. Use `DOTS_SOURCE_ROOT=/path/to/checkout` for development checkouts.

Requirements: `curl` or `wget`, `sha256sum` or `shasum`, plus `git` for first-time bootstrap cloning. Make sure `~/.local/bin` is on your `PATH` after install.

### Homebrew

macOS and Linux users can also install the same checksum-backed release through the Homebrew Distribution:

```bash
brew install yersonargotev/tap/dots
```

That fully-qualified install is the preferred Tap Trust path because Homebrew trusts only the `dots` formula, not every current and future entry in `yersonargotev/tap`. If you keep the tap installed and want to use the short name, trust the formula before installing:

```bash
brew tap yersonargotev/tap
brew trust --formula yersonargotev/tap/dots
brew install dots
```

For Brewfile-managed machines, declare the formula trust with the package entry:

```ruby
brew "yersonargotev/tap/dots", trusted: true
```

Verify the binary is available:

```bash
dots --version
```

Homebrew installs only the released binary. Initialize the default Installed
Repository before running read-only diagnostics or install previews:

```bash
dots init
```

For released binaries, `dots init` clones the matching release tag by default.
Use `--repository-ref` only when you intentionally want a different Source of
Truth ref.

## Quickstart

Start with a dry run. If you installed through Homebrew, run `dots init` first.
The CLI should show the Install Plan without writing files:

```bash
dots install --profile workstation --dry-run
```

There is no implicit install Profile. Repeat `--profile` to compose selections:
`workstation` covers `core + desktop + agents`, while `web` and `mobile` stay
explicit opt-ins. For a full workstation plus optional web and mobile setup, run
`dots install --profile workstation --profile web --profile mobile`.

Then inspect the current machine state:

```bash
dots status
dots doctor
```

After a successful explicit install, `status`, `doctor`, `plan`, `deps check`,
and `deps plan` reuse the recorded Installed Selection when no `--profile` or
`--tag` is supplied. Supplying either flag makes that invocation's complete
explicit selection win without changing Installation Metadata. A machine with
no recorded selection must pass an explicit `--profile` or `--tag`; read-only
commands never invent an implicit Profile.

### Machine-readable output

Scripts and agents can request a stable JSON envelope from result-producing
commands with `--output json`. Read-only diagnostics also expose semantic
findings exit codes, so callers can branch without parsing human text:

```bash
dots status --output json   # exit 0 aligned, 2 findings to act on, 1 error
```

See [`docs/agents/output-contract.md`](docs/agents/output-contract.md) for the
envelope shape and exit-code contract.

Inspect and install missing Dependencies deliberately:

```bash
dots deps check          # report present and missing tools
dots deps plan           # show OS-aware guidance only
dots deps install --dry-run  # preview installable and manual actions
dots deps install        # preview, then ask before executing package managers
dots deps install --yes  # execute installable actions without prompting
```

`dots deps install` executes package managers with direct argv only after confirmation. It does not bypass `sudo`, does not run manual-only guidance, and does not promise rollback, version constraints, reinstall, or upgrade behavior.

For honest fresh-machine validation, `deps check` and `deps plan` accept
`--home` to root font detection and Installed Selection lookup at a sandbox
instead of your real home:

```bash
tmp=$(mktemp -d); mkdir -p "$tmp/home"
dots deps check --file dots.yaml --home "$tmp/home"
dots deps plan  --file dots.yaml --home "$tmp/home"
```

Unlike `doctor`/`install`, deps commands manage system-global tools rather than
`$HOME` files, so read-only deps commands derive both the Installed Repository
and Installation Metadata locations from `--home` instead of offering separate
`--source-root`/`--state-root` flags.

List Backup Metadata created by safe installs or restores:

```bash
dots backups list
```

When the plan looks right, run the install for real:

```bash
dots --version
dots install --profile workstation
```

Non-interactive installs stay conservative: `dots install --profile workstation --yes` skips Conflicts. To explicitly adopt an existing machine by backing up and replacing every Conflict, use:

```bash
dots install --profile workstation --yes --backup-and-replace
```

That mode creates Backup Sets before replacement, reports them in JSON as `data.backup_sets`, and still runs selected Provisioners after Managed Configuration is applied.

To reverse an install, `dots uninstall` removes only the symlinks and copied files `dots` recorded it owns, previewing first and skipping anything that drifted:

```bash
dots uninstall --dry-run          # preview the Uninstall Plan, change nothing
dots uninstall                    # preview, then ask before removing
dots uninstall --yes              # remove owned targets without prompting
dots uninstall --restore-backups  # also restore each target's pre-install Backup Set
```

## Core concepts

| Concept | Meaning |
|---------|---------|
| Source of Truth | This repository's tracked dotfiles and manifest are the canonical desired state. |
| Install Manifest | The manifest that maps repository files to home-directory targets. |
| Managed Entry | A target file or link managed by `dots`, such as shell, git, terminal, editor, or agent-tool config. |
| Install Plan | The preview of create, update, replace, skip, or conflict actions before install applies changes. |
| Uninstall Plan | The preview of remove, skip, modified, or not-owned actions before uninstall reverses an install, driven by the Installation Metadata. |
| Installation Metadata | Local state used to remember what `dots` installed. |
| Backup Set | A preserved copy of user-owned files before a restore or overwrite path changes them. |
| Dependency Plan | OS-aware guidance for missing tools, including which actions are installable and which remain manual. |
| Dependency Install | A guarded workflow that previews missing Dependency actions, asks for confirmation by default, and executes only installable package-manager actions. |

## Supported platforms

Release artifacts are published for:

| OS | Architecture |
|----|--------------|
| macOS | `amd64`, `arm64` |
| Linux | `amd64`, `arm64` |

## v1 scope boundary

The v1 scope focuses on safe repository-owned configuration declared in `dots.yaml`: shell, git, terminal, editor, and agent-tool config with explicit install strategies. It includes guarded dependency inspection and installation for supported package-manager tiers, but does not include Windows, official WSL support, NixOS-specific behavior, Alpine/musl specialization, arbitrary manifest hooks, dependency rollback, version constraints, reinstall, or upgrade orchestration.

## Canonical docs

- [`CONTEXT.md`](CONTEXT.md) — domain vocabulary, architecture context, and project model.
- [`docs/scope.md`](docs/scope.md) — v1 scope, non-goals, and deferred work.
- [`docs/uninstall.md`](docs/uninstall.md) — the reversible `dots uninstall` command and its ownership-safety model.
- [`docs/release.md`](docs/release.md) — release workflow, checksums, Homebrew, and Bootstrapper details.
- [`AGENTS.md`](AGENTS.md) — shared guide for autonomous agents (also exposed as `CLAUDE.md`).
- [`docs/agents/output-contract.md`](docs/agents/output-contract.md) — JSON envelope and semantic exit codes for agents and scripts.
- [`docs/adr/0001-bootstrap-with-go-cli.md`](docs/adr/0001-bootstrap-with-go-cli.md) — ADR for bootstrapping with the Go CLI.
