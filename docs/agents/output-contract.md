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
  "schema_version": "1",
  "command": "doctor",
  "status": "ok",
  "data": { "...": "command-specific report" }
}
```

| Field            | Meaning                                                              |
|------------------|---------------------------------------------------------------------|
| `schema_version` | Envelope version, bumped independently of the CLI binary.           |
| `command`        | The command that ran (`status`, `plan`, `doctor`, `deps check`).    |
| `status`         | Outcome discriminator: `ok` \| `findings` \| `error`.               |
| `data`           | The command's domain report (present on `ok` / `findings`).         |
| `error`          | Structured error string (present only on `status: "error"`).        |

On failure the envelope is still valid JSON:

```json
{
  "schema_version": "1",
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

The envelope carries the machine-meaningful report, not everything the text
surface prints:

- `status`'s informational provisioner listing is text-only; read machine-readable
  Provisioner readiness from `doctor` instead.
- `plan`'s `resolved_source` (a per-machine absolute path) and `deps`'s
  `probe_detail`/`hint` (unstable human prose) are excluded. Agents key on the
  portable, structured fields (`source`, `present`, `warning`, and the
  `deps plan` install action).

The domain reports' `json:` field names are part of the public contract;
renaming a field is a `schema_version` bump, locked by the JSON Golden Output
Test (`TestEnvelopeGolden`).
