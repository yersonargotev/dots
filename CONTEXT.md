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

**Entry Ownership**:
The part of a Managed Entry's target that dots claims when evaluating Dotfiles Status. The default ownership is the whole installed file, but an entry may declare a narrower ownership mode when a supported tool legitimately co-owns the target.
_Avoid_: loose ownership, ignored drift, special case

**JSON Subset Ownership**:
An Entry Ownership mode for co-owned JSON targets where the repository source is the dots-owned baseline and the workstation target may contain additional object keys or array elements added by another supported owner. All scalar values, object keys, and array elements present in the baseline must still be present and equal in the target; otherwise the target is Drift when Installation Metadata proves dots installed it, or a Conflict when it does not.
_Avoid_: JSON merge, partial sync, tolerate anything

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
An external tool, application, package, or font required for a managed configuration to work correctly, such as Neovim, Ghostty, Starship, ripgrep, tmux, or a Nerd Font. The Dotfiles CLI declares and checks Dependencies; their presence is detected as an executable command on the search path, a declared macOS application bundle, or, for non-executable assets such as fonts, by the presence of their installed files.
_Avoid_: package, app, required tool

**Dependency Plan**:
The OS-aware installation guidance the Dotfiles CLI produces for missing Dependencies. It is the reviewable intent that `dots deps plan` previews and that `dots deps install` may execute.
_Avoid_: install script, package install, setup commands

**Package Manager Setup**:
An explicit, user-confirmed preparation step in `dots install` that installs or activates a supported package manager required to provision selected Dependencies. It is separate from Dependency installation and must not run in non-interactive Confirmed Install mode.
_Avoid_: bootstrap install, hidden package-manager install, provider magic

**Font Dependency**:
A Dependency that is a font rather than an executable, such as a Nerd Font required so Starship, tmux, or editor configuration renders its glyphs. Because a font is not on the search path, its presence is detected by scanning the workstation font directories for its installed files, not by a command lookup.
_Avoid_: typeface, glyph pack, icon font

**Supported Platform**:
An operating system and architecture combination the Dotfiles CLI intentionally supports. v1 supports macOS arm64/amd64 and Linux arm64/amd64 for configuration installation.
_Avoid_: environment, system, machine type

**Dependency Plan Tier**:
The level of OS-specific dependency guidance available for a Supported Platform. v1 provides specific guidance for macOS/Homebrew, Debian/Ubuntu, Fedora, and Arch, with a generic fallback for other Linux distributions.
_Avoid_: distro support, package support level

**User-Local Provider**:
A reviewed Dependency provider that installs a tool into the user's home-owned environment, such as `~/.local/bin` or `~/.local/opt/<tool>`, without mutating system package managers or requiring `sudo`. It is a first-class provider category with allowlisted tool behavior, not arbitrary shell execution.
_Avoid_: local script, custom install command, manual curl pipe

**Rolling User-Local Provider**:
A closed User-Local Provider recipe for a high-release-cadence tool. The Install Manifest selects only an allowlisted recipe; dots resolves the latest stable official release for the Supported Platform, requires an immutable artifact and official digest, and records the resolved evidence in Dependency Installation Metadata. A command already present on `PATH` satisfies the Dependency without resolution or replacement.
_Avoid_: latest installer script, mutable download, unpinned URL

**Install Plan**:
The preview of filesystem changes, conflicts, dependency findings, and backup requirements that the Dotfiles CLI computes before applying installation. It is shown during normal installation and is the output of dry-run mode.
_Avoid_: preview, dry run output, change list

**Uninstall Plan**:
The preview of removals the Dotfiles CLI computes from the Installation Metadata before reversing an install, classifying each recorded target as remove, skip, modified, or not-owned. It is the mirror of the Install Plan and is the output of `dots uninstall --dry-run`.
_Avoid_: removal preview, deletion list, reverse plan

**Dotfiles Status**:
The current alignment between a workstation and the repository-owned Source of Truth, including whether managed entries are installed, skipped, conflicting, or drifted.
_Avoid_: health, sync state, installed state

**Installation Metadata**:
The state file stored under `~/.local/state/dots/installed.json` that records what the Dotfiles CLI installed, including Managed Entries, Provisioners, strategies, hashes, timestamps, and an optional Installed Selection. It lets the CLI detect Drift for copied and templated targets while preserving machine-level selection intent separately from historical inventory.
_Avoid_: install log, state file, tracking file

**Installed Selection**:
The authoritative machine-level installation intent recorded only after terminal success of an explicit install, update, or upgrade, including an operator-confirmed Selection Migration Candidate. It preserves the ordered Profiles and explicit extra Tags selected by the operator, the ordered resolved Tag snapshot, Source of Truth provenance, and recording time. Profiles and explicit extra Tags are intent; resolved Tags are an audit snapshot. Per-Managed-Entry and per-Provisioner Profiles and Tags remain historical inventory and ownership evidence, not a substitute for an Installed Selection.
_Avoid_: inferred selection, installed profile, tag inventory

**Installed Selection Change**:
A complete explicit Profile/Tag request on a mutating command that differs from the authoritative Installed Selection. Its delta reports added and removed Profiles, explicit extra Tags, effective Tags, Managed Entries, Dependencies, and Provisioners before mutation. Removing a recorded Profile or explicit extra Tag is a reduction that requires distinct interactive confirmation or, in Confirmed Install mode, `--acknowledge-selection-change` in addition to `--yes`.
_Avoid_: implicit selection update, selection merge, automatic retirement

**Selection Migration Candidate**:
A non-authoritative selection proposed for Installation Metadata v1 or v2 from historical Managed Entry and Provisioner records plus current Install Manifest, target, and Source of Truth evidence. It reports ordered Profiles, explicit extra Tags, effective Tags, confidence, and ambiguity reasons. Only an unambiguous candidate can become an Installed Selection, and only after interactive operator confirmation and terminal success; ambiguous or absent evidence requires an explicit selection.
_Avoid_: inferred selection, migrated selection, implicit default

**Dependency Installation Metadata**:
The state record of Dependencies the Dotfiles CLI installed through executable providers, including the dependency name, provider, installed path, artifact version or checksum when applicable, and timestamp. It is separate from Installation Metadata because dependency tools are external workstation capabilities, not Managed Entries.
_Avoid_: package log, tool cache, install receipt

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

**Install Profile**:
A named selection in `dots.yaml` (such as `core`, `desktop`, or `workstation`) that resolves to a set of tags and decides which Managed Entries are installed on a given machine. Install Profiles are explicit and can be composed by repeating `--profile`; there is no repository-owned implicit `default` install baseline. It is about machine scope, not editor behavior.
_Avoid_: profile, machine profile, install set

**Project Name**:
The public repository name for the dotfiles project. The canonical repository name is `dotfiles`, while the command-line binary remains `dots`.
_Avoid_: repo name, product name, tool name

**Command Name**:
The executable name users run to manage installation, status, diagnostics, backups, and future updates. The canonical command name is `dots`.
_Avoid_: binary name, CLI name, app name

**Profile**:
A named installation selection that represents an intended workstation role, such as `core`, `desktop`, `agents`, `workstation`, `personal`, `work`, or `minimal`. Profiles select Managed Entries through tags rather than duplicating manifests per machine, and repeated Profiles compose by ordered tag union.
_Avoid_: machine config, host config, preset

**Tag**:
A label assigned to a Managed Entry so Profiles can include related configuration by intent, such as `core`, `agents`, `web`, `mobile`, or `desktop`. Tags from repeated Profiles and explicit `--tag` flags are de-duplicated while preserving order.
_Avoid_: category, group, label

**Core Development Baseline**:
The `core` Tag's intended workstation role: a general development environment that should be useful on any supported machine. It includes shell and terminal foundations plus common developer runtimes and package tools, rather than only minimal dotfile plumbing. GUI applications, agent-specific tooling, web/mobile-specialized tooling, secrets, and machine-specific state remain outside core unless explicitly selected by another Profile or Tag.
_Avoid_: minimal shell, everything profile, desktop baseline


**OS Filter**:
A manifest constraint that limits a Managed Entry to specific operating systems, such as `darwin` or `linux`. OS Filters complement Profiles but do not replace them.
_Avoid_: platform condition, system rule, distro flag

**Interactive Install**:
The normal installation mode used when a terminal is available. It shows an Install Plan, asks for confirmation, and uses the TUI or text prompts to resolve conflicts.
_Avoid_: normal install, guided install, TUI install

**Confirmed Install**:
A non-interactive installation mode requested with `--yes`. It applies safe changes without prompting and resolves conflicts with conservative defaults such as `skip`. It does not authorize an Installed Selection reduction; removing a recorded Profile or explicit extra Tag also requires `--acknowledge-selection-change`.
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
The package-manager installation path for the Dotfiles CLI through the `yersonargotev/tap/dots` Homebrew formula. It uses the same checksum-backed Release Artifacts as the Bootstrapper, so Homebrew is a distribution surface, not a second installer implementation.
_Avoid_: brew install, tap, package manager install

**MVP Configuration Set**:
The first group of Managed Configuration used to prove the installer end-to-end without migrating the entire workstation. The current set is the bounded `dots.yaml` manifest: shell, git, terminal, editor, and agent-tool config with explicit install strategies and dependencies.
_Avoid_: first dotfiles, initial configs, starter set

**Deferred Configuration**:
Managed Configuration intentionally postponed until the installer is proven with the MVP Configuration Set. Generated editor state, machine-local state, secrets, runtime caches, and non-portable application configuration remain Deferred Configuration because they can introduce plugin, runtime-state, and dependency complexity unrelated to installer correctness.
_Avoid_: later dotfiles, backlog config, skipped config

**Implementation Sequence**:
The ordered construction plan for the Dotfiles CLI, starting with the project skeleton, then manifest/platform logic, dry-run planning, filesystem installation, diagnostics, TUI conflict UX, and finally bootstrap/release distribution.
_Avoid_: roadmap, build order, task list

**Provisioner**:
An allowlisted external agent-configuration tool the Dotfiles CLI drives declaratively after Dependencies and Managed Entries are in place. dots versions only the invocation — the tool plus its declarative spec — and renders it into one exact, idempotent command (such as a `gentle-ai install`, a `gentle-ai uninstall`, a `claude plugin install`, or a `codex mcp add`). It never versions the Regenerated Content the tool owns. The allowlist is closed, so dots is never a generic command runner.
_Avoid_: hook, post-install script, command runner

**Provisioner Spec**:
The declarative values the Dotfiles CLI owns for a single Provisioner, which render into that tool's command. A spec speaks exactly one tool dialect at a time and carries no Regenerated Content.
_Avoid_: provisioner config, tool arguments

**Regenerated Content**:
The agent-tool state a Provisioner's tool owns and rewrites on its own — skills, personas, MCP-server entries, plugin registries, and machine-specific values such as absolute project paths or auth tokens (for example most of `~/.codex/config.toml` or `~/.claude.json`). dots keeps it out of the Source of Truth and reproduces it by re-running the invocation, never by versioning the tool's config file. Skills and personas are Regenerated Content owned by `gentle-ai`, not by `dots`.
_Avoid_: generated config, tool output, machine state

**Config Overlay**:
A dots-owned configuration fragment that a co-owned tool merges over its own config rather than replacing it, so dots adds its piece without versioning or clobbering the other owner's file. Used for the OpenCode `chrome-devtools` MCP server, merged via the `OPENCODE_CONFIG` environment variable over the gentle-ai-rendered `opencode.json`.
_Avoid_: include file, partial config, patch
