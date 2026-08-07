# Agent Output Contract

`dots` exposes a stable machine-readable surface so autonomous agents can drive
the CLI without scraping human prose. The contract has two parts: a JSON
envelope and semantic exit codes. The full rationale lives in
[`docs/adr/0006-agent-output-contract.md`](../adr/0006-agent-output-contract.md);
this page is the usage reference.

## Output format

A persistent `--output text|json` flag selects the surface. `text` is the
default, so human output and Golden Output Tests are unchanged.

```bash
dots status --output json
dots installed --output json
dots doctor --output json
dots plan   --output json
dots deps check --output json
```

Under `--output json`, every result-producing command emits one centralized
envelope on stdout. Human-only surfaces such as help text, shell completion
scripts, and parent commands without a selected subcommand return an error
envelope instead of falling back to prose.

Machine Output Mode implies non-interactive: it never writes a prompt onto the
JSON stream. Commands that would normally prompt require an explicit non-
interactive choice such as `--dry-run` or `--yes`; otherwise they return an
error envelope. `--output json` owns stdout; diagnostics and child-process
progress go to stderr, so `dots status --output json | jq` is always safe.

## Envelope shape

Every result-producing command renders the same envelope. The `data` object is
the command's domain report, so the JSON and text surfaces never disagree on
state.

```json
{
  "schema_version": "5",
  "command": "doctor",
  "status": "ok",
  "data": { "...": "command-specific report" }
}
```

| Field            | Meaning                                                              |
|------------------|---------------------------------------------------------------------|
| `schema_version` | Envelope version, bumped independently of the CLI binary.           |
| `command`        | The command that ran (`status`, `installed`, `plan`, `doctor`, `deps check`). |
| `status`         | Outcome discriminator: `ok` \| `findings` \| `error`.               |
| `data`           | The command's domain report (present on `ok` / `findings`; selected action-command errors may include a partial report). |
| `error`          | Structured error string (present only on `status: "error"`).        |

On failure the envelope is still valid JSON. Most errors contain only `error`;
commands that can preserve a useful partial report may also include `data`.
Schema version `3` introduced this partial-error report allowance:

```json
{
  "schema_version": "5",
  "command": "doctor",
  "status": "error",
  "error": "read manifest: open dots.yaml: no such file or directory"
}
```

The error envelope is best-effort: a malformed invocation that fails before its
command resolves (unknown command, or a flag error parsed before `--output`)
reports on stderr with exit `1`, because there is no command context to label.

## Semantic exit codes

Exit codes let a caller branch on the outcome without parsing output.

| Code | Name           | Meaning                                                                 |
|------|----------------|-------------------------------------------------------------------------|
| `0`  | `ExitOK`       | Ran successfully: diagnostics found alignment, or an action command completed. |
| `2`  | `ExitFindings` | Ran successfully but surfaced a non-error divergence to act on: Drift, an unresolved Conflict, a missing Dependency, or a doctor concern. |
| `1`  | `ExitError`    | The command failed to run: bad manifest, I/O failure, or misuse.        |

The `2` findings code applies only to read-only diagnostic commands such as
`status`, `plan`, `doctor`, and dependency diagnostics. Action commands keep
`0`/`1` even when their JSON payload includes a plan or preview. Reserving `1`
for real failures means existing `|| handle-error` scripts never confuse a
divergent-but-healthy run with a broken one.

```bash
dots status --output json
case $? in
  0) echo "aligned" ;;
  2) echo "divergence to act on" ;;
  1) echo "execution error" ;;
esac
```

## Contract scope

The envelope carries the machine-meaningful report, not every piece of human
prose the text surface prints:

- `status` includes selected Provisioner completion state under
  `data.provisioners.summary` and per-command state under
  `data.provisioners.items`. Pending or failed Provisioners are findings so
  agents can distinguish aligned Managed Entries from an incomplete full-profile
  setup. Read dependency readiness for those commands from `doctor`.
- `status`, `doctor`, `plan`, `deps check`, `deps plan`, `update`, and `upgrade` include
  `data.selection` with `source` (`explicit` or `recorded`), ordered `profiles`,
  explicit `extra_tags`, and recomputed `effective_tags`. When no selection
  flags are supplied, these commands resolve the recorded Profile names and
  extra Tags against the current Install Manifest; the stored `resolved_tags`
  remains audit-only. Update and upgrade resolve the same intent again after
  Source of Truth refresh and preserve its source across binary continuation.
  Their selection report also includes `delta` with deterministic `previous`
  and `current` snapshots; `added` and `removed` effective Tags, Managed
  Entries, Dependencies, and Provisioners; and ordered `missing_profiles` and
  `stale_extra_tags` arrays. Empty arrays remain present whenever `delta` is
  emitted. A blocking evolution error returns the same delta as
  `data.selection_delta`.
  Any explicit Profile or Tag selection wins completely for that invocation and
  is never merged with recorded intent.
  If neither source exists, the command returns an execution error (`1`) with
  selection guidance rather than falling back to a manifest `default`. For
  Installation Metadata v1/v2, historical evidence may be reported as a
  non-authoritative migration candidate, but is never consumed implicitly.

- `installed` is a read-only report over Installation Metadata. Its optional
  `data.installed_selection` is the authoritative machine-level intent recorded
  by the last successful explicit install: ordered `profiles`, ordered
  `extra_tags`, the ordered `resolved_tags` snapshot, Source of Truth
  `provenance`, and the selection recording time in
  `provenance.recorded_at`. The text surface renders the same information in a
  distinct **Installed Selection** section.
- The existing top-level `data.tags`, `data.profiles`, `data.managed_entries`,
  and `data.provisioners` remain historical inventory evidence. In particular,
  represented Tags and recorded or inferred Profile coverage are not an
  authoritative selection. When v1/v2 or other legacy Installation Metadata has
  no Installed Selection, JSON omits `data.installed_selection` and text says
  that no authoritative Installed Selection is recorded; dots does not promote
  inventory inference. The command is informational rather than an alignment
  diagnostic, so an absent Installed Selection or partial Profile coverage
  remains `status: "ok"`; use `status` or `doctor` when an agent needs
  drift/dependency findings.
- For Installation Metadata v1/v2 without an Installed Selection, `installed`
  may add `data.selection_migration` alongside (never inside) the historical
  inventory. It contains stable arrays `profiles`, `extra_tags`,
  `effective_tags`, and `ambiguity_reasons`, plus `confidence` and, only when
  available, `recommended_command`. This candidate is non-authoritative and
  does not change `data.installed_selection`.
- When selection migration cannot proceed non-interactively, selection-aware
  commands return exit `1`, `status: "error"`, and error code
  `selection-migration-required`. Error `data` contains `code`, nullable
  `candidate`, and `remediation`; candidate arrays remain present even when
  empty, and `remediation.recommended_command` is included when the evidence
  supports an explicit command. `--yes` and JSON output never confirm a
  candidate.
- Mutating `install`, `update`, and `upgrade` reports may add
  `data.selection.change` when a complete explicit request differs from the
  authoritative Installed Selection. `change.delta` contains stable
  `previous`/`current` intent snapshots and `added`/`removed` arrays for
  `profiles`, `extra_tags`, `effective_tags`, `managed_entries`,
  `dependencies`, and `provisioners`. The sibling booleans
  `acknowledgement_required` and `acknowledgement_accepted` let automation
  distinguish a preview, an accepted change, and a blocked reduction.
  `install` exposes the same portable `data.selection` object as update and
  upgrade.
- A non-interactive explicit request that removes a recorded Profile or extra
  Tag requires both `--yes` and `--acknowledge-selection-change`. Without both,
  the command exits `1` before mutation with
  `selection-change-acknowledgement-required`; error `data` contains `code` and
  the complete `selection_change`. This acknowledgement is independent from
  Conflict Resolution and `--backup-and-replace`.
- `plan`'s `resolved_source` (a per-machine absolute path) and `deps`'s
  `probe_detail`/`hint` (unstable human prose) are excluded. Agents key on the
  portable, structured fields (`source`, `present`, `warning`, and the
  `deps plan` install action).
- `plan` action `status` values are portable intent, not prose. `create`,
  `update`, and `unchanged` are non-findings; `conflict` and `missing-source`
  are findings. `update` means dots has enough Installation Metadata and
  Entry Ownership proof to safely mutate a previously managed target, creating a
  Backup Set before writing, while preserving the conservative conflict model
  for unmanaged or incompatible targets.
- Plan actions and Status Managed Entry items may add the optional reason
  `source-override-not-selected` and a deterministic `matching_tags` array when
  a conflicting target exactly matches one or more alternate sources whose
  `source_overrides` tags were not selected. These fields diagnose the omitted
  selection without replacing the action's `conflict` status or the Status
  item's `conflict` state. To recover the intended selection, omit explicit
  selection flags so dots can reuse an available Installed Selection, or supply
  each intended exact tag with repeated `--tag <tag>`.
- Dependency provider availability is an internal planning check, not a JSON
  contract field. `deps plan` and dependency install previews expose the stable
  outcome (`status`, selected `provider`, executable action, `manual` guidance)
  plus portable provider `candidates`; they do not expose whether host-local
  commands such as `brew`, `apt-get`, or `sudo` were present on the machine that
  produced the report.
- Selected User-Local Provider actions include `user_local` with the resolved
  recipe, version, artifact, URL, checksum/digest, platform, layout, command,
  home-owned destination, and installed link path. Rolling recipes expose these fields only after
  official metadata resolution succeeds; a resolution failure is an execution
  error rather than a partially installable action.
- Dependency reports include `requirement` (`required` or `optional`) anywhere a
  dependency item/action is exposed. Omitted manifest values are normalized to
  `required`; optional unresolved Dependencies remain reportable but do not
  block Managed Configuration during integrated install.
- `install` reports Package Manager Setup separately from Dependencies under
  `data.package_manager_setup` when selected required Dependencies need
  package-manager preparation before they can be provisioned. For example, a
  macOS Homebrew setup preview or gate is reported there instead of rendering
  Homebrew as a normal Dependency item.
- `install --dry-run --output json` includes dependency preflight under
  `data.dependencies.preview` before the file `data.plan`. A confirmed
  `install --yes --output json` includes dependency execution results under
  `data.dependencies.result` alongside the same install plan and provisioner
  plan, so agents can inspect what was provisioned before Managed
  Configuration was applied. If the dependency gate fails, the error envelope
  still includes this partial install report so agents can inspect the failed
  Dependency result and prove Managed Configuration was not applied.
  `data.dependencies` is optional and omitted when dependency provisioning is
  intentionally bypassed with `install --skip-deps`. When
  `install --yes --backup-and-replace --output json` replaces Conflicts,
  `data.backup_sets` lists the Backup Sets created by that run, including each
  set ID, target list, and state-root path.

Profile-aware reports retain the compatibility `profile`, `profiles`, and
`tags` fields. For selection-aware commands, agents should prefer the
portable `selection` object because it distinguishes invocation intent from
recorded machine intent and separates explicit extra Tags from effective Tags.

The domain reports' `json:` field names are part of the public contract.
Renaming, removing, or changing the meaning of an existing field is a
`schema_version` bump, locked by the JSON Golden Output Test
(`TestEnvelopeGolden`). Adding a portable, optional field to an existing report is
schema-compatible and keeps the current `schema_version`, but it must update the
matching JSON golden so agent-visible shape changes stay reviewable.
