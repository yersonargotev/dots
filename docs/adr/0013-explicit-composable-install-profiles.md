# Require explicit, composable Profiles

The Dotfiles CLI previously treated `default` as an implicit install baseline.
That made bare `dots install` select the `core` tag and install base shell,
terminal, editor, and Git configuration without the caller naming that intent.
This is convenient for the maintainer but unsafe for sharing the Source of Truth
with people who may only want one capability area.

We now require an explicit Profile selection for the repository manifest and make
Profile composition first-class. The repository no longer defines `default`; it
has an explicit `core` Profile (`tags: [core]`). Capability Profiles are pure:
`desktop`, `agents`, `web`, and `mobile` select only their own tag. The
opinionated `workstation` Profile remains a composite of `core`, `desktop`, and
`agents`; `web` and `mobile` stay opt-in.

`--profile` is repeatable anywhere profile selection is accepted. The effective
selection is the ordered, de-duplicated union of all selected Profile tags,
followed by any explicit `--tag` values. Human and JSON reports expose the
composed Profile list (`profiles`) and resolved `tags` so agents can branch on
structured data instead of inferring from prose.

Consequences: `dots install` with this repository manifest and no `--profile`
fails before dependency installation, Managed Entries, Provisioners, or home
mutation. Users run commands such as `dots install --profile core`,
`dots install --profile agents --profile web`, or
`dots install --profile core --profile agents --profile web` to state exactly
what they want. Legacy test fixtures may still define a `default` Profile, but
that name is no longer part of the repository manifest or public examples.

## Amendment: pure Profile invariant

A Profile is a removable name for an ordered Tag selection. Its declaration
contains descriptive metadata, lifecycle status, and Tags only; Dependencies,
Managed Entries, and Provisioners belong to their narrowest declarative owner.
Capability-wide Dependencies therefore use Tag-scoped Dependency Sets. The same
effective Tags on an operating system select the same declarative surface
whether supplied by Profiles or explicit `--tag` values. Historical manifests
may be read only for update-evolution inventory when they contain retired
Profile Dependencies; current manifest loading rejects that field with migration
guidance.

## Amendment: evidence-gated historical retirement

Tags select declarative surface only and do not trigger built-in Go cleanup.
Gentle AI retirement runs in the terminal historical-retirement workflow after
successful application and before Installed Selection recording. Its authority
is a successful historical `gentle-ai install` Provisioner receipt in
Installation Metadata; Profile fields, Provisioner Tag fields, and Installed
Selection containing `agents` are insufficient by themselves. The filesystem
adapter continues to remove only recognized marker-owned blocks, preserves
regular-file modes and unrelated bytes, reports non-regular targets for manual
cleanup, and fails closed before terminal Installed Selection recording.
