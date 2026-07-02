# `dots upgrade`

`dots upgrade` refreshes the complete dots-owned system in one command: the
[`dots` command](../CONTEXT.md#command-name), the
[Installed Repository](../CONTEXT.md#installed-repository),
[Managed Entries](../CONTEXT.md#managed-entry), and
[Provisioners](../CONTEXT.md#provisioner). It does **not** run broad system
package upgrades or manage non-dots dependencies.

The command runs in two phases:

1. **Binary phase** — detect how the current `dots` command was installed and
   update only that command.
2. **Source of Truth phase** — re-run the same safe flow as [`dots update`](update.md):
   fast-forward the Installed Repository, compute the Install Plan, resolve
   conflicts, apply Managed Entries, and re-run selected Provisioners.

If the binary phase fails, `dots upgrade` stops before touching the Installed
Repository or workstation home files.

## Installation channels

| Channel | Behavior |
| --- | --- |
| Homebrew Distribution | Runs `brew update`, then `brew upgrade yersonargotev/tap/dots`. |
| Release Artifact / Bootstrapper | Downloads the latest Release Artifact, verifies its checksum, writes `dots.new`, preserves the current binary as `dots.old`, then atomically promotes the new binary. |
| Development/local build | Aborts with a manual rebuild message. |

After a successful binary change, `dots upgrade` execs the new binary through a
hidden continuation phase so the Source of Truth phase runs with the updated
code.

## Previewing changes

```bash
dots upgrade --profile workstation --dry-run
```

Dry-run mode previews both phases without replacing the binary, updating the
Installed Repository, writing Installation Metadata, or changing home files.
There is no implicit Profile for the Source of Truth phase: repeat `--profile`
to compose selections. `workstation` covers `core + desktop + agents`; `web` and
`mobile` remain opt-in Profiles.

## Machine output

`dots upgrade` supports the Agent Output Contract:

```bash
dots upgrade --profile workstation --dry-run --output json
dots upgrade --profile workstation --yes --output json
```

A real non-dry-run JSON upgrade requires `--yes` so Machine Output Mode never
prompts on stdout.

For an unattended workstation upgrade, keep the explicit Profile selection and
add `--yes`:

```bash
dots upgrade --profile workstation --yes
```

## Source of Truth flags

The Source of Truth phase accepts the same relevant flags as `dots update`:

```bash
dots upgrade \
  --file dots.yaml \
  --profile core \
  --profile desktop \
  --source-root ~/.local/share/dots \
  --home "$HOME" \
  --state-root ~/.local/state/dots \
  --yes \
  --no-tui
```

Use temporary `--home`, `--source-root`, and `--state-root` values when testing
upgrade behavior so real workstation configuration is never modified.
