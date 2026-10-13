# Herdr Tasks and Codex shared-daemon feasibility

Research date: 2026-10-12. Tracking: [dots #550](https://github.com/yersonargotev/dots/issues/550). Installed binary: `codex-cli 0.162.1`; installed Herdr: `0.9.3`, protocol `22`; Herdr Tasks revision: `add3f2f855ca7687e4be4b4b57e1400d2a06066d`. This note recommends a decision; it does not supersede [ADR 0021](adr/0021-herdr-task-identity-plugin.md) or authorize removal.

## Decision

No supported, demonstrated path was found for the current Herdr Tasks plugin to recover the originating pane from an ordinary Codex TUI attached to the shared daemon. This is not proof that integration is impossible. A client-side bridge or upstream Codex change could supply the missing association. Those are additional integrations to design and maintain, rather than a small correction in title storage.

Under the requirement that normal `codex` work without special launch flags, transcript parsing, another model call, or manual session reporting, retiring Herdr Tasks is a reasonable recommendation unless maintaining that new bridge is worthwhile. Repository deletion should follow dependency removal, a release, and explicit approval of the concrete deletion scope.

## Official supported boundaries

Official [Hooks documentation](https://learn.chatgpt.com/docs/hooks#common-input-fields) defines session identity and session working directory in hook input, but no originating terminal client ID, terminal pane ID, or arbitrary client environment. `SessionStart` adds a source value. Plugin-specific environment variables identify plugin directories. The transcript is explicitly not a stable interface. These facts establish the public contract, not a guarantee that no undocumented extension exists.

The official [App Server documentation](https://learn.chatgpt.com/docs/app-server) describes a client/server protocol and generating protocol bindings from the installed binary. A custom client can retain the returned thread identity and its own terminal association. That would be a separate client/bridge, not functionality automatically provided by a lifecycle hook.

The installed `codex --help` explicitly describes `--no-daemon` as running without the shared background server, even when it is running. This establishes native support for shared-daemon operation in 0.162.1, but does not establish what first started this user's daemon.

## Installed protocol evidence

Generated with:

```bash
codex app-server generate-ts --experimental --out /tmp/herdr-tasks-codex-daemon-ts
```

Relevant generated files:

| Boundary | Fields relevant to routing |
| --- | --- |
| `InitializeParams.ts`, `ClientInfo.ts` | Client name/title/version and capability negotiation; no environment map |
| `v2/ThreadStartParams.ts` | Session cwd, config overrides, source classification, environments; no terminal client environment |
| `v2/ThreadResumeParams.ts` | Thread ID and config overrides; no originating pane field |
| `v2/TurnStartParams.ts` | Context fragments and Responses API metadata are not documented hook environment fields |
| `v2/TurnEnvironmentParams.ts` | `environmentId`, `cwd`, `runtimeWorkspaceRoots`; not arbitrary process environment variables |

`config` permits per-thread configuration, so schema absence alone cannot exclude every imaginable config-based bridge. The next probe specifically tested the most plausible existing shell environment setting.

## Isolated runtime probe

The probe starts a disposable app-server process with a temporary `CODEX_HOME` and workspace. It does not connect to or alter the live daemon, real hook configuration, title database, or terminal panes. The server receives `HERDR_PROBE_SENTINEL=SERVER`; `thread/start` receives:

```json
{"config":{"shell_environment_policy.set":{"HERDR_PROBE_SENTINEL":"CLIENT"}}}
```

A synthetic `SessionStart` command records only the sentinel and input key names. The final probe leaves shell snapshots at their defaults, returns `continue:false` from the hook, and selects a dummy provider at `http://127.0.0.1:1`. There is no actual model invocation. A preceding probe without the stop response could only attempt the nonexistent loopback endpoint.

Executed command:

```bash
python3 /tmp/herdr-daemon-env-probe.py
```

Relevant output:

```text
start_ok True
result {'sentinel': 'SERVER', 'keys': ['cwd', 'hook_event_name', 'model', 'permission_mode', 'session_id', 'source', 'transcript_path']}
PASS: hook inherits server sentinel despite per-thread shell env override
```

The server sentinel was observed in two isolated runs. This is a regression-capable test of the environment boundary, not a full end-to-end reproduction of the user's missing sidebar title. It establishes that `shell_environment_policy.set` does not supply the per-thread value to this native hook in the installed version.

Trust limitation: the harness copied the synthetic hook's exact `key` and `currentHash` from native `hooks/list` into a trust record in the temporary config. This was not a native interactive trust review and is not a recommended installation method. No live trust was changed. Do not retain that mechanism as production integration or represent this probe as validation of a supported trust flow. No further trust-fixture experiments are required for this conclusion.

## Alternatives and limits

1. **Upstream client context support.** Codex could convey terminal context on attach/start/resume and expose it to hooks, with explicit semantics for multiple clients and pane moves. This is technically plausible but not demonstrated by the present contract.
2. **A client bridge owned by Herdr.** A custom app-server client or CLI modification knows both the thread ID and its pane. An authenticated mapping could serve hooks. Stock TUI hooks cannot independently reconstruct that mapping. OSC terminal signaling could be one transport only if the CLI emits a trustworthy thread identity to the correct terminal; no existing supported stock mechanism was established here.
3. **Per-pane server isolation.** Separate processes avoid shared process environment, but special launch configuration or `--no-daemon` fails the requested ordinary-CLI criterion. It also does not address moves/resume lifecycle by itself.
4. **Per-thread shell environment policy.** The probe shows that a `thread/start` `shell_environment_policy.set` override does not reach `SessionStart` in 0.162.1. Static daemon-global or per-project pane values would not disambiguate multiple clients.
5. **Process ancestry, cwd, transcript, or title inference.** These lack a reliable session-to-client association under a shared daemon. They should not replace an explicit contract.

The current evidence supports “no supported plugin-only fix demonstrated for stock Codex 0.162.1,” not “Codex can never support it.”

## Live evidence and failure boundary

Read-only inspection of the affected installation established:

- The Codex TUI in `packy/w1:p1` inherited the correct Herdr socket and pane, but connected to the shared app-server worker whose environment selected `ge-web/w2:p1`. `lsof` matched the TUI socket peer to the daemon socket descriptor. Both binaries report version `0.162.1`.
- The native Herdr Codex integration is installed as version 8. Its installed `herdr-agent-state.sh` reads `HERDR_SOCKET_PATH` and `HERDR_PANE_ID`, and reports the hook payload's session ID to that destination.
- The plugin store already contains the affected conversation's Task Name, `Evaluar issue #828`. Two read-only `herdr --session packy api snapshot` checks still found neither `agent_session` nor `tokens.herdr_task_name` in `w1:p1`.
- Therefore successful naming/persistence does not prove correct Pane Binding. Both the native identity hook and the task hook can share the same wrong destination; validating one against the other does not independently establish the physical client pane. This is the supported explanation of the observed failure, not a claim that every hook failure has the same cause.

The plugin defaults its socket and pane flags from those environment variables and validates the hook session against the selected native occupant. Restoring a saved name cannot repair a missing or misdirected native Conversation Identity. See pinned [flag binding](https://github.com/yersonargotev/herdr-tasks/blob/add3f2f855ca7687e4be4b4b57e1400d2a06066d/internal/app/app.go), [producer implementation](https://github.com/yersonargotev/herdr-tasks/blob/add3f2f855ca7687e4be4b4b57e1400d2a06066d/internal/app/producers.go), and [producer contract](https://github.com/yersonargotev/herdr-tasks/blob/add3f2f855ca7687e4be4b4b57e1400d2a06066d/docs/producers.md).

CodeGraph was unavailable in the tool catalog and PATH; targeted source reads were used. No native identity, lifecycle state, tab name, or other conversation's task was reported during the research. No live configuration or session was changed.

## Acceptance gate for keeping the capability

A future bridge or upstream change must pass one isolated two-client scenario before continued investment in task naming is justified:

1. Start two ordinary Codex clients, with the shared daemon enabled and shell snapshots left at their normal defaults, in different Herdr panes but the same working directory.
2. Verify an explicit native binding for each conversation to its own socket/pane; neither hook may derive the destination from a daemon-global value or guess from cwd.
3. Publish distinct Task Names, continue both conversations, and verify neither name or identity crosses panes.
4. Exit/resume one conversation in another pane and replace the other pane's conversation. Verify binding follows the supported lifecycle, saved names restore only to their owner, and old callbacks cannot claim the replacement.
5. Specify simultaneous attachment of one conversation from two clients: reject ambiguity or define a supported owner. Test delayed hook delivery against that rule.

The existing successful isolated `--no-daemon` trial does not satisfy this gate. The research probe only rejects a proposed config workaround; it does not validate a replacement bridge.

## Proposed retirement and repository deletion

Recommendation: retire the capability if preserving ordinary stock Codex behavior is the priority and a new client integration is outside the desired scope. Do not grow the task plugin into a second terminal-client identity system simply to preserve a title feature.

The following is a proposed sequence, not work executed by this research:

1. **Prepare a recovery copy.** Preserve a Git mirror and the upstream issue/PR/release information needed for historical references. Back up plugin-owned runtime state and its integration ownership registry locally. These may contain personal Task Names and must not be published as source assets.
2. **Remove distribution and presentation through dots.** A separate issue/PR removes the Herdr Tasks Provisioner and its shared YAML anchor/alias from both `herdr` and `herdr-custom` Tags in [dots.yaml](../dots.yaml). Remove only the task row and setup/correction shortcuts from all three [Herdr configurations](../configs/herdr/); retain engine/native state, Tabby labels, Git metadata, and unrelated plugins. Update [operator documentation](herdr.md), relevant installation expectations, and supersede ADR 0021 with the retirement decision while preserving historical research. Audit dependencies before removing any shared Go/Git requirement.
3. **Release the dependency-free configuration.** Verify the new Source of Truth no longer attempts to install this repository. Publish and apply that release before deleting the upstream repository. Tag deselection and dots uninstall do not reverse external Provisioner effects; explicit plugin cleanup is still necessary. See [Herdr provisioner behavior](herdr.md).
4. **Remove owned engine hooks while the executable still exists.** Preview the plugin's `remove --dry-run` using the actual recorded state and native configuration roots, inspect the result, then apply `remove`. It subtracts only unchanged owned entries. Preserve edited/unowned hooks, Herdr's native integration, Engram hooks, and trust belonging to other handlers. Reload the affected engines using their native supported lifecycle. See the pinned [producer removal instructions](https://github.com/yersonargotev/herdr-tasks/blob/add3f2f855ca7687e4be4b4b57e1400d2a06066d/docs/producers.md).
5. **Deactivate and uninstall the plugin.** Reconcile the new presentation with every running Herdr session. Stop this plugin's callbacks, clear only its owned `herdr_task_name` metadata where still present, and use Herdr's native plugin uninstall. Do not stop the shared Codex daemon or destroy terminal sessions as a cleanup shortcut. Verify no active session or hook references the plugin executable. The installed CLI exposes `pane report-metadata --clear-token` and `plugin uninstall`; exact source ownership and sequence must be captured before applying cleanup.
6. **Retire only owned state.** After verifying that no writer or configured hook remains, remove only this plugin's state directory and local checkout if that is the approved scope. Preserve the recovery copy and unrelated Herdr/Codex state. Do not delete the state registry before step 4.
7. **Delete `yersonargotev/herdr-tasks` last.** Obtain explicit authorization for permanent GitHub repository deletion against the reviewed removal result. Deleting it breaks historical pinned installs and source links in older dots releases; a current release without that dependency cannot make old releases independent retroactively. If historical reproducibility must remain, archive the repository instead. The user requested a deletion proposal, not execution, so neither action is performed here.

Likely test changes are scoped to `internal/cli/herdr_provision_integration_test.go`, `internal/cli/terminal_cockpit_acceptance_test.go`, and `internal/cli/herdr_custom_install_test.go`. This inventory follows current references, not a replacement implementation plan. Final removal must search for remaining references and distinguish historical records from active install/configuration inputs.
