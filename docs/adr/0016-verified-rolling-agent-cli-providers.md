---
status: proposed
---

# Resolve fast-moving Agent CLIs with verified rolling providers

The Agent CLI Baseline will install missing Codex, Claude Code, OpenCode, Antigravity, and Copilot CLI capabilities through Rolling User-Local Providers. These closed, allowlisted providers resolve only the latest stable artifact from official metadata, require a published checksum or digest before installation, use the existing home-owned layout, and record the resolved release as Dependency Installation Metadata. A command already present on `PATH` satisfies its Dependency without network resolution, replacement, authentication, or update.

This decision partially supersedes ADR 0011 only for the separately named rolling category. Fixed User-Local Providers continue to declare versions and checksums in the Install Manifest. Rolling resolution moves that review boundary to allowlisted metadata sources, deterministic stable-release and platform-asset selection, fail-closed integrity verification, and a receipt that captures the version, URL, digest, platform, and installed path actually chosen. Dependency plans and dry runs may read official metadata to make that intent reviewable before mutation; missing or unverifiable metadata leaves a required Dependency unresolved and aborts installation before Managed Configuration changes.

**Considered Options**: Pinning all five tools in the Install Manifest was rejected because their daily release cadence would require continuous repository churn unrelated to dotfiles intent. A Homebrew/native-script hybrid was rejected because it cannot converge Linux machines without Homebrew uniformly, gives each tool a different installation and PATH boundary, and Antigravity has no official Homebrew package. Executing upstream `curl | sh` installers or resolving unverified latest artifacts was rejected because mutable remote code and inconsistent payload verification would bypass the reviewed provider boundary.

**Consequences**: Rolling resolution requires read-only network access when a selected command is absent, official-metadata adapters, deterministic stable/prerelease and platform filtering, safe archive handling, and structured preview fields for the resolved artifact. Vendor auto-update defaults remain outside dots' ownership after installation, so the receipt is audit evidence rather than an enforced current version. Providers do not mutate shell startup files; `~/.local/bin` remains an explicit PATH prerequisite.
