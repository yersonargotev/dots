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

## Read-only reuse amendment

Issue #341 authorizes `status`, `doctor`, `plan`, `deps check`, and `deps plan`
to reuse the Installed Selection when the invocation supplies neither
`--profile` nor `--tag`. Any explicit selection is complete for that invocation,
wins over recorded intent, and never rewrites Installation Metadata. Recorded
Profile names and explicit extra Tags are resolved again against the current
Install Manifest; the stored resolved Tag snapshot remains audit-only.

If neither explicit nor recorded intent exists, these commands return consistent
selection guidance and do not fall back to a manifest `default` or infer intent
from historical inventory. Human and machine reports identify whether the
effective selection was `explicit` or `recorded`. This amendment does not change
install, update, upgrade, migration, selection-delta, or removal policy.

## Update and upgrade reuse amendment

Issue #342 authorizes `update` and `upgrade` to use the same
explicit-over-recorded selection rule. Both commands resolve authoritative
intent before mutating the Installed Repository or replacing the binary, then
resolve that same intent again against the refreshed Install Manifest before
building or applying Managed Configuration. A Profile or extra Tag removed by a
Source of Truth refresh therefore stops configuration and Provisioner
application without falling back to a manifest default.

One resolved selection is threaded through the Install Plan, Managed Entries,
Provisioners, and text/JSON reports. Binary upgrade continuation carries the
ordered Profile names, explicit extra Tags, and `explicit` or `recorded` source
through hidden internal flags; it does not expose recorded intent as
caller-supplied public flags or persist the stored resolved Tag snapshot as
intent.

After terminal success, update/upgrade refreshes the Installed Selection,
including its current resolved Tag snapshot and provenance. A dry run,
cancellation, invalid refreshed selection, Managed Entry failure, Provisioner
failure, continuation failure, or metadata-write failure does not replace the
previous Installed Selection.

## Selection evolution amendment

Issue #343 authorizes update and upgrade to compare the selection resolved
before Source of Truth refresh with the same authoritative Profile and explicit
extra Tag intent resolved afterward. The deterministic delta reports previous
and current intent and effective Tags, plus added and removed Managed Entries,
Dependencies, and Provisioners. Binary continuation preserves the intent and
recomputes the pre-refresh comparison basis before advancing the Installed
Repository.

A saved Profile missing from the refreshed Install Manifest, or an explicit
extra Tag no longer declared by any Managed Entry, Dependency Set, or
Provisioner, blocks Managed Configuration and Provisioner application. The
structured error retains the delta and the stale intent; dots does not silently
rewrite it. Successful text and JSON output expose the same delta before
application.

Removed surfaces are informational. Selection evolution does not authorize
dots to unlink targets, delete ownership or Dependency metadata, uninstall
Provisioners or capabilities, or otherwise prune historical inventory.
Explicit selection replacement, reduction acknowledgement, legacy inference,
and automatic removal remain separate policy.
