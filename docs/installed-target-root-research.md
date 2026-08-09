# Installed `xdg-state` target research

Date: 2026-08-09

Tracking issue: [#413](https://github.com/yersonargotev/dots/issues/413)

## Question

Why does `dots installed --output json` fail after the v0.68.0 Neovim
migration with:

```text
unsafe target "nvim/lazy-lock.json": target must be ~ or ~/...
```

and what contract must the correction preserve?

## Sources and scope

This note uses primary repository sources only: the v0.68.0 tag, its Git
history, source, tests, manifest, and project documentation. The migration was
introduced by [`c207c09` (`feat(nvim): separate config from seeded state
(#403)`)](https://github.com/yersonargotev/dots/commit/c207c09) and is included
in [`v0.68.0`](https://github.com/yersonargotev/dots/releases/tag/v0.68.0),
whose tagged commit is `2f3f5fe`.

## Delivery status

Issue #413 corrects the defect by threading the resolved XDG state home through
both direct inventory construction and legacy Installed Selection analysis,
then using the entry-aware resolver for metadata matching and Profile coverage
identity. Package and sandboxed CLI regression tests now cover the successful
match and complete coverage. The diagnosis below describes the pre-fix
`v0.68.0` implementation; links to that implementation are pinned to its tag.

## Symptom and minimal triggering state

v0.68.0 declares the lazy.nvim lockfile as a Managed Entry with the relative
target `nvim/lazy-lock.json`, `target_root: xdg-state`, `strategy: copy`, and
`ownership: seeded` ([`dots.yaml`](../dots.yaml#L491-L500)). After Installation
Metadata records its resolved target beneath `$XDG_STATE_HOME`, `dots installed`
must match that record against the current Install Manifest to build its
read-only inventory.

In the affected version, its first match attempt fails before JSON rendering: `matchEntry`
calls `plan.ResolveTarget(entry.Target, opts.Home)`
([`internal/installed/installed.go` at
`v0.68.0`](https://github.com/yersonargotev/dots/blob/2f3f5fe/internal/installed/installed.go#L249-L266)).
`ResolveTarget` deliberately accepts only `~` and `~/...`, producing the
reported error for the relative XDG-state target
([`internal/plan/plan.go` at
`v0.68.0`](https://github.com/yersonargotev/dots/blob/2f3f5fe/internal/plan/plan.go#L505-L524)). This is a
deterministic input-validation mismatch, not a damaged lockfile or an unsafe
target accepted by the manifest.

## Expected contract

The manifest contract explicitly allows the sole non-home target root:
`target_root: xdg-state` resolves a confined relative target beneath an
absolute `$XDG_STATE_HOME`, requires `ownership: seeded`, remains inside the
selected home, and is separate from dots' `--state-root`
([`docs/manifest.md`](manifest.md#L131-L143);
[`internal/manifest/manifest.go`](../internal/manifest/manifest.go#L457-L466)).

The planner already implements that contract in
`ResolveEntryTarget(entry, home, xdgStateHome)`
([`internal/plan/plan.go`](../internal/plan/plan.go#L527-L565)). It selects the
home resolver for ordinary entries and validates/constrains `xdg-state` entries
under the provided state-home. The machine-readable output contract also states
that XDG-state plan actions retain the absolute resolved target and expose
`target_root: "xdg-state"` ([`docs/agents/output-contract.md`](agents/output-contract.md#L179-L205)).

Therefore, `installed` must use the same entry-aware resolution contract when
matching metadata and deriving profile coverage. It remains a read-only report:
its command help promises that it never evaluates or mutates managed targets
([`internal/cli/installed.go`](../internal/cli/installed.go#L31-L34)).

## Root cause

In `v0.68.0`, `installed.Options` has `Home` but no `XDGStateHome`
([`internal/installed/installed.go` at
`v0.68.0`](https://github.com/yersonargotev/dots/blob/2f3f5fe/internal/installed/installed.go#L91-L96)),
and the CLI does not pass the already-resolved `paths.XDGStateHome` into
`installed.Build` ([`internal/cli/installed.go` at
`v0.68.0`](https://github.com/yersonargotev/dots/blob/2f3f5fe/internal/cli/installed.go#L51-L56)).
Consequently, `matchEntry` cannot call the entry-aware resolver and still uses
the old home-only resolver.

The adjacent `entryKey` repeats the same home-only call
([`internal/installed/installed.go` at
`v0.68.0`](https://github.com/yersonargotev/dots/blob/2f3f5fe/internal/installed/installed.go#L280-L286)).
It participates in profile-entry coverage, so changing only `matchEntry` would
avoid the immediate error but would leave XDG-state entries unable to contribute
their resolved key reliably.

The migration commit updated `selectionmigration.Analyze` to receive
`XDGStateHome`, but its nested `installed.Build` call also omitted that value.
That secondary propagation gap affected legacy metadata requiring Installed
Selection analysis. Installation, status, and doctor already used the new
target model, while both direct and nested installed-inventory construction did
not.

## Pre-fix coverage and regression seam

`TestInstalledJSONEnvelope` constructs only a `~/.zshrc` record
([`internal/cli/installed_test.go`](../internal/cli/installed_test.go#L13-L49)).
The Neovim lifecycle test does construct the XDG-state Managed Entry and
exercises install, status, update preservation, and uninstall
([`internal/cli/issue388_nvim_seeded_lifecycle_test.go`](../internal/cli/issue388_nvim_seeded_lifecycle_test.go#L41-L113)),
but never invokes `installed`. Thus no pre-fix test drives this exact command
path.

The focused existing tests pass:

```text
go test ./internal/cli -run 'TestInstalledJSONEnvelope|TestNeovimSeededStateLifecycleUsesConfinedXDGStateAndPreservesLocalEvolution' -count=1
ok   github.com/yersonargotev/dots/internal/cli
```

That green result confirms the coverage gap; it is not a reproduction of the
defect. The correct regression seam is a CLI test with a temporary home and
absolute temporary `XDG_STATE_HOME`, a manifest containing the seeded entry,
and Installation Metadata containing the resolved XDG-state target. It should
assert that `installed --output json` exits `0`, emits the `installed` envelope,
and reports that entry as `manifest_matched: true` with complete coverage where
otherwise represented. Against `v0.68.0`, that test is red with the exact error
in the symptom section. After the #413 correction, package-level and CLI
variants are green, including legacy metadata that exercises Installed
Selection analysis.

## Scope and implications

- The defect is limited to manifest-to-metadata matching and its derived
  profile coverage in `dots installed`; it does not invalidate the security
  confinement rules for XDG state.
- The correction must retain `ResolveEntryTarget`'s checks rather than relaxing
  `ResolveTarget` to accept arbitrary relative paths, because the latter is the
  home-only safety boundary.
- Every use of a resolved Managed Entry identity in the installed inventory
  should use the same root-aware resolution, so a successful match and profile
  coverage describe the same target.
- The remediation includes the dedicated regression tests described above;
  this note records the evidence and preserved contract rather than serving as
  an implementation mechanism.
