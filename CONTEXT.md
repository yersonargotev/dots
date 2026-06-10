# Dotfiles Distribution

This context defines the language for a personal dotfiles project that keeps workstation configuration consistent across macOS and Linux machines.

## Language

**Source of Truth**:
The repository-owned configuration that represents the canonical desired state for shared dotfiles across machines. Local machine-specific configuration may coexist only through explicit extension points, not by silently diverging from the repository.
_Avoid_: master config, main config, real config

**Machine-Specific Configuration**:
Configuration intentionally scoped to one machine, host, operating system, or private environment and therefore not part of the shared canonical dotfiles. It must be isolated from shared configuration through approved include files, overlays, or ignored local files.
_Avoid_: random local changes, personal hacks, one-off edits

**Local Extension Point**:
An intentional place where a machine may add private or host-specific behavior without modifying the shared configuration. Examples include local include files, OS overlays, hostname overlays, and ignored private files.
_Avoid_: manual patch, hidden override, workaround

**Conflict**:
A destination file or symlink that would be changed by installation and does not already match the repository-managed target. Conflicts require an explicit resolution strategy before replacement.
_Avoid_: mismatch, dirty file

**Bootstrapper**:
A minimal installation script whose only responsibility is to download, verify, and launch the dotfiles CLI for the current platform. It is not the primary installer and must not contain the full installation logic.
_Avoid_: install script, bash installer, curl script

**Dotfiles CLI**:
The compiled command-line application that owns installation, backup, conflict resolution, diagnostics, and interactive flows for the dotfiles project.
_Avoid_: installer script, setup script

**Installed Repository**:
The local clone or checkout of the dotfiles repository used by the Dotfiles CLI as the source for managed configuration. Its default location is `~/.local/share/dots`, but callers may override it explicitly for development or custom workstation layouts.
_Avoid_: working copy, install folder, dotfiles folder

**Install Manifest**:
The repository file, named `dots.yaml`, that declares which configuration entries the Dotfiles CLI manages and how each one should be installed. It is the reviewable contract between the repository source of truth and workstation filesystem changes.
_Avoid_: config list, installer config, file map

**Install Strategy**:
The declared mechanism the Dotfiles CLI uses to place a managed configuration entry at its target. Initial strategies are `symlink`, `template`, and `copy`; arbitrary shell execution is not part of the initial contract.
_Avoid_: install mode, file operation, action

**Managed Entry**:
A single configuration item declared in the Install Manifest, including its source, target, install strategy, filters, and local extension points.
_Avoid_: dotfile item, config row, install target

**Backup Set**:
A timestamped collection of files preserved before an installation changes existing workstation targets. Backup sets live under `~/.local/state/dots/backups/` and include metadata describing what was protected and why.
_Avoid_: old files, backup folder, snapshot

**Backup Metadata**:
The machine-readable record stored with each Backup Set that describes when it was created, which repository and machine created it, which targets were backed up, and the reason each backup was taken.
_Avoid_: log file, notes, restore info

**Conflict Resolution**:
The explicit action chosen when a managed target conflicts with the expected installed state. Supported initial actions are `skip`, `replace`, `adopt`, and `diff`; non-interactive installation defaults to `skip`.
_Avoid_: overwrite behavior, fix, merge

**Adoption**:
A conflict resolution action that turns an existing local target into repository-owned source configuration. Adoption is a migration tool and must be explicit because it can contaminate the Source of Truth with machine-specific content.
_Avoid_: import, copy into repo, take over

**Secret**:
Any credential, token, private key, sensitive environment value, or authenticated CLI state that must not be stored in the repository-owned Source of Truth. Secrets may only live in ignored local files or external secret stores.
_Avoid_: private config, credential config, sensitive dotfile

**Secret Scan**:
A safety check that looks for known credential and private-key patterns in repository-managed configuration before installation or publication. It is a guardrail, not proof that the repository is safe.
_Avoid_: security audit, secret validation, token check

**Dependency**:
An external tool or package required for a managed configuration to work correctly, such as Neovim, Starship, ripgrep, or tmux. Dependencies are declared and checked by the Dotfiles CLI but are not automatically installed in v1.
_Avoid_: package, app, required tool

**Dependency Plan**:
The OS-aware installation guidance produced by the Dotfiles CLI for missing dependencies. In v1 it is advisory only; automatic dependency installation is reserved for a later version.
_Avoid_: install script, package install, setup commands

**Supported Platform**:
An operating system and architecture combination the Dotfiles CLI intentionally supports. v1 supports macOS arm64/amd64 and Linux arm64/amd64 for configuration installation.
_Avoid_: environment, system, machine type

**Dependency Plan Tier**:
The level of OS-specific dependency guidance available for a Supported Platform. v1 provides specific guidance for macOS/Homebrew, Debian/Ubuntu, Fedora, and Arch, with a generic fallback for other Linux distributions.
_Avoid_: distro support, package support level

**Install Plan**:
The preview of filesystem changes, conflicts, dependency findings, and backup requirements that the Dotfiles CLI computes before applying installation. It is shown during normal installation and is the output of dry-run mode.
_Avoid_: preview, dry run output, change list

**Dotfiles Status**:
The current alignment between a workstation and the repository-owned Source of Truth, including whether managed entries are installed, skipped, conflicting, or drifted.
_Avoid_: health, sync state, installed state

**Installation Metadata**:
The state file stored under `~/.local/state/dots/installed.json` that records what the Dotfiles CLI installed, including managed entries, strategies, hashes, and timestamps. It lets the CLI detect drift for copied and templated targets.
_Avoid_: install log, state file, tracking file

**Drift**:
A workstation state where a managed target no longer matches the repository-owned Source of Truth or the Installation Metadata recorded when it was installed. Drift is detected by `dots status` and is distinct from an initial Conflict.
_Avoid_: local change, mismatch, dirty config

**Dotfiles Update**:
The `dots update` workflow that fast-forwards the Installed Repository to its upstream and re-runs the safe install flow so managed configuration stays aligned with the Source of Truth. It refuses to touch a repository with local changes and only ever applies a clean fast-forward, never a merge or rebase. Shipped in v1.1.
_Avoid_: sync, pull, upgrade

**Repository Layout**:
The top-level organization of the project into CLI source code, managed configuration, templates, scripts, documentation, and the Install Manifest. It keeps installer code separate from repository-owned configuration.
_Avoid_: folder structure, project tree, repo organization

**Managed Configuration**:
The repository-owned files, directories, templates, or assets that become workstation configuration through the Install Manifest. Managed Configuration lives under `configs/` or `templates/` rather than being mixed with CLI implementation code.
_Avoid_: dotfiles, config files, installed files

**Portable Terminal Preference**:
A terminal preference that can follow the user across machines without encoding host-specific ergonomics, such as theme, intentional keybindings, cursor behavior, scrollback, copy/paste behavior, or close confirmations.
_Avoid_: terminal setup, my terminal config, machine terminal preference

**Project Name**:
The public repository name for the dotfiles project. The canonical repository name is `dotfiles`, while the command-line binary remains `dots`.
_Avoid_: repo name, product name, tool name

**Command Name**:
The executable name users run to manage installation, status, diagnostics, backups, and future updates. The canonical command name is `dots`.
_Avoid_: binary name, CLI name, app name

**Profile**:
A named installation selection that represents an intended workstation role, such as `default`, `personal`, `work`, or `minimal`. Profiles select Managed Entries through tags rather than duplicating manifests per machine.
_Avoid_: machine config, host config, preset

**Tag**:
A label assigned to a Managed Entry so Profiles can include related configuration by intent, such as `core`, `dev`, `personal`, `work`, or `desktop`.
_Avoid_: category, group, label

**OS Filter**:
A manifest constraint that limits a Managed Entry to specific operating systems, such as `darwin` or `linux`. OS Filters complement Profiles but do not replace them.
_Avoid_: platform condition, system rule, distro flag

**Interactive Install**:
The normal installation mode used when a terminal is available. It shows an Install Plan, asks for confirmation, and uses the TUI or text prompts to resolve conflicts.
_Avoid_: normal install, guided install, TUI install

**Confirmed Install**:
A non-interactive installation mode requested with `--yes`. It applies safe changes without prompting and resolves conflicts with conservative defaults such as `skip`.
_Avoid_: auto install, force install, unattended install

**Text Prompt Mode**:
A non-TUI interactive mode requested with `--no-tui` for terminals where the TUI is undesirable or unsupported. It preserves explicit conflict resolution without using Bubble Tea.
_Avoid_: basic mode, fallback mode, simple prompts

**Temporary Home Test**:
An integration test that runs installer behavior against an isolated temporary HOME, repository, and state directory so real workstation files are never touched.
_Avoid_: integration test, fake home, temp test

**Golden Output Test**:
A test that compares stable command output, such as status, dependency plans, or dry-run install plans, against approved fixture files.
_Avoid_: snapshot test, output test, CLI fixture

**TUI Model Test**:
A focused test of terminal UI state transitions and decisions through the Bubble Tea model/update layer, without requiring full visual snapshot coverage.
_Avoid_: TUI snapshot, screen test, visual test

**Release Artifact**:
A platform-specific Dotfiles CLI binary and its associated checksum published through GitHub Releases. Release Artifacts are what the Bootstrapper downloads and verifies.
_Avoid_: binary, build, download

**Checksum Verification**:
The bootstrap safety step that confirms a downloaded Release Artifact matches the checksum published for that release before installing or executing it.
_Avoid_: validation, hash check, download check

**Homebrew Distribution**:
The later package-manager installation path for the Dotfiles CLI through a Homebrew tap. It is planned for v1.1 or phase 2 after GitHub Releases and checksum-based bootstrapping are working.
_Avoid_: brew install, tap, package manager install

**MVP Configuration Set**:
The first group of Managed Configuration used to prove the installer end-to-end without migrating the entire workstation. The v1 MVP Configuration Set is zsh, git, starship, and tmux.
_Avoid_: first dotfiles, initial configs, starter set

**Deferred Configuration**:
Managed Configuration intentionally postponed until the installer is proven with the MVP Configuration Set. Neovim and larger app configurations are Deferred Configuration because they can introduce plugin and dependency complexity unrelated to installer correctness.
_Avoid_: later dotfiles, backlog config, skipped config

**Implementation Sequence**:
The ordered construction plan for the Dotfiles CLI, starting with the project skeleton, then manifest/platform logic, dry-run planning, filesystem installation, diagnostics, TUI conflict UX, and finally bootstrap/release distribution.
_Avoid_: roadmap, build order, task list
