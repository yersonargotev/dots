# Record authoritative Installed Selection

Installation Metadata historically recorded Profiles and Tags only on individual
Managed Entries and Provisioners. Those records describe what earlier runs
installed, but their union cannot identify one current machine-level intent:
records can survive partial runs and can represent different selections over
time.

We add an optional top-level Installed Selection to Installation Metadata v3.
It records, in deterministic order, the selected Profile names, explicit extra
Tags, the resolved Tag snapshot, Source of Truth provenance, and recording time.
Profile names and explicit extra Tags are the authoritative intent. The resolved
Tags are an audit snapshot of what that intent selected at recording time; they
do not replace the named Profiles or explicit extra Tags.

The Installed Selection is committed only at terminal success, after Managed
Entries, required Provisioners, and final convergence work complete. The commit
reloads and merges with current Installation Metadata so inventory written
during the run is retained. A failed or canceled run, including a failure to
write the new selection, preserves the previously recorded Installed Selection.

Installation Metadata v1 and v2 remain readable. Because the new field is
optional, their Managed Entry and Provisioner records are not promoted or
inferred into an authoritative Installed Selection. `dots installed` therefore
reports the Installed Selection separately from represented Tags and recorded
or inferred Profile coverage, and clearly reports when authoritative selection
metadata is absent.

ADR 0013 still governs first installation from the repository manifest:
`dots install` requires an explicit Profile and fails before Dependencies,
Managed Entries, Provisioners, home mutation, or state creation when none is
provided. Legacy fixtures may retain their existing `default` Profile
compatibility. This slice records Installed Selection but does not reuse it as
an implicit input to a later install, and it does not introduce a repository
`default` or `core` selection.

Consequences: a completed explicit install leaves one durable statement of the
operator's chosen Profiles and extra Tags, while per-record Profiles and Tags
remain historical inventory and ownership evidence. This ADR does not authorize
other commands to reuse the Installed Selection, migrate legacy metadata,
interpret future Profile changes, report selection deltas, or remove items that
are no longer selected.
