# dots

`dots` is a safe Dotfiles CLI for installing repository-owned shell, git, starship, and tmux configuration. It treats this repository as the Source of Truth, reads an Install Manifest, shows an Install Plan before changing files, and preserves user-owned configuration with Backup Sets instead of overwriting blindly.

The Bootstrapper in [`scripts/install.sh`](scripts/install.sh) installs the released `dots` binary, verifies the matching SHA-256 checksum, and then delegates setup to the tested Go CLI. That keeps install behavior in one place instead of duplicating dangerous filesystem logic in shell.

## Install

### Bootstrapper

Pin the version explicitly so the Bootstrapper downloads a known Release Artifact and verifies it against the published checksum manifest:

```bash
curl -fsSL https://raw.githubusercontent.com/yersonargotev/dots/main/scripts/install.sh | DOTS_VERSION=v0.1.0 bash
```

The Bootstrapper:

1. Detects the current Supported Platform.
2. Downloads the matching GitHub Release Artifact and `checksums.txt`.
3. Performs Checksum Verification before installing the binary to `~/.local/bin/dots`.
4. Runs `dots install` so the Dotfiles CLI owns the Install Plan and file changes.

Requirements: `curl` or `wget`, plus `sha256sum` or `shasum`. Make sure `~/.local/bin` is on your `PATH` after install.

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
dots --help
```

## Quickstart

Start with a dry run. The CLI should show the Install Plan without writing files:

```bash
dots install --dry-run
```

Then inspect the current machine state:

```bash
dots status
dots doctor
```

Inspect and install missing Dependencies deliberately:

```bash
dots deps check          # report present and missing tools
dots deps plan           # show OS-aware guidance only
dots deps install --dry-run  # preview installable and manual actions
dots deps install        # preview, then ask before executing package managers
dots deps install --yes  # execute installable actions without prompting
```

`dots deps install` executes package managers with direct argv only after confirmation. It does not bypass `sudo`, does not run manual-only guidance, and does not promise rollback, version constraints, reinstall, or upgrade behavior.

List Backup Metadata created by safe installs or restores:

```bash
dots backups list
```

When the plan looks right, run the install for real:

```bash
dots install
```

## Core concepts

| Concept | Meaning |
|---------|---------|
| Source of Truth | This repository's tracked dotfiles and manifest are the canonical desired state. |
| Install Manifest | The manifest that maps repository files to home-directory targets. |
| Managed Entry | A target file or link managed by `dots`, such as `.zshrc`, `.gitconfig`, Starship, or tmux config. |
| Install Plan | The preview of create, replace, skip, or conflict actions before install applies changes. |
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

The v1 scope focuses on the MVP Configuration Set: zsh, git, starship, and tmux. It includes guarded dependency inspection and installation for supported package-manager tiers, but does not include Windows, official WSL support, NixOS-specific behavior, Alpine/musl specialization, Neovim configuration, arbitrary manifest hooks, dependency rollback, version constraints, reinstall, or upgrade orchestration.

## Canonical docs

- [`CONTEXT.md`](CONTEXT.md) — domain vocabulary, architecture context, and project model.
- [`docs/scope.md`](docs/scope.md) — v1 scope, non-goals, and deferred work.
- [`docs/release.md`](docs/release.md) — release workflow, checksums, Homebrew, and Bootstrapper details.
- [`docs/adr/0001-bootstrap-with-go-cli.md`](docs/adr/0001-bootstrap-with-go-cli.md) — ADR for bootstrapping with the Go CLI.
