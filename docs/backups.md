# `dots backups`

`dots backups` inspects and restores the [Backup Sets](../CONTEXT.md) the Dotfiles CLI records under the state root (default `~/.local/state/dots/backups`). Every time `install` or `update` would overwrite an existing target — a `replace`, or an `adopt` with the symlink strategy — it first copies that target into a timestamped Backup Set and records [Backup Metadata](../CONTEXT.md) describing it. These commands read that history so preserved files can be audited and, in v1.1, returned to disk.

## `dots backups list`

`list` reports each recorded Backup Set: when it was created, why the backup was taken, and which targets it protected.

```
Backup Sets

  ID: backup-20260608T134850.191732000Z
  Created: 2026-06-08T13:48:50Z
  Reason: pre-install conflict protection
  Protected targets:
    - /home/user/.zshrc

Summary: 1 Backup Set
```

When no metadata exists yet, `list` reports that no Backup Sets are recorded in the state root rather than failing.

## `dots backups restore <set>`

`restore` returns the targets recorded in a Backup Set to the content preserved when the set was created. It is the safe inverse of the conflict `replace` path: nothing about a restore is one-way.

```
dots backups restore backup-20260608T134850.191732000Z
```

### What it does

1. **Finds the Backup Set.** The single argument is the Backup Set ID (as shown by `list`). An unknown ID stops with an error naming the state root searched.
2. **Checks provenance.** Backup Metadata records the `machine` and `repo` a set was captured on. If the set was captured on a different machine, `restore` refuses unless you pass `--force`. A set with no recorded machine (created before provenance tracking) is allowed, because there is nothing to mismatch.
3. **Plans the change.** For each recorded target, `restore` reports whether it would `create` the target (currently absent) or `overwrite` it (currently present). A Backup Set whose preserved files are missing is reported before any target is touched.
4. **Backs up what it would overwrite.** Before replacing any target that currently exists, `restore` records a new Backup Set with the reason `pre-restore safety backup`. A restore therefore never destroys the current state irreversibly — you can restore that safety set to undo it.
5. **Restores the preserved content.** Regular files are written back with their preserved permissions; symlinks are recreated to their preserved destination.

### Provenance and `--force`

```
Error: Backup Set backup-... was captured on machine "some-laptop" but this
machine is "workstation"; re-run with --force to restore anyway
```

The machine check exists because a Backup Set holds files captured against one workstation's layout. Restoring another machine's set is occasionally legitimate (migrating a config), so `--force` is the explicit, auditable escape hatch rather than a silent default.

### Flags

| Flag | Purpose |
|------|---------|
| `--dry-run` | Report the provenance and the per-target `create`/`overwrite` plan without touching any files or recording a safety backup. |
| `--force` | Restore even when the Backup Set was captured on a different machine. |
| `--home` | Home directory used to resolve the default state root and to bound restore targets. Use a sandbox path to avoid touching real config. |
| `--state-root` | State directory holding Backup Metadata and preserved files (default `~/.local/state/dots`). |

### Dry run

`dots backups restore <set> --dry-run` shows exactly what the restore would change and stops:

```
Restore Backup Set backup-20260608T134850.191732000Z
  Created: 2026-06-08T13:48:50Z
  Machine: workstation
  Repo: /home/user/.local/share/dots
  Reason: pre-install conflict protection

Planned changes:
  overwrite /home/user/.zshrc

Dry run: no files changed.
```

## Sandbox validation

Before using `dots install` against a real home directory, validate the safety
guarantee in a temporary sandbox: create a small manifest, pre-create the target
file under `--home`, run `dots install --no-tui` and choose `replace`, then run
`dots backups restore <set>` with the same `--home` and `--state-root`. The
expected result is:

- install records a `pre-install conflict protection` Backup Set before
  replacing the user-owned target;
- the managed file or symlink is installed under the sandbox home;
- restore returns the original content; and
- restore records a `pre-restore safety backup` for the managed state it
  overwrote.

The regression test `TestInstallReplaceAndBackupRestoreEndToEndUsesSandbox`
covers this journey with temporary `--home`, `--source-root`, and `--state-root`
paths only.

## References

- [`docs/scope.md`](scope.md) — automatic backup restore was deferred from v1 and delivered in v1.1.
- [`CONTEXT.md`](../CONTEXT.md) — vocabulary for Backup Set, Backup Metadata, Source of Truth, Conflict, and Drift.
- [`docs/adr/0001-bootstrap-with-go-cli.md`](adr/0001-bootstrap-with-go-cli.md) — the central backup location and `dots backups list`/`restore` design.
