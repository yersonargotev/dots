# Herdr workspace setup

The `herdr` Tag installs Herdr configuration and compatible pinned plugins on
macOS: all three on Apple Silicon, and the two Spaces plugins on Intel.
It is already part of the `core` and `workstation` Profiles. Select it explicitly
with `dots install --tag herdr` on a new installation, or include it in the
selection you intend to retain. Review selection reconciliation before changing
an existing installation's Tags.

## Plugins and prerequisites

| Plugin | Source | Reviewed commit |
|---|---|---|
| Space Tab Metadata | `szrenwei/herdr-space-tab-metadata` | `c696c36256eddc6ee1983ab9f202848b84460e06` |
| Tab Git Status | `hasuwini77/herdr-tab-git` | `83ce41a11c5cc3ab2de1452ab303f6dfb976a937` |
| Tabby | `yersonargotev/tabby` | `34c01f9791dd3228acae7ca378adb38e09d9fb6c` |

The bundle was verified with Herdr 0.9.1 on Apple Silicon macOS. Herdr, Git,
Python 3, Node LTS through fnm, and Rust stable through rustup are declared
Dependencies. Python 3.9+ is needed by Space Tab Metadata; Node runs Tab Git
Status. Tabby's pinned installer downloads a checksum-verified Apple Silicon
binary and falls back to `cargo build --release --locked` if that artifact is
missing, so its build toolchain is declared as well. The pinned Tabby installer
rejects Intel Macs, so its Provisioner and Rust fallback Dependency are filtered
to `arm64`. Intel Macs receive both Spaces plugins and retain their normal tab
labels. Tabby is not needed to read the active tab's label.
The `herdr` Tag still selects no surface on Linux.

`dots plan --tag herdr --output json` previews exact Provisioner commands without
running them. Each entry invokes Herdr directly with `plugin install`, a full
commit pin, and `--yes`. Herdr runs the plugin's declared build step and registers
it enabled. Repeating installation refreshes the managed checkout and may repeat
download/build work; it is not a network-free no-op.

## Tabby configuration

The `herdr` Tag manages
`~/.config/herdr/plugins/config/yersonargotev.tabby/config.toml` on macOS using
TOML Subset Ownership. The baseline sets configuration `version = 1`, adds `pi`
to `commands.additional_significant`, and sets `labels.command_format` to
`command_and_directory`. Once managed, target-only settings remain intact. An
existing unmanaged file that differs from the baseline uses the normal Conflict
handling; review its contents and backup before choosing replacement. Changes to
owned values also use normal Conflict handling. Intel Macs receive the baseline
but still do not install Tabby through its Apple Silicon-only Provisioner.

## Existing sessions

A fresh Herdr session runs plugin startup hooks. Installing or reloading config
in a running session does not itself run those startup hooks. After installing,
select the intended session explicitly (replace `work` below; omit the Tabby
action on Intel):

```sh
herdr --session work server reload-config
herdr --session work plugin action invoke start --plugin yersonargotev.tabby
herdr --session work plugin action invoke refresh --plugin herdr-space-tab-metadata
herdr --session work plugin action invoke refresh --plugin hasuwini77.tab-git
herdr --session work plugin log list --plugin herdr-space-tab-metadata --limit 3
herdr --session work plugin log list --plugin hasuwini77.tab-git --limit 3
```

An action response may mean only that execution started. Confirm `succeeded` in
the logs. Multiple plugins call their action `refresh`; always qualify its ID.

## Approved Spaces layout

Each Space uses three lines, with one blank row between Spaces:

1. Semantic state icon and bold workspace name (`#cdd6f4`). The state remains a
   workspace-wide rollup so an inactive blocked agent stays visible.
2. Active tab label (`$active_tab`, `#cba6f7`), reusing Tabby's command/directory
   labels. The plugin's `tab:` prefix is preserved. The tab count is hidden.
3. Active-tab Git branch (`$gitbranch`, `#89b4fa`) and status (`$gitstatus`,
   `#f9e2af`), with an exact `clean` rule using `#a6e3a1`.

Styled text explicitly disables dimming. Agents, keybindings, and active/Navigate
background colors remain unchanged. Default and adaptive variants share the same
sidebar layout. Token foregrounds are fixed Mocha hex values: Herdr theme switching
does not translate these inline colors to Latte. The approved visual target is
dark Catppuccin; light-mode token contrast remains a limitation.

Without plugin metadata, the corresponding tokens and empty lines disappear;
native first-tab Git values are not silently substituted. A non-Git active pane
also has no Git line. Long names may still be truncated by the sidebar width.

## Ownership and rollback

The Install Manifest owns configuration as a TOML Subset. Plugin checkouts,
registry, runtime state, and plugin configuration outside the Tabby baseline
remain Herdr-owned.
Checkouts and plugin configuration live under `~/.config/herdr/plugins`; the registry
and its lock live beside that directory under `~/.config/herdr`, and plugin state
lives under `~/.local/state/herdr/plugins`. Possible build cache/data and Rust roots
are also disclosed in the Install Plan. These files are not vendored or copied into
the Installed Repository. Herdr retains plugin configuration when replacing a managed plugin
checkout. Deselection or dots uninstall does not reverse Provisioner effects or
uninstall these external plugins.

For a temporary rollback, restore your backed-up sidebar configuration, clear Git
metadata, and disable the two display plugins in the intended session:

```sh
herdr --session work plugin action invoke clear --plugin hasuwini77.tab-git
herdr --session work plugin disable hasuwini77.tab-git
herdr --session work plugin disable herdr-space-tab-metadata
herdr --session work server reload-config
```

Space Tab Metadata has no clear action; restoring the previous layout hides its
remaining display-only metadata. Tabby can remain enabled because it owns tab
labels independently. A later dots install reapplies the declared layout and
plugin installations; change the Source of Truth if rollback should be permanent.

## Known upstream limitations

Tab Git Status updates on focus/creation events, not continuously. A `cd`, branch
switch, or file edit without another focus event may leave stale values; use its
qualified refresh action when needed. Ahead/behind uses local tracking refs and
performs no fetch. Its current Git-error handling can report `clean` after a
failed status command, and overlapping hooks can race. Inactive workspaces with
multiple panes may use a fallback pane rather than their remembered focused pane.
These are upstream plugin limitations, not correctness guarantees from dots.

See [the research](herdr-active-tab-research.md) for pinned source links and
isolated tests. No custom plugin code or polling was introduced by this integration.
