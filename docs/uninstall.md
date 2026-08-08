# `dots uninstall`

`dots uninstall` reverses a `dots install`. It is the mirror of `install`: where install computes an [Install Plan](../CONTEXT.md) and writes managed configuration, uninstall computes an [Uninstall Plan](../CONTEXT.md) and removes it. The command is driven entirely by the [Installation Metadata](../CONTEXT.md) (`~/.local/state/dots/installed.json`) — the record of what `dots` installed is the only source of truth. Uninstall never touches a path it did not record.

```bash
dots uninstall --dry-run   # preview the Uninstall Plan, change nothing
dots uninstall             # preview, then ask for confirmation before removing
dots uninstall --yes       # remove owned targets without prompting
```

## What it does

1. **Reads the Installation Metadata.** Each newly recorded target carries its source, install strategy, explicit whole/partial ownership, and — for copied files — a content hash. Strict-JSON subset records also carry the prior dots-owned contribution. A run with no recorded targets reports that and stops.
2. **Builds the Uninstall Plan.** Every recorded target is classified against current disk state into one of four actions (see below).
3. **Previews by default.** The plan is printed before anything is removed. Without `--yes`, the command then asks for confirmation; answering anything other than `y`/`yes` cancels with no changes.
4. **Removes only owned content.** Symlinks are deleted only when they still resolve to the recorded repository source; whole-owned copies only when their content still matches the recorded hash. Strict-JSON subset targets lose only the recorded contribution and remain in place while external content survives.
5. **Optionally restores backups.** With `--restore-backups`, each removed target's most recent Backup Set is restored, returning the workstation to its pre-install state.
6. **Prunes the metadata.** Records for successfully removed targets are dropped, and the metadata file is deleted entirely once nothing remains.

## Plan actions

| Action | Meaning | Default behavior |
|--------|---------|------------------|
| `remove` | A symlink or whole-owned copy is still proven owned, or a strict-JSON subset contribution can be subtracted safely. | Removes the target or only its proven contribution. |
| `modified` | A whole-owned copy drifted, or a recorded partial contribution changed externally. | Whole-owned copies may be forced; partial targets are always preserved. |
| `not-owned` | A symlink that is missing or now points elsewhere, or a target whose type changed. `dots` can no longer prove it owns it. | Skipped, always. |
| `skip` | A copied target that is already absent. | Nothing to do. |

```
Uninstall Plan

  remove     /home/user/.zshrc
  modified   /home/user/.gitconfig
  not-owned  /home/user/.tmux.conf

Summary: 1 to remove, 1 modified, 1 not-owned, 0 skipped
```

## Ownership and drift safety

Uninstall is conservative by design: it would rather leave a file in place than delete something it does not own.

- **Symlinks** are verified by destination, not by name. A link is removed only if `readlink` still resolves to the source `dots` recorded (resolved against `--source-root`). A link the user repointed, or replaced with a real file, is `not-owned` and left untouched.
- **Whole-owned copies** are verified by content hash. A file whose hash still matches the recorded value is removed; a file you edited is `modified` and preserved unless you pass `--force`.
- **Strict-JSON subsets** are reconciled against their recorded contribution. Matching owned values are removed recursively, target-only keys and array items survive, and the physical file is deleted only when no external content remains. Changed owned values are `modified`; `--force` never broadens partial ownership or deletes the whole co-owned target. Legacy Installation Metadata without contribution evidence may remove an unchanged hash match, but cannot force-remove a modified copy; a safe install must first record current ownership evidence.
- **Re-verification at apply time.** The classification shown in the preview is recomputed against disk immediately before each removal, so a target that changed between preview and apply is never removed by surprise.
- **No removal escapes HOME.** Every target is validated to stay inside the home sandbox, with the same no-symlink-escape guards install uses, before it is deleted.

## Restoring pre-install state

When `install` overwrites or adopts an existing file, it first records a pre-install Backup Set. `dots uninstall --restore-backups` reuses that history: after removing a managed target, it restores the most recent Backup Set that covers it. Because the managed target is removed first, the restore re-creates the original file rather than overwriting anything.

```bash
dots uninstall --restore-backups
```

A Backup Set is restored at most once even if it covers several removed targets, and restore targets are bounded to the home sandbox just like removals.

## Flags

| Flag | Purpose |
|------|---------|
| `--dry-run` | Print the Uninstall Plan and stop without modifying files. |
| `--yes` | Remove owned targets without the confirmation prompt. |
| `--force` | Also remove whole-owned copied targets whose content drifted from the recorded hash; never broadens partial ownership. |
| `--restore-backups` | Restore each removed target's most recent Backup Set after removal. |
| `--source-root` | Installed repository root used to verify symlink ownership (default `~/.local/share/dots`). |
| `--home` | Target home directory to uninstall from (default: the current user's home). Use a sandbox path to avoid touching real config. |
| `--state-root` | State directory holding Installation Metadata (default `~/.local/state/dots`). |

## Scope boundaries

- Uninstall does **not** remove Dependencies — that stays in the `deps` domain.
- It does **not** touch any path `dots` never recorded.
- `--restore-backups` does not restore a whole-file Backup Set over a partially
  owned target, because doing so could overwrite external content retained by
  the partial uninstall.
- It adds no rollback or version semantics beyond Backup Set restore.

## References

- [`docs/scope.md`](scope.md) — reversible uninstall in the v1 scope.
- [`docs/backups.md`](backups.md) — the Backup Sets `--restore-backups` reuses.
- [`CONTEXT.md`](../CONTEXT.md) — vocabulary for Uninstall Plan, Install Plan, Installation Metadata, Backup Set, and Drift.
