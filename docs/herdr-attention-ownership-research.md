# Herdr readability, task identity, and attention ownership research

Date: 2026-10-11. Status: Research complete; ownership decision accepted in the design interview. Implementation is not started. Tracking: [dots issue #543](https://github.com/yersonargotev/dots/issues/543).

## Scope and current product decision

The user selected readability and task identity as the first delivery. The agent proposes a human-readable task name automatically, with its engine displayed on a separate line. The name must remain stable through questions, tests, and continuations, and change automatically only on a new task in the same conversation. A manual correction takes priority for the current task and is released when a new task starts. Both the name and a manual correction must survive Herdr restart and resume of the same conversation, and must not be inherited by a different conversation using the same pane. Codex and Claude are required; other engines are included only if the shared mechanism adds negligible work. Attention filtering and explicit review are later work. The user confirmed a dedicated Herdr task-identity plugin provisioned and configured through dots; [ADR 0021](adr/0021-herdr-task-identity-plugin.md) records the decision. This note investigates ownership and extension boundaries; the confirmation does not authorize runtime implementation or live configuration changes.

The previous HTML prototype is a design experiment, not proof of API requirements. In particular, its statement that the attention filter requires a Herdr core feature is corrected below: installed Herdr already supports a native Agent view projection. Explicit human review remains a separate product concept.

## Evidence and version boundaries

| Surface | Evidence inspected | Boundary |
| --- | --- | --- |
| Installed executable | `/opt/homebrew/bin/herdr --version`: `herdr 0.9.3`; `herdr api schema --output /tmp/herdr-0.9.3-attention-schema.json`: protocol `22`, schema version `1` | These commands inspect the binary without starting a session. The schema proves shapes, not every runtime behavior. |
| Release-matched Herdr source | `v0.9.3`, resolved commit `7b116c05bfda646af39d2524c54e70c751f57ee8` | Source inspection corroborates filtering, naming, event, and seen behavior; no live configuration was mutated. [Release commit](https://github.com/herdrdev/herdr/commit/7b116c05bfda646af39d2524c54e70c751f57ee8). |
| Current Herdr website | Official configuration, plugins, Agents, and socket documentation | Current docs can change. Release source and installed schema take precedence for the installed baseline. |
| dots Source of Truth | [Herdr config](../configs/herdr/config.toml), [adaptive variant](../configs/herdr/config-adaptive.toml), [Tabby policy](../configs/herdr/plugins/tabby.toml), [Install Manifest](../dots.yaml) | Repository policy is desired state; it does not prove that every live session runs the same plugin binary. |
| dots Tabby pin | `19031d668f0f8a79b58591db4ce8bc8f8846e329`, manifest `0.1.19` | Fetched exact README and manifest through GitHub because this object is absent from the adjacent local checkout. [Pinned manifest](https://github.com/yersonargotev/tabby/blob/19031d668f0f8a79b58591db4ce8bc8f8846e329/herdr-plugin.toml). |
| Adjacent `../tabby` checkout | `34c01f9791dd3228acae7ca378adb38e09d9fb6c`, manifest `0.1.17` | Older than the dots pin. Local README alone would incorrectly reject the configured `directory_and_command` and `separator` fields. [Local revision README](https://github.com/yersonargotev/tabby/blob/34c01f9791dd3228acae7ca378adb38e09d9fb6c/README.md). |
| Git metadata plugin pin | `yersonargotev/herdr-tab-git`, `48fe5a66c17970a77919dc50a6ab3b6512bc7300` | Workspace active-tab/Git metadata provider, not a task or review service. [Pinned README](https://github.com/yersonargotev/herdr-tab-git/blob/48fe5a66c17970a77919dc50a6ab3b6512bc7300/README.md). |

CodeGraph was unavailable in the deferred-tool catalog and on `PATH`. Narrow release-source reads were the fallback; no broad architecture traversal was performed. Adjacent repositories and live home configuration were left unchanged.

## Capability ownership matrix

| Prototype behavior | Existing mechanism | Capability owner / alternatives | Additional implementation? |
| --- | --- | --- | --- |
| Compact workspace rows | Configured token rows and `row_gap`; existing `$gitbranch` and category tokens | dots config; existing Git plugin remains producer | Configuration only. |
| Manual-only task-name approximation | Manual pane label on the `pane` row, or named agent display | Herdr runtime state; dots declares row layout/keybindings | No plugin needed for the approximation; it does not cover the selected isolation/restoration/correction contract. |
| Selected automatic task identity | `pane.report_metadata` tokens; native session reference as an identity input | Dedicated identity plugin, with a shared semantic producer in the agents | A producer and persistence/reconciliation contract are required; dots installs and configures it. |
| Dynamic command/directory tab label | Tabby's existing Label Policy | Existing Tabby plugin, configured and pinned by dots | Usually policy only; does not identify the task inside `codex`. |
| Current activity line | Metadata token, or `terminal_title_stripped` as an explicitly weaker fallback | Agent integration; Herdr renders it | Configuration renders an existing title; trustworthy activity needs a producer. |
| Native attention ordering | `ui.agent_panel_sort = "priority"` | dots config and Herdr renderer | Configuration only. |
| Attention-only Agent list | `agent.view.set`, with status/seen/token filters; `agent.view.clear` restores the view | Bounded action or a Herdr plugin; Herdr renders/navigates | Already supported in installed 0.9.3; does not require core changes. |
| Focus a result | `agent.focus` / normal navigation | Herdr | Already supported; this also affects native seen state. |
| Focus presentation | Existing zoom and collapsed-sidebar options | Herdr; dots binds/configures | Exact one-button combination needs a verified action path; no core extension assumed. |
| Explicit “Mark reviewed” | A plugin-owned review record/token and declared action | Dedicated review owner if this product is wanted | Native seen-on-focus is insufficient. A popup/action can implement it without replacing the sidebar. |
| Arbitrary custom banner, card controls, native non-terminal panel | Not a plugin v1 UI surface | Herdr core if that exact appearance is mandatory | Core changes needed for arbitrary native chrome, not for metadata/filter behavior. |
| Font density | Outer terminal font settings | Ghostty config in dots | Configuration; Herdr row spacing and terminal font are separate controls. |

Rows, custom `$name` tokens, per-agent row overrides, color styles, sidebar collapse, and native priority sorting are documented configuration controls. The existing adaptive config still applies fixed Mocha token foregrounds to its sidebar rows; its light/dark background overrides do not by themselves make those authored foregrounds adaptive. [Configuration](https://herdr.dev/docs/configuration/#sidebar-row-layouts), [config reference](https://herdr.dev/docs/config-reference/), [adaptive Source of Truth](../configs/herdr/config-adaptive.toml).

## What plugin v1 can and cannot own

Plugins are executable workflow packages. They call the CLI/socket API, declare actions/event hooks/panes in a manifest, and can use terminal popups. Native non-terminal plugin UI and runtime action registration are outside v1. Herdr supplies separate config/state directories; plugin source checkouts should not store mutable durable state. Startup hooks are one-shot restore commands, not supervised daemons. [Official plugin contract](https://herdr.dev/docs/plugins/).

This means a small plugin can own task metadata, a popup picker, an explicit review action, and a saved view preference. Herdr can render the resulting tokens and native filtered list. dots should distribute configuration, dependencies, and pinned Provisioners through the Install Manifest; task/review records are application-owned runtime state, not Managed Entries or Installation Metadata. [dots domain](../CONTEXT.md), [Install Manifest](../dots.yaml).

### Native filtering is already present

Installed schema includes `AgentViewSetParams`, filter fields `status`, `seen`, IDs, `state_change_seq`, and custom token fields. Release source filters the native entries and applies stable sort keys. The projection is transient and source-owned; `agent.view.clear` can clear only a matching source. A source prefixed `plugin:` must identify an installed enabled plugin. A bounded invocation with an appropriate non-plugin source can also apply a projection; persistence across restart would be a separate owner decision. [Release evaluator](https://github.com/herdrdev/herdr/blob/v0.9.3/src/app/agent_view.rs), [set/clear handlers](https://github.com/herdrdev/herdr/blob/v0.9.3/src/app/api/agent_view.rs).

Current docs explain that the projection affects expanded/collapsed sidebar, mobile Agent list, mouse targets, indexed focus, and next/previous navigation; it does not change `agent.list`, notifications, or global attention counts. Consequently it can realize an Attention view but is not a wholesale sidebar replacement. [Agent view queries](https://herdr.dev/docs/socket-api/#agent-view-queries).

### Seen is not reviewed, and completion is not verified success

Release code defines native `done` as `Idle` plus `seen = false`. `agent.focus` calls `mark_active_tab_seen()`. This provides a native unread-like indicator that disappears on viewing, not an explicit human approval action. [Status mapping](https://github.com/herdrdev/herdr/blob/v0.9.3/src/app/agent_view.rs), [focus handler](https://github.com/herdrdev/herdr/blob/v0.9.3/src/app/agents.rs), [documented rollups](https://herdr.dev/docs/agents/#state-rollups).

`AgentInfo.completion_seq` identifies an idle transition that completed work independently of viewing. It is a useful future completion-generation input, but neither it nor `idle` proves that the assigned task succeeded, validation passed, or a human reviewed the result. No dedicated public reviewed/acknowledge endpoint appeared in the inspected installed request schema. [Agent response contract](https://github.com/herdrdev/herdr/blob/v0.9.3/src/api/schema/agents.rs).

If explicit review is later selected, its owner must key acknowledgements to a native conversation/task identity and completion generation, rather than only a reusable pane ID. It must define behavior for a new prompt, new completion, process replacement, restored session, and missing identity. Publishing a separate review token preserves Herdr's semantic lifecycle authority. This is a design inference, not an implemented contract.

### Metadata, events, and restoration

Metadata is presentation-only; it can supply title, visible agent label, state labels, and arbitrary tokens without claiming semantic lifecycle authority. Tokens are patches: strings set, null clears, and optional TTL expires updated keys. Source sequence handling and presentation guards are available, but token reporters remain responsible for refresh/clearing. Tokens are not restored after a cold server restart. [Metadata contract](https://herdr.dev/docs/socket-api/#agent-state-reporting), [release metadata handler](https://github.com/herdrdev/herdr/blob/v0.9.3/src/app/api/panes.rs).

Manifest hooks are narrower than socket subscriptions. In 0.9.3 they include focus, creation/closure, detection, and `pane.agent_status_changed`, but exclude `pane.updated`, `workspace.metadata_updated`, `pane.output_changed`, and `layout.updated`. Do not design a metadata consumer assuming every event can launch a manifest hook. A subscription-owning runtime has its own reconnection/reconciliation costs. [Release event contract](https://github.com/herdrdev/herdr/blob/v0.9.3/src/api/schema/events.rs), [hook dispatch](https://github.com/herdrdev/herdr/blob/v0.9.3/src/app/api/plugins/runtime.rs).

Manual agent names have uniqueness and restricted 1–32-character lowercase identifiers. Agent token display resolves `display_agent` before manual agent name, then engine, then title. A manual pane label can therefore preserve an engine line while showing a separate task row; renaming an agent may replace the visible engine label. Both agent names and pane labels appear in saved snapshots, but writing a snapshot field does not prove that every cold restore path reapplies it; see the restore qualification below. [Naming](https://github.com/herdrdev/herdr/blob/v0.9.3/src/app/agents.rs), [sidebar precedence](https://github.com/herdrdev/herdr/blob/v0.9.3/src/client/shell/agent_sidebar.rs), [snapshot contract](https://github.com/herdrdev/herdr/blob/v0.9.3/src/persist/snapshot.rs).

## Existing plugins and alternatives

### Stable task-name producer feasibility

The release-matched Codex integration script (integration version 8) accepts a `SessionStart` input and reports `pane.report_agent_session` with the native session ID. It requires a transcript path to validate input but does not publish a task title. The Claude script (integration version 10) also handles `SessionStart`, rejects subagent inputs, and reports native session ID and transcript path. Neither script publishes task-name metadata or derives a stable name from the first request. Both are integration-managed files explicitly intended to be replaced on reinstall; custom hooks must coexist beside them. [Codex source](https://github.com/herdrdev/herdr/blob/v0.9.3/src/integration/assets/codex/herdr-agent-state.sh), [Claude source](https://github.com/herdrdev/herdr/blob/v0.9.3/src/integration/assets/claude/herdr-agent-state.sh).

`terminal_title_stripped` is already a shared rendering input. It normalizes an OSC title and removes a leading activity glyph, but it does not establish that the producer supplied a stable task name. A changing, missing, or generic terminal title cannot satisfy the chosen stable Task Identity contract. [Release token renderer](https://github.com/herdrdev/herdr/blob/v0.9.3/src/ui/sidebar/tokens.rs), [terminal title semantics](https://herdr.dev/docs/configuration/#sidebar-row-layouts).

A shared instruction/helper that asks either agent to publish a task token after understanding the first request is a plausible smaller alternative to a new plugin. The Herdr API accepts the same presentation report regardless of engine. This is an architectural inference, not evidence that either engine guarantees instruction execution, first-request availability, reset detection, or title publication. Verify those guarantees before choosing it. If manual correction is required, define a protected user override so a later report cannot overwrite it. If cold restart or conversation replacement must be reliable, the producer must bind metadata to the native conversation, clear stale tokens, and rehydrate the stable name; merely setting a long TTL does not provide those semantics.

### Native identity helps, but persistence needs an owner

Herdr's `agent.name` is stronger than a raw pane label while running: `set_agent_name` associates its owner with the effective agent and native session reference when available. `reconcile_agent_name_owner` clears an ordinary alias when the engine or known native session changes, with a special exception for managed-agent launches. A manual pane label is saved as pane/terminal presentation and is not a task-generation record. [Native name ownership](https://github.com/herdrdev/herdr/blob/v0.9.3/src/terminal/state.rs#L2110), [owner reconciliation](https://github.com/herdrdev/herdr/blob/v0.9.3/src/terminal/state.rs#L2353).

Important restore qualification: the ordinary alias is serialized, but the normal cold native-resume path explicitly skips `(Some(_), None)` where there is no managed-agent kind. Ordinary aliases are restored for imported live terminals and an unavailable-terminal fallback; those paths do not prove ordinary aliases survive normal cold restart. Managed-agent aliases follow another path. Thus native `agent.rename` alone does not satisfy the requested general cold-restart guarantee. [Normal native resume](https://github.com/herdrdev/herdr/blob/v0.9.3/src/persist/restore.rs#L600), [imported-terminal branch](https://github.com/herdrdev/herdr/blob/v0.9.3/src/persist/restore.rs#L700).

A shared instruction using `agent.rename` only on new tasks is still a possible publication surface if a unique lowercase slug of at most 32 characters is acceptable, the rename happens after the native session is anchored, and an explicit persistence/rehydration helper owns the correction. Existing `display_agent` can mask that name. Native alias state does not distinguish automatic naming from a manual correction or represent a task generation. A manual correction that resets at the next new task could simplify the lock model; a correction protected across new tasks needs a separate policy. Neither a bare instruction nor pane naming alone proves the full chosen lifecycle contract.

For a human-readable `$task` token, the chosen restart and replacement guarantees require persistent conversation/task data and a rehydration/reset owner. That can be a small helper with engine hooks and shared instructions in dots, or a focused Herdr plugin with declared actions and state storage. The packaging choice should follow the amount of lifecycle and action behavior required; automatic naming alone does not justify a plugin.

Keep semantic naming separate from deterministic persistence. The agent understands whether a request begins a new task and proposes its name; a small identity component can validate the reported native conversation, save the task generation and manual correction, and publish/reconcile presentation. A Herdr plugin is a useful package for that component because startup, declared actions, and a state directory already exist. The examined requirements do not imply an always-on daemon: bounded startup, publication, correction, and supported session hooks are candidates. Their ordering must be verified so startup cannot republish a stale record before the current conversation is established.

The exact Tabby `0.1.19` pin supports `command_only`, `command_and_directory`, and `directory_and_command`, plus a configurable separator. Its default directory-first label explains labels such as `packy-catalog  codex`. Tabby owns stable foreground-command/directory labels, manual tab locks, and session runtime coordination; it deliberately observes only the focused tab. A task-title producer would introduce a different lifecycle and should not silently convert Tabby into a cross-agent review service. [Pinned README](https://github.com/yersonargotev/tabby/blob/19031d668f0f8a79b58591db4ce8bc8f8846e329/README.md), [pinned runtime ADR](https://github.com/yersonargotev/tabby/blob/19031d668f0f8a79b58591db4ce8bc8f8846e329/docs/adr/0010-adopt-session-owned-runtime-for-herdr-0.8.md).

The Git plugin already supplies active-tab `$tab_name` and workspace Git category tokens, following that tab's effective foreground directory. Keep that responsibility cohesive; adding agent task review there would conflate unrelated freshness, persistence, and identity requirements. [Pinned Git provider](https://github.com/yersonargotev/herdr-tab-git/blob/48fe5a66c17970a77919dc50a6ab3b6512bc7300/README.md).

| Alternative | Benefit | Cost / limitation | Appropriate trigger |
| --- | --- | --- | --- |
| dots config + manual pane/task name | Smallest reversible visual experiment, no new process or package | Does not alone satisfy automatic new-task naming and conversation replacement isolation | A readability prototype or manual-only contract. |
| Existing Tabby policy change | Reuses current label contract | Command/cwd distinguish location and tool, not multiple tasks in the same repo | Requested improvement is specifically tab labeling. |
| Narrow task-presentation plugin + dots pin/config | Separates task identity from tab/Git data; owns restore and publication | New maintained package and lifecycle; needs a trustworthy producer | Automatic stable task identities or explicit review are requirements. |
| Agent-specific integrations publish metadata | Close to authoritative task/conversation events | Codex and Claude adapters and restore behavior must be verified | Supported inputs supply the identity/title needed by both required engines. |
| Shared instruction/helper publishes a task token | Uses existing API and can serve Codex and Claude without a new package | Instructions alone are not a lifecycle guarantee; manual override, reset, and restore rules need evidence | Lightweight publication satisfies the final identity contract. |
| Shared instruction + native alias + persistence helper | Reuses live native conversation ownership and API publication | Cold alias restore requires rehydration; slug/uniqueness, precedence, managed-agent exception, no native auto/manual provenance | Short aliases plus helper-owned correction satisfy the final contract. |
| Herdr core contribution | Can change arbitrary native layout/controls | Upstream coordination and release dependency | Exact native chrome is required beyond rows, views, actions, and terminal popups. |

## Existing Auto Title follow-up

The user subsequently asked whether `kryptamine/herdr-auto-title` already
satisfies this contract and could avoid another plugin. The
[source- and test-backed comparison](herdr-auto-title-comparison-research.md)
distinguishes its substantial contextual pane/tab naming from stable
conversation/task identity. Reuse is a viable alternative for a relaxed
contextual-title product; it is not equivalent fulfillment of the selected
new-task, correction, and conversation-replacement rules. Avoid two independent
tab-name writers. This follow-up neither installs a candidate nor authorizes
implementation of the accepted plugin boundary.

## Accepted ownership and design vocabulary

The user confirmed the dedicated plugin alternative after comparing configuration-only, expanding Tabby, and a shared helper. This is an ownership decision, not implementation evidence. The plugin owns task identity, saved task names, protected manual corrections, and conversation-aware restoration; Codex and Claude supply the semantic task name through one shared contract; dots owns portable layout/preferences, dependency declarations, and a pinned Provisioner. Herdr keeps rendering, navigation, and semantic lifecycle state. No always-on daemon is implied by this decision. See [ADR 0021](adr/0021-herdr-task-identity-plugin.md).

For this proposed integration, a **Task Name** is the human-readable description shown in the Agent sidebar. A **Task** is a coherent unit of user intent that may span multiple prompts, questions, tests, and continuations inside one agent conversation. A new task may begin without creating a new conversation. A **Conversation Identity** is the engine-qualified native conversation reference used to distinguish resumptions from a different occupant of the same pane. The Task Name is presentation, not that identity. A **Manual Correction** overrides the automatic Task Name only for its current task. These are design terms for the future plugin, not additions to the dotfiles distribution glossary; [CONTEXT.md](../CONTEXT.md) remains the single canonical glossary for this repository.

### Settled interview decisions

| Decision | User selection |
| --- | --- |
| First delivery | Readability plus task identity; attention/review deferred. |
| Name semantics | Stable through the current task; current activity is separate. |
| Producers | Codex and Claude required; other engines only if the shared mechanism adds negligible work. |
| Initial name | Automatic from the understood request, with manual correction. |
| Task transition | Automatic new name only when a distinct task begins, including inside the same conversation. |
| Resume / replacement | Name and manual correction survive restart/resume of the same conversation; a different conversation in the same pane must not inherit them. |
| Manual priority | Protected during the current task; released at a new task. |
| Visual form | Preserve engine plus a human title on its own line. |
| Ownership | Dedicated Herdr plugin, installed at a pinned revision and configured by dots; keep Tabby labels-only. |

### Remaining implementation evidence

These items do not reopen the ownership choice, but must be resolved before an implementation contract claims readiness:

- Verify the smallest producer/entrypoint for Codex and Claude against their actual supported host versions. Herdr's existing integration scripts provide native conversation references, not the chosen task name. Preserve integration-managed scripts and install any additional reporting beside them.
- Make a new-task signal explicit in the shared producer contract. The plugin must not infer semantic task transitions from cwd, process names, terminal titles, or native idle transitions.
- Verify storage keys, missing native identity, conversation replacement, task-generation ordering, manual correction races, clearing, restore timing, and bounded failures. Define a safe display fallback when no validated task identity exists; absence must not be presented as restored success.
- Verify publication into arbitrary existing panes without stealing focus or overwriting another metadata owner. Package only the lifecycle and actions needed; do not add a resident runtime by default.
- Test startup, close, restart, same-conversation resume, different-conversation replacement, same-task continuation, new task, and manual correction using temporary Herdr home/session roots. Source/schema inspection is not substitute runtime acceptance evidence.
- Check selected Source of Truth pins and actual running server/plugin versions separately, and select an independently meaningful installation capability under [ADR 0018](adr/0018-atomic-capability-tags-and-profile-presets.md) before adding the plugin to the Install Manifest.
- If attention/review is selected later, first specify seen versus explicit acknowledgement and whether actions/popups/native projections satisfy interaction needs. Native filtering already exists; exact custom sidebar chrome is a separate core-UI question.

No runtime implementation, plugin upgrade, dotfiles installation, adjacent repository edit, or live UI mutation was performed by this research.
