# Herdr Spaces active-tab research

Research date: 2026-09-16. Installed CLI verified locally: Herdr 0.9.1 (API protocol 22). Read-only upstream inspection, temporary clones, and isolated tests; no live plugin installation or configuration changes.

Research issue: [#499](https://github.com/yersonargotev/dots/issues/499). The findings below record the initial investigation. The subsequently approved implementation is tracked by [#500](https://github.com/yersonargotev/dots/issues/500); see [current setup and approved layout](herdr.md).

Historical record: the upstream Tab Git plugin and commands below describe the
initial trial. The current `herdr` Tag uses the independently maintained
`yersonargotev/herdr-tab-git` fork; follow [current setup](herdr.md) for its pin,
token names, and migration steps.

## Diagnosis

Confirmed in the stable v0.9.1 source: `Workspace::resolved_identity_cwd_from` selects `tabs.first()` and that tab's `root_pane`, falling back to stored identity cwd. `workspace_git_refresh_items` uses this method. Built-in Git branch/status therefore describe the first tab root pane, not the active tab's focused pane. Automatic workspace name has the same identity basis. Preserve the stable workspace name while making a separate contextual line dynamic. Agent state rollup should remain workspace-wide so a blocked background agent stays visible.

Sources: [stable workspace implementation](https://github.com/herdrdev/herdr/blob/v0.9.1/src/workspace.rs#L1011), [stable Git refresh](https://github.com/herdrdev/herdr/blob/v0.9.1/src/app/git_refresh.rs#L114), [upstream Q&A #2988](https://github.com/herdrdev/herdr/discussions/2988). The Q&A answer is a community contributor's explanation; the first-tab behavior was independently verified in source.

## Configuration and extension boundary

Spaces support built-in `state_icon`, `state_text`, `workspace`, `branch`, `git_status`, and custom `$name` workspace tokens. A `tab` token available in Agents is not a built-in Spaces token. Reordering config alone does not change where Git information comes from. Use plugins to publish workspace metadata and config to render it. Plugin v1 does not offer arbitrary native sidebar UI replacement, but the metadata API is sufficient here. Metadata remains display-only, separate from semantic state and workspace identity.

Sources: [configuration](https://herdr.dev/docs/configuration/#sidebar-row-layouts), [stable plugins docs](https://github.com/herdrdev/herdr/blob/v0.9.1/docs/next/website/src/content/docs/plugins.mdx), [stable socket API docs](https://github.com/herdrdev/herdr/blob/v0.9.1/docs/next/website/src/content/docs/socket-api.mdx).

## Existing exact-match plugins

### hasuwini77/herdr-tab-git

Repository: https://github.com/hasuwini77/herdr-tab-git
Inspected commit: `83ce41a11c5cc3ab2de1452ab303f6dfb976a937` (version 0.1.0, minimum Herdr 0.8.0, Linux/macOS).

Publishes `$gitbranch` and `$gitstatus`, resolving each workspace's `active_tab_id`, a focused pane when available (otherwise a fallback pane), and `foreground_cwd` with `cwd` fallback. Status includes dirty entry count, locally known ahead/behind counts, or `clean`. A non-repository active pane clears both values. Runtime needs Node.js, Git, and Herdr; launcher checks PATH and common Node install locations. No npm/build dependencies and no runtime network calls in the inspected scripts. Calls Git with argv and writes only Herdr workspace metadata. Git status may perform Git's ordinary index refresh.

Manifest startup and manual refresh sweep all workspaces; `pane.focused`, `workspace.focused`, and `pane.created` update the currently focused workspace. No timer, no daemon. Caveats from code: changing branch, files, or cwd without one of these events leaves stale metadata; overlapping hooks have no lock or sequence protection; values have no TTL; Git errors collapse to empty output and a status-command failure can incorrectly produce `clean`; binary discovery uses `HERDR_BIN` rather than official `HERDR_BIN_PATH`. Startup hooks do not run when initially linking/enabling, so manually invoke the qualified refresh action after installation. This is a suitable bounded trial, not yet a dependable continuously updated Git monitor.

[Manifest](https://github.com/hasuwini77/herdr-tab-git/blob/83ce41a11c5cc3ab2de1452ab303f6dfb976a937/herdr-plugin.toml), [implementation](https://github.com/hasuwini77/herdr-tab-git/blob/83ce41a11c5cc3ab2de1452ab303f6dfb976a937/tab-git.js), [launcher](https://github.com/hasuwini77/herdr-tab-git/blob/83ce41a11c5cc3ab2de1452ab303f6dfb976a937/run.sh).

### szrenwei/herdr-space-tab-metadata

Repository: https://github.com/szrenwei/herdr-space-tab-metadata
Inspected commit: `c696c36256eddc6ee1983ab9f202848b84460e06` (version 0.1.0, minimum Herdr 0.8.0, Python 3.9+, Linux/macOS).

Publishes `$active_tab` (prefixed `tab:`) and `$tab_count`. Uses session snapshot, prioritizes each workspace's active_tab_id, and updates only changed workspace metadata. Startup and workspace-created/tab-created/closed/focused/renamed/moved hooks. File lock serializes refreshes and snapshot is read after locking. Uses official HERDR_BIN_PATH. Build only runs Python bytecode compilation; runtime writes its state lock and workspace metadata. No network requests or third-party Python dependencies. Subprocesses and lock waits have no timeout; it has no clear action or TTL, so disabling does not automatically clear existing metadata. Its 3 included unit tests passed locally.

[Manifest](https://github.com/szrenwei/herdr-space-tab-metadata/blob/c696c36256eddc6ee1983ab9f202848b84460e06/herdr-plugin.toml), [implementation](https://github.com/szrenwei/herdr-space-tab-metadata/blob/c696c36256eddc6ee1983ab9f202848b84460e06/sync_tabs.py).

## Local integration context

At the time of initial research, the Source of Truth configured two Space lines in both
`configs/herdr/config.toml` and `configs/herdr/config-adaptive.toml`, using native
`branch` and `git_status`. A future integration should keep both variants aligned.

Read-only `herdr plugin list` confirms `yersonargotev.tabby` is already enabled at
`34c01f9791dd3228acae7ca378adb38e09d9fb6c`. Its
[pinned README](https://github.com/yersonargotev/tabby/blob/34c01f9791dd3228acae7ca378adb38e09d9fb6c/README.md)
describes foreground-command labels with a working-directory fallback. Reuse
those labels through `$active_tab`; do not add another command detector. The
metadata plugin's `tab.renamed` hook should propagate Tabby changes, but combined
live behavior still requires verification.

The live read-only snapshot confirms per-workspace `active_tab_id`, per-tab layout
`focused_pane_id`, and pane `cwd`/`foreground_cwd`. This establishes available API
data, not that either candidate handles all inactive-workspace or multi-client
cases correctly. Raw snapshots were kept outside the repository.

## Recommendation

Try the two existing plugins together before writing a custom plugin or forking Herdr. Their token names do not collide. Both expose an action named refresh, so always qualify by plugin ID. A compact three-line layout preserves Space identity and semantic rollup, exposes the selected tab, and uses the selected pane's Git context:

```toml
[ui.sidebar.spaces]
row_gap = 0
rows = [
  ["state_icon", "workspace"],
  ["$active_tab", "$tab_count"],
  ["$gitbranch", "$gitstatus"],
]
```

No installation was executed. Proposed future trial commands, pinning reviewed revisions:

```sh
herdr plugin install szrenwei/herdr-space-tab-metadata --ref c696c36256eddc6ee1983ab9f202848b84460e06
herdr plugin install hasuwini77/herdr-tab-git --ref 83ce41a11c5cc3ab2de1452ab303f6dfb976a937
herdr plugin action invoke refresh --plugin herdr-space-tab-metadata
herdr plugin action invoke refresh --plugin hasuwini77.tab-git
```

Config rendering can be hot-reloaded; plugin startup hooks themselves do not rerun on config reload. Confirm events in plugin logs, switching two tabs with distinct repositories and focused panes. Test return to shell/non-repo, tab rename/close, restart, rapid switching, cwd changes and Git edits without focus. Ahead/behind values use locally fetched tracking refs; no fetch is performed.

For broader reliable info later, improve the existing Git plugin upstream: official binary/config env, explicit timeout/error state instead of false clean, serialize/coalesce refreshes, correct per-tab focused-pane resolution for inactive workspaces using layout.focused_pane_id, and bounded refresh/expiry behavior. Optional repo basename helps when different repos share branch names; dirty status is already useful. Keep CPU/RAM/quotas out of the initial scope unless explicitly wanted; separate existing plugins provide those but do not address this problem.

## Actual validation

- `python3 -m unittest -v test_sync_tabs.py`: 3/3 passed for metadata plugin.
- Executed real unmodified tab-git.js with real Node and isolated Git repositories through a fake Herdr executable and snapshot. Active second tab pointed to repository `two` while first/launch cwd pointed to `one`: reported `gitbranch=two` and `gitstatus=●1` as expected.
- Set the active pane foreground cwd to a non-repository temporary directory: plugin cleared both custom tokens as expected.
- No live event delivery, startup integration, multi-client behavior, or in-app visual result validated. Static analysis identifies freshness and concurrency limitations above.

## Evidence scope

Upstream master was also inspected at `e7e3dfa60e359404def46503dd105c165d02a561`, but diagnosis was rechecked on v0.9.1. Discovery searched official docs, upstream Q&A and author repositories. Marketplace listings are unreviewed; search is not exhaustive. These exact-match plugins remove the need to build a new implementation merely to test the idea.
