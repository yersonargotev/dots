# `dots update`

`dots update` refreshes the [Installed Repository](../CONTEXT.md#installed-repository) (default `~/.local/share/dots`) and re-runs the safe install flow so managed configuration stays aligned with the [Source of Truth](../CONTEXT.md). It reuses the existing Install Plan, Conflict Resolution, and Backup Set machinery rather than reimplementing filesystem logic — the only behavior unique to `update` is advancing the repository's Git state.

## What it does

1. **Validates Git state.** The source root must be a Git work tree. If it is not, `update` stops with an error instead of guessing.
2. **Preserves local Installed Repository changes.** If the repository has uncommitted changes (modified, staged, or untracked files), `update` moves them into Git's stash before continuing. This keeps customer updates self-healing while preserving local edits for inspection instead of discarding them.
3. **Fast-forwards only.** `update` fetches the upstream and advances the branch with `git merge --ff-only`. If the branch has diverged from its upstream, it cannot be fast-forwarded; `update` reports the divergence and asks you to resolve it manually with Git. It never performs an automatic merge or rebase.
4. **Recomputes the Install Plan.** After the fast-forward, the manifest is loaded from the updated repository (so a manifest change pulled from upstream is honored) and a fresh Install Plan is computed against the current workstation state, surfacing any new Conflicts or Drift.
5. **Applies safely.** The post-update install resolves conflicts exactly like `dots install`: interactive TUI by default, text prompts with `--no-tui`, or the conservative skip default with `--yes`. Any `replace` still creates a [Backup Set](../CONTEXT.md) before touching an existing target.
6. **Runs provisioners and convergence.** After the file plan is applied, `update` runs the same selected Provisioners as `install`, then converges marker-owned instruction cleanup for the native Agent CLI Baseline. Provisioners run only when file application was not canceled; `--dry-run` renders the plan without executing it. If Conflict Resolution is canceled, the whole run aborts before any tool-managed configuration changes.

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
Source of Truth, the Install Plan may report an `update` action. Strict-JSON
subset targets use the prior contribution recorded in Installation Metadata to
add new values and remove unchanged retired values while preserving target-only
content; an externally changed former value remains a Conflict. Legacy metadata
without that evidence stays additive-only until a safe run records the current
baseline. TOML subset updates remain additive. Every `update` creates a Backup
Set before mutation, and unmanaged or incompatible targets remain Conflicts.

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

Provisioners are scoped by the same resolved tags as file entries. There is no implicit default Profile; reuse a recorded Installed Selection, choose `core`, compose pure capability Profiles such as `--profile agents --profile web`, or use `workstation` for `core + desktop + agents`. The `desktop` Profile selects desktop configuration and non-web desktop integrations. The `agents` Profile is the native Agent CLI Baseline: Codex, Claude Code, OpenCode, Antigravity, Copilot CLI, shared `jq`, and their dots-owned native Managed Configuration. It does not select gentle-ai, Engram, Context7, generated permissions, SDD/persona operations, third-party engineering skills, delegation, or dots-owned global agent rules. `core` owns GitHub CLI and also selects `jq`. CodeGraph remains an explicit Tag, while `codex-delegation`, `web`, and `mobile` retain independent intent. The `web` Profile composes its Chrome DevTools overlay over OpenCode's native JSON baseline without replacing it. Use `workstation` when you explicitly want core, desktop integrations, and the Agent CLI Baseline together; web and mobile remain separate opt-ins.

Use `--profile codex-delegation` when only Codex delegation is desired. It installs the dots-owned `delegation` skill for Codex, the model-neutral `<!-- dots:delegation -->` overlay, and native `dots-explorer`/`dots-worker` agents without the broader agent baseline. Remove the overlay and native agents with `--profile codex-delegation --tag without-codex-delegation`. The legacy Spark-named install and cleanup tags remain compatibility aliases. See [`docs/agents/delegation.md`](agents/delegation.md) for the portable task-fit delegation policy and the current inventory of native delegation artifacts by supported agent surface.

During update or upgrade, selection evolution reports gentle-ai and Engram
Dependencies plus their gentle-ai/skills Provisioners as removed surfaces. This
report is informational: dots does not uninstall binaries, delete directories,
remove Dependency Installation Metadata, or erase historical Provisioner
receipts. After Managed Configuration succeeds, dots removes only known
marker-delimited gentle-ai trigger/persona/Engram blocks and the dots-owned
global-rules block from supported instruction files. Unmarked and co-owned
content, complete files, Codex delegation, authentication, and Secret state are
preserved.

To remove residual gentle-ai state, first review it outside dots (for example
`~/.gentle-ai`, agent instruction files, generated skills, and any external
gentle-ai/Engram installation). Use the vendor's explicit uninstall flow or
remove reviewed residual paths manually only after confirming their ownership.
Do not delete entire shared agent configuration directories, authentication
files, or historical dots receipts. The retired implementation remains under a
non-Profile manifest tag only until #371 removes it; production Profiles never
select that tag. OpenCode's `web` Managed Entry composes its MCP subset directly
into the native `~/.config/opencode/opencode.json`, so
`--profile agents --profile web` does not depend on `core` shell configuration.

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
