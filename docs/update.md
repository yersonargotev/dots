# `dots update`

`dots update` refreshes the [Installed Repository](../CONTEXT.md#installed-repository) (default `~/.local/share/dots`) and re-runs the safe install flow so managed configuration stays aligned with the [Source of Truth](../CONTEXT.md). It reuses the existing Install Plan, Conflict Resolution, and Backup Set machinery rather than reimplementing filesystem logic — the only behavior unique to `update` is advancing the repository's Git state.

## What it does

1. **Validates Git state.** The source root must be a Git work tree. If it is not, `update` stops with an error instead of guessing.
2. **Refuses to overwrite local work.** If the repository has any uncommitted changes (modified, staged, or untracked files), `update` reports the dirty state and stops. It never stashes, resets, or discards local changes on your behalf.
3. **Fast-forwards only.** `update` fetches the upstream and advances the branch with `git merge --ff-only`. If the branch has diverged from its upstream, it cannot be fast-forwarded; `update` reports the divergence and asks you to resolve it manually with Git. It never performs an automatic merge or rebase.
4. **Recomputes the Install Plan.** After the fast-forward, the manifest is loaded from the updated repository (so a manifest change pulled from upstream is honored) and a fresh Install Plan is computed against the current workstation state, surfacing any new Conflicts or Drift.
5. **Applies safely.** The post-update install resolves conflicts exactly like `dots install`: interactive TUI by default, text prompts with `--no-tui`, or the conservative skip default with `--yes`. Any `replace` still creates a [Backup Set](../CONTEXT.md) before touching an existing target.
6. **Runs provisioners.** After the file plan is applied, `update` runs the same provisioners `install` would for the active profile, so provisioner-managed agent configuration (gentle-ai cleanup/install, Claude plugins, Codex MCP server) stays aligned with the Source of Truth. Provisioners run in manifest order, which allows a cleanup command to run before an install command. Provisioners run only when the file apply was not canceled; a `--dry-run` renders the Provisioners plan section without executing anything. If conflict resolution is canceled, the whole run aborts before any provisioner can mutate tool-managed config.

## Versioning model

`update` is intentionally a thin layer over Git. The "version" of your managed configuration is the Git revision of the Installed Repository — `update` reports the short revision it moved from and to, plus the one-line summaries of the commits it applied:

```
Updated Installed Repository a1b2c3d -> e4f5a6b:
  e4f5a6b add tmux config
```

Because the update path is fast-forward only, the local revision is always a strict ancestor of the new revision. This keeps the model auditable (you can always inspect the exact commits applied) and avoids dots ever rewriting history or fabricating merge commits. To roll back, use Git directly in the Installed Repository.

## Post-update conflict handling

A fast-forward changes the Source of Truth, so targets that were previously aligned can become Conflicts or Drift after an update. `update` surfaces these in the recomputed Install Plan and resolves them with the same rules as `install`:

| Decision | Effect | Backup Set |
|----------|--------|------------|
| `skip` (default when non-interactive) | Leaves the existing target untouched | — |
| `replace` | Replaces the target with the updated Source of Truth | Created first |
| `adopt` | Copies the local target into the Source of Truth | Created for symlink strategy |

Non-interactive runs (`--yes`) default every conflict to `skip`, so an unattended `update` never silently overwrites a workstation file.

## Flags

`update` accepts the same path, profile, and safety flags as `install`:

| Flag | Purpose |
|------|---------|
| `--dry-run` | Fetch and report the available fast-forward and the Install Plan without fast-forwarding the working tree or installing files. |
| `--file`, `--profile` | Select the manifest and profile to install after updating. |
| `--source-root` | Installed Repository to update (default `~/.local/share/dots`). |
| `--home` | Target home directory; use a sandbox path to avoid touching real config. |
| `--state-root` | State directory for Installation Metadata and Backup Sets. |
| `--yes` | Apply safe actions without prompting; conflicts default to `skip`. |
| `--no-tui` | Use text prompts instead of the interactive TUI for conflict resolution. |

## Dry run

`dots update --dry-run` fetches the upstream and reports the commits a fast-forward would apply, then renders the current Install Plan, without modifying the repository working tree or installing anything:

```
Installed Repository can fast-forward a1b2c3d -> e4f5a6b:
  e4f5a6b add tmux config

(dry run: working tree and managed files not modified)
```

Because no fast-forward is applied in a dry run, the rendered plan reflects the **current** Source of Truth, not the post-update state. Run `update` without `--dry-run` to apply the fast-forward and compute the plan against the updated repository.

## Profiles and provisioners

Provisioners are scoped by profile tags, exactly like file entries. The `default` profile selects only `core`-tagged provisioners (gentle-ai); the chrome-devtools integration for Claude, Codex, and the OpenCode overlay is tagged `desktop` and is only provisioned under `--profile desktop`. This is design-intent — chrome-devtools drives a real browser and does not belong in headless or server installs.

The gentle-ai provisioner can model cleanup before install by using separate entries in manifest order. Missing `action` still defaults to `install`; `yes` is only valid for `uninstall` because it confirms a cleanup action:

```yaml
provisioners:
  - tool: gentle-ai
    tags: [core]
    spec:
      action: uninstall
      agents: [codex, claude-code, opencode]
      components: [sdd]
      yes: true
  - tool: gentle-ai
    tags: [core]
    spec:
      preset: custom
      agents: [codex, claude-code]
      components: [engram, context7, persona, permissions]
```

To keep that requirement discoverable, both `install` and `update` print a one-line hint when the active profile skips provisioners another profile would select on this OS:

```
Note: profile "default" skips 3 provisioner(s); run with --profile desktop to include them.
```

File entries are profile-scoped the same way, and the `default` profile silently omits the `desktop`-tagged ones (the Ghostty and Zed configs, plus the OpenCode MCP overlay). To close the same discoverability gap, both commands print a parallel hint for skipped file entries:

```
Note: profile "default" skips 5 file entries; run with --profile desktop to include them.
```

Both hints also appear in `--dry-run`, so you can see what a profile omits before committing to it. The fuller profile that already selects every entry and provisioner prints no hint.

## References

- [`docs/scope.md`](scope.md) — `dots update` was deferred from v1 and delivered in v1.1.
- [`CONTEXT.md`](../CONTEXT.md) — vocabulary for Installed Repository, Source of Truth, Conflict, Drift, and Backup Set.
- [`docs/adr/0001-bootstrap-with-go-cli.md`](adr/0001-bootstrap-with-go-cli.md) — the Go CLI and thin-Git-wrapper tradeoffs.
