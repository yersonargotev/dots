# Bootstrap with a Go CLI

The project will expose a curl-compatible bootstrap command for convenience, but the bootstrapper will only detect the platform, download and verify the compiled Dotfiles CLI, and delegate installation to it. The installation logic will live in a Go CLI, with Cobra for command structure and Bubble Tea for interactive conflict resolution, because backup handling, OS-specific behavior, dry runs, and local extension points are too important to bury in an increasingly complex shell script.

**Considered Options**: A shell-only installer would be simpler to start but harder to test and maintain once conflict resolution, rollback, and TUI flows exist. A Go CLI has higher upfront cost but gives us a safer, testable installer while preserving the simple `curl | sh` entrypoint.

The installed repository will default to `~/.local/share/dots`, with explicit overrides supported for development checkouts or custom workstation layouts. Installation inputs will be declared in a `dots.yaml` manifest rather than hardcoded in Go, so managed files, targets, strategies, OS filters, and local extension points remain reviewable as repository data.

The initial manifest install strategies will be limited to `symlink`, `template`, and `copy`. Arbitrary shell hooks are intentionally excluded from v1 because they would turn the manifest from a declarative installation contract into an execution surface where any side effect is possible.

Before changing any existing target that is not already in the expected installed state, the Dotfiles CLI will create a timestamped Backup Set under `~/.local/state/dots/backups/` with metadata. This central backup location avoids scattering `.backup` files through the home directory and gives future commands such as `dots backups list` and `dots backups restore` a stable source of truth.

Conflict resolution will support `skip`, `replace`, `adopt`, and `diff`. Interactive installs will ask the user to resolve conflicts, while non-interactive installs default to `skip`; `replace` always creates a Backup Set first, and `adopt` must be explicit because it can contaminate repository-owned configuration with machine-specific content.

Secrets are excluded from the repository-owned Source of Truth. Shared configuration may reference ignored local files or external stores, but private keys, tokens, credential files, authenticated CLI state, and sensitive environment values must not be managed as shared dotfiles. The CLI should provide a Secret Scan guardrail through commands such as `dots doctor` or `dots check`, while making clear that pattern scanning is not a full security audit.

The v1 CLI will declare and verify external dependencies, then produce an OS-aware Dependency Plan for missing tools, but it will not install packages automatically. Automatic package installation is explicitly reserved for v2 because it introduces package-manager selection, sudo behavior, distro differences, repositories/taps, version constraints, and higher operational risk.

v1 officially supports macOS arm64/amd64 and Linux arm64/amd64 for configuration installation. Dependency Plans will provide specific package-manager guidance for macOS/Homebrew, Debian/Ubuntu, Fedora, and Arch, with a generic fallback for other Linux distributions; Windows, WSL as an official target, NixOS-specific behavior, Alpine/musl specialization, and automatic package installation are out of scope for v1.

The first-install flow will keep the bootstrapper minimal, install the `dots` binary under `~/.local/bin/dots`, then run `dots install`. The CLI will compute and show an Install Plan before applying changes, support `dots install --dry-run` from v1, and include the v1 commands `install`, `status`, `doctor`, `deps check`, `deps plan`, and `backups list`; `diff`, `backups restore`, and `deps install` are optional or future work.

The v1 CLI will store Installation Metadata under `~/.local/state/dots/installed.json` so `dots status` can detect basic Drift, especially for `copy` and `template` strategies where symlink inspection is insufficient. `dots update` is documented for v1.1 rather than required for the first MVP because repository updates introduce Git state, local changes, versioning, and post-update conflict handling.

The repository will use a Go-oriented CLI layout (`cmd/dots`, `internal/*`) alongside repository-owned configuration (`configs/`, `templates/`), a minimal bootstrapper (`scripts/install.sh`), `dots.yaml`, `CONTEXT.md`, `README.md`, and ADRs under `docs/adr/`. This separates installer implementation from Managed Configuration and keeps the Install Manifest as the contract between them.

The public repository should be named `dotfiles`, while the executable, manifest, install directory, and state directory keep the shorter `dots` naming: `dots`, `dots.yaml`, `~/.local/share/dots`, and `~/.local/state/dots`. This gives the repository a clear human-readable purpose while keeping daily commands short.

The Install Manifest will support Profiles, Tags, and OS Filters in v1. Profiles such as `default`, `personal`, `work`, and `minimal` select Managed Entries by intent through Tags, while OS Filters constrain entries to macOS or Linux where needed. Hostname-specific behavior is intentionally not the primary mechanism because it encourages every machine to become a one-off exception.

`dots install` will use an Interactive Install when a TTY is available, showing the Install Plan and using the TUI or text prompts for conflicts. `--yes` selects a Confirmed Install that skips prompts and keeps conservative conflict defaults, while `--no-tui` selects Text Prompt Mode. Global conflict policy may allow `skip` or `replace`, but `adopt` is intentionally not allowed as a global conflict action.

The v1 implementation will follow Strict TDD and include unit tests for pure logic, Temporary Home Tests for filesystem installation behavior, Golden Output Tests for important CLI output, and focused TUI Model Tests for Bubble Tea state transitions. Full visual TUI snapshot testing is out of scope for v1 because installer correctness matters more than pixel-perfect terminal rendering.

Distribution starts with GitHub Releases in v1, publishing Release Artifacts and checksums for macOS amd64/arm64 and Linux amd64/arm64. The Bootstrapper must perform Checksum Verification before installing or executing the downloaded CLI. Homebrew Distribution uses the same checksum-backed artifacts through `yersonargotev/tap/dots`, so the tap remains a distribution surface rather than a separate install implementation. Initial versioning uses `v0.x`.

The MVP Configuration Set is the bounded `dots.yaml` manifest: shell, git, terminal, editor, and agent-tool config with explicit install strategies and dependencies. Generated editor state, machine-local state, secrets, runtime caches, and non-portable application configuration remain Deferred Configuration because they distract from proving installer, backups, manifest selection, status, and conflict handling.

Implementation will proceed in this order: project skeleton, manifest/platform logic, Install Plan and dry-run behavior, filesystem installation with backups and metadata, status/doctor/dependency/secret checks, TUI conflict UX, and finally bootstrapper plus release automation. The TUI intentionally comes after installer correctness so user-interface work does not distract from safe filesystem behavior.
