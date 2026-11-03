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
code. The continuation carries the authoritative Profile and explicit extra Tag
intent together with its `explicit` or `recorded` source, so recorded intent is
not reinterpreted as caller-supplied flags.

Current continuations also carry exclusion intent and the exact previous
Installed Selection identity, rejecting concurrent authority changes before
configuration effects. The released v0.96.1 caller lacks that identity. Its
recognized continuation is supported as a fresh request: dots captures current
authority, preserves the complete carried selection and options, and uses the
same locked ownership checks and terminal selection commit. This compatibility
is limited to v0.96.1's complete Release Artifact or Homebrew continuation;
missing or malformed internal arguments do not enable it.

If that fresh request reduces Installed Selection, interactive execution asks
for a new, distinct confirmation. An inherited `--acknowledge-selection-change`
does not count. With `--yes` or JSON output, dots stops before repository,
configuration or metadata changes and reports
`selection-change-acknowledgement-required`. Follow its complete public recovery
command after reviewing the reduction. Text errors include the command; JSON
also includes `data.remediation.recommended_command`. It preserves repeated
Profiles, extra Tags, custom roots, manifest and relevant output/execution
options. Do not replace it with a selector-free retry when you originally
requested an explicit selection. Legacy migration continuations also require a
new public request with the complete explicit selection.
Recovery for a recorded request continues to reuse recorded intent.

At that point the binary phase has already completed. A failed or declined
continuation preserves the previous Installed Selection. Repository or
configuration progress may remain after a later failure; the usual ownership
checks and safe retry behavior still apply. Compatibility never uninstalls
packages or automatically retires configuration.

## Previewing changes

```bash
dots upgrade --profile workstation --dry-run
```

Dry-run mode previews both phases without replacing the binary, updating the
Installed Repository, writing Installation Metadata, or changing home files.
There is no implicit Profile for the Source of Truth phase. When selection flags
are absent, upgrade reuses the Installed Selection; otherwise the supplied
Profiles and extra Tags form the complete selection. Repeat `--profile` to
compose selections. `workstation` covers `core + desktop + agents`; `web` and
`mobile` remain opt-in Profiles.

## Machine output

`dots upgrade` supports the Agent Output Contract:

```bash
dots upgrade --profile workstation --dry-run --output json
dots upgrade --profile workstation --yes --output json
```

A real non-dry-run JSON upgrade requires `--yes` so Machine Output Mode never
prompts on stdout. If the complete explicit selection removes a recorded
Profile or extra Tag, it also requires `--acknowledge-selection-change`.

After a successful explicit install, an unattended workstation upgrade can
reuse the Installed Selection:

```bash
dots upgrade --yes
```

For Installation Metadata v1/v2 without an Installed Selection, upgrade derives
a non-authoritative Selection Migration Candidate from historical Managed Entry
and Provisioner records plus current manifest, target, and Source of Truth
evidence. Only an unambiguous candidate can be confirmed interactively.
Ambiguous or absent evidence requires repeated `--profile` and `--tag` flags
that state the complete selection. `--yes`, JSON output, and other
non-interactive runs fail before binary or repository mutation with the stable
`selection-migration-required` error and a recommended explicit command when
possible. Upgrade never chooses an implicit default.

An explicit selection that differs from Installed Selection is reported before
binary replacement. The delta contains added and removed Profiles, explicit
extra Tags, effective Tags, Managed Entries, Dependencies, and Provisioners.
Removing a recorded Profile or explicit extra Tag requires a distinct
interactive confirmation; non-interactive execution requires both `--yes` and
`--acknowledge-selection-change`, otherwise upgrade returns
`selection-change-acknowledgement-required` before binary or repository
mutation. Selection is then validated before binary replacement and re-resolved against the
refreshed manifest before Managed Configuration is applied. Upgrade reports
effective Tag and selected Managed Entry, Dependency, and Provisioner additions
and removals before application. Missing Profiles and explicit extra Tags no
longer declared by selectable manifest surfaces stop the mutating phase without
an implicit default or automatic intent rewrite. Removed surfaces are never
automatically deleted or uninstalled. Only terminal success records or refreshes
Installed Selection metadata; declined migration confirmation, binary, update,
Provisioner, or continuation failures preserve the previous state and legacy
ownership records.

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
  --acknowledge-selection-change \
  --no-tui
```

Use temporary `--home`, `--source-root`, and `--state-root` values when testing
upgrade behavior so real workstation configuration is never modified.

After an upgrade, `dots installed` provides the official read-only report for
what Installation Metadata currently records. It keeps the authoritative
Installed Selection, any non-authoritative Selection Migration Candidate, and
historical Profile/Tag coverage, Managed Entries, Provisioner runs, and Source
of Truth provenance separate.
