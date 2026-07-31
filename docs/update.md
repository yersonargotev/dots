# `dots update`

`dots update` refreshes the [Installed Repository](../CONTEXT.md#installed-repository) (default `~/.local/share/dots`) and re-runs the safe install flow so managed configuration stays aligned with the [Source of Truth](../CONTEXT.md). It reuses the existing Install Plan, Conflict Resolution, and Backup Set machinery rather than reimplementing filesystem logic — the only behavior unique to `update` is advancing the repository's Git state.

## What it does

1. **Validates Git state.** The source root must be a Git work tree. If it is not, `update` stops with an error instead of guessing.
2. **Preserves local Installed Repository changes.** If the repository has uncommitted changes (modified, staged, or untracked files), `update` moves them into Git's stash before continuing. This keeps customer updates self-healing while preserving local edits for inspection instead of discarding them.
3. **Fast-forwards only.** `update` fetches the upstream and advances the branch with `git merge --ff-only`. If the branch has diverged from its upstream, it cannot be fast-forwarded; `update` reports the divergence and asks you to resolve it manually with Git. It never performs an automatic merge or rebase.
4. **Recomputes the Install Plan.** After the fast-forward, the manifest is loaded from the updated repository (so a manifest change pulled from upstream is honored) and a fresh Install Plan is computed against the current workstation state, surfacing any new Conflicts or Drift.
5. **Applies safely.** The post-update install resolves conflicts exactly like `dots install`: interactive TUI by default, text prompts with `--no-tui`, or the conservative skip default with `--yes`. Any `replace` still creates a [Backup Set](../CONTEXT.md) before touching an existing target.
6. **Runs provisioners.** After the file plan is applied, `update` runs the same provisioners `install` would for the active profile, so provisioner-managed agent configuration (gentle-ai cleanup/install, Claude plugins, Claude MCP servers, Codex MCP servers) stays aligned with the Source of Truth. Provisioners run in manifest order, which allows a cleanup command to run before an install command. Provisioners run only when the file apply was not canceled; a `--dry-run` renders the Provisioners plan section without executing anything. If conflict resolution is canceled, the whole run aborts before any provisioner can mutate tool-managed config.

## Versioning model

`update` is intentionally a thin layer over Git. The "version" of your managed configuration is the Git revision of the Installed Repository — `update` reports the short revision it moved from and to, plus the one-line summaries of the commits it applied:

```
Updated Installed Repository a1b2c3d -> e4f5a6b:
  e4f5a6b add tmux config
```

Because the update path is fast-forward only, the local revision is always a strict ancestor of the new revision. This keeps the model auditable (you can always inspect the exact commits applied) and avoids dots ever rewriting history or fabricating merge commits. If local Installed Repository changes were present, `update` reports the stash reference that preserved them, for example:

```
Preserved local Installed Repository changes in stash@{0}.
```

To inspect those preserved edits, use Git directly in the Installed Repository. To roll back, use Git directly in the Installed Repository.

## Post-update conflict handling

A fast-forward changes the Source of Truth, so targets that were previously aligned can become Conflicts or Drift after an update. `update` surfaces these in the recomputed Install Plan and resolves them with the same rules as `install`:

| Decision | Effect | Backup Set |
|----------|--------|------------|
| `skip` (default when non-interactive) | Leaves the existing target untouched | — |
| `replace` | Replaces the target with the updated Source of Truth | Created first |
| `adopt` | Copies the local target into the Source of Truth | Created for symlink strategy |

Non-interactive runs (`--yes`) default every conflict to `skip`, so an unattended `update` never silently overwrites a workstation file.

Some co-owned copied configuration can be updated without treating the target as
a Conflict. When Installation Metadata proves dots installed the previous
Source of Truth and the target still contains that dots-owned subset, the Install
Plan may report an `update` action. Today this is used for TOML subset-owned
Codex config migrations such as adding the CodeGraph `SessionStart` hook while
preserving Codex- or user-owned settings. An `update` creates a Backup Set before
mutating the target, and it is safe for Confirmed Install mode because unmanaged
or incompatible targets still remain Conflicts.

## Flags

`update` accepts the same path, selection, and safety flags as `install`:

| Flag | Purpose |
|------|---------|
| `--dry-run` | Fetch and report the available fast-forward and the Install Plan without fast-forwarding the working tree or installing files. |
| `--file`, `--profile`, `--tag` | Select the manifest, one or more Profiles, and optional capability tags to install after updating. Repeat `--profile` to compose Profiles and repeat `--tag` for opt-in tags. If neither selection flag is present, reuse the Installed Selection. |
| `--source-root` | Installed Repository to update (default `~/.local/share/dots`). |
| `--home` | Target home directory; use a sandbox path to avoid touching real config. |
| `--state-root` | State directory for Installation Metadata and Backup Sets. |
| `--yes` | Apply safe actions without prompting; conflicts default to `skip`. |
| `--acknowledge-selection-change` | With `--yes`, acknowledge removal of recorded Profiles or explicit extra Tags after reviewing the Installed Selection Change. |
| `--no-tui` | Use text prompts instead of the interactive TUI for conflict resolution. |

## Dry run

`dots update --profile workstation --dry-run` fetches the upstream and reports the commits a fast-forward would apply, then renders the current Install Plan, without modifying the repository working tree or installing anything:

```
Installed Repository can fast-forward a1b2c3d -> e4f5a6b:
  e4f5a6b add tmux config

(dry run: working tree and managed files not modified)
```

Because no fast-forward is applied in a dry run, the rendered plan reflects the **current** Source of Truth, not the post-update state. Run `update` without `--dry-run` to apply the fast-forward and compute the plan against the updated repository.

After a successful explicit install, an unattended update can reuse the
Installed Selection:

```bash
dots update --yes
```

For Installation Metadata v1/v2 without an Installed Selection, update first
builds a non-authoritative Selection Migration Candidate from historical
Managed Entry and Provisioner records plus the current manifest, target, and
Source of Truth evidence. An interactive update may confirm only an
unambiguous candidate. Ambiguous or absent evidence requires the complete
selection to be repeated with `--profile` and `--tag`. `--yes`, JSON output,
and other non-interactive runs return `selection-migration-required` before
mutation, with a recommended explicit command when one can be constructed.
There is no implicit default.

Any supplied `--profile` or `--tag` makes the complete explicit selection win
for that invocation; dots never merges it with recorded intent. Before the
Installed Repository changes, update reports the requested selection delta from
the Installed Selection, including added and removed Profiles, explicit extra
Tags, effective Tags, Managed Entries, Dependencies, and Provisioners. Removing
a recorded Profile or explicit extra Tag requires a distinct interactive
confirmation. Non-interactive execution requires both `--yes` and
`--acknowledge-selection-change`; `--yes` alone returns
`selection-change-acknowledgement-required` before mutation. The selection is
then validated and resolved again against
the refreshed manifest before Managed Configuration is applied. Update reports
the resulting effective Tag and selected Managed Entry, Dependency, and
Provisioner additions and removals before application. A missing saved Profile
or explicit extra Tag no longer declared by any selectable manifest surface
stops application with remediation and structured delta data instead of
choosing an implicit default or silently rewriting intent. Removed surfaces are
informational and are never automatically deleted or uninstalled. Only terminal
success records or refreshes the Installed Selection, so dry runs, declined
selection-change or migration confirmation, cancellations, and
failed Managed Entry or Provisioner work preserve the previous intent.

## Profiles and provisioners

Provisioners are scoped by the same resolved tags as file entries. There is no implicit default Profile; reuse a recorded Installed Selection, choose `core`, compose pure capability Profiles such as `--profile agents --profile web`, or use `workstation` for `core + desktop + agents`. The `desktop` profile selects desktop configuration and non-web desktop integrations. The `agents` profile selects gentle-ai memory/context setup and cleanup provisioners for Codex, Claude Code, OpenCode, Antigravity, and VS Code Copilot, plus agent settings baselines, shared engineering skills, the dots-owned `delegation` skill, and compact dots-owned global rules for supported agents. Codex-only delegation is available through `--profile codex-delegation`, which installs the dots-owned `delegation` skill only for Codex and the model-neutral `<!-- dots:delegation -->` overlay; that overlay is removable with `--tag without-codex-delegation`. gentle-ai persona prompt Regenerated Content is cleaned up rather than installed by this repository; SDD remains optional with `--tag sdd`. CodeGraph is independent and selected with `--tag codegraph`; CodeGraph's installer owns generated MCP/instruction setup, while dots owns the scoped `<!-- dots:codegraph-mode -->` routing and verification policy overlay plus a Codex SessionStart hook that runs `codegraph init` only when the current Git repository lacks `.codegraph`. The `web` profile selects frontend design skills plus Chrome DevTools for Claude, Codex, and the OpenCode overlay. The `mobile` profile selects Dart and Flutter agent skills plus the Dart and Flutter MCP server for Claude, Codex, Antigravity, and GitHub Copilot in VS Code. Use `workstation` when you explicitly want both desktop integrations and agent setup; web and mobile tooling remain separate opt-ins. This is design-intent: desktop installs should configure desktop tools, not opt into SDD, gentle-dev agent setup, browser/frontend tooling, or mobile-specific skills.

Use `--profile codex-delegation` when only Codex delegation is desired. It installs the dots-owned `delegation` skill for Codex and the model-neutral `<!-- dots:delegation -->` overlay without the broader agent baseline. Remove the overlay with `--profile codex-delegation --tag without-codex-delegation`. The legacy Spark-named install and cleanup tags remain compatibility aliases. See [`docs/agents/delegation.md`](agents/delegation.md) for the portable task-fit delegation policy.

The gentle-ai provisioner can model cleanup before install by using separate entries in manifest order. Missing `action` still defaults to `install`; `yes` is only valid for `uninstall` because it confirms a cleanup action. Install/update plan output also calls out provisioners that may install or update user-local global tools, such as Claude Code through the npm prefix under `~/.local`:

The `gentle-ai` and `engram` executables themselves are still Dependencies, not
Provisioner side effects. On Linux their reviewed User-Local Providers install
pinned release tarballs only when the executable probe is missing; the later
Provisioner step only runs after those probes pass.

```yaml
provisioners:
  - tool: gentle-ai
    tags: [agents]
    spec:
      action: uninstall
      agents: [codex, claude-code, opencode, antigravity, vscode-copilot]
      components: [sdd, persona]
      yes: true
  - tool: gentle-ai
    tags: [agents]
    spec:
      preset: custom
      agents: [claude-code]
      components: [engram, context7, permissions]
```

After installs or updates, use `dots installed` to inspect the read-only
Installation Metadata inventory: the authoritative Installed Selection, any
non-authoritative Selection Migration Candidate, and historical recorded
Managed Entries, represented Tags, Profile coverage, Provisioner runs, and
captured Source of Truth provenance. These sections remain separate; inspecting
a candidate never records it or deletes legacy ownership records.

To keep that requirement discoverable, both `install` and `update` print a one-line hint when the active profile skips provisioners another profile would select on this OS:

```
Note: profile "core" skips provisioner(s); run with --profile core --profile agents to keep core and add agent setup.
```

File entries are profile-scoped the same way, and `core` intentionally omits profile-specific entries such as the `desktop` Ghostty/Zed configs and the `web` OpenCode MCP overlay. To close the same discoverability gap, both commands print a parallel hint for skipped file entries:

```
Note: profile "core" skips file entries; run with --profile core --profile desktop to keep core and add desktop entries.
```

Both hints also appear in `--dry-run`, so you can see what a profile omits before committing to it. The fuller profile that already selects every entry and provisioner prints no hint.

## References

- [`docs/scope.md`](scope.md) — `dots update` was deferred from v1 and delivered in v1.1.
- [`CONTEXT.md`](../CONTEXT.md) — vocabulary for Installed Repository, Source of Truth, Conflict, Drift, and Backup Set.
- [`docs/adr/0001-bootstrap-with-go-cli.md`](adr/0001-bootstrap-with-go-cli.md) — the Go CLI and thin-Git-wrapper tradeoffs.
