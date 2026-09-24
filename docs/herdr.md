# Herdr workspace setup

The `herdr` Tag installs Herdr configuration and compatible pinned plugins on
macOS: three on Apple Silicon, and the Spaces plugin on Intel.
It is already part of the `core` and `workstation` Profiles. Select it explicitly
with `dots install --tag herdr` on a new installation, or include it in the
selection you intend to retain. Review selection reconciliation before changing
an existing installation's Tags.

## Plugins and prerequisites

| Plugin | Source | Reviewed commit |
|---|---|---|
| Tab Git Tokens | `yersonargotev/herdr-tab-git` | `48fe5a66c17970a77919dc50a6ab3b6512bc7300` |
| Tabby | `yersonargotev/tabby` | `34c01f9791dd3228acae7ca378adb38e09d9fb6c` |
| Pluck | `rmarganti/herdr-pluck` | `d1eacb80956c3a23ab6f7428a9e83961fb86ba28` |

The Herdr configuration and the existing Spaces/Tabby bundle were verified with
Herdr 0.9.1 on Apple Silicon macOS; Pluck declares Herdr 0.7.4 or newer but has
not been exercised in a live 0.9.1 session. Herdr, Git, LazyGit, fzf, fd, bat,
curl, tar, pbcopy, open, Python 3, Node LTS through fnm, and Rust stable through
rustup are declared Dependencies. The popup tools belong directly to the `herdr`
Tag, so selecting that atomic surface does not rely on `core` or another Profile
to make its keybindings work. Node runs Tab Git Tokens. Tabby's pinned installer
downloads a checksum-verified Apple Silicon binary and falls back to
`cargo build --release --locked` if that artifact is missing, so its build
toolchain is declared as well. The pinned Tabby installer rejects Intel Macs,
so its Provisioner and Rust fallback Dependency are filtered to `arm64`. Intel
Macs receive the Spaces plugin and retain their normal tab labels. Tabby is
not needed to read the active tab's label.

Pluck is also filtered to Apple Silicon. Its reviewed installer prefers the
`aarch64-apple-darwin` v0.3.1 release and uses `cargo build --release` when the
release download cannot be used. curl and tar support the release path; Rust
stable and Cargo support the fallback; pbcopy and open provide token-copy and
URL-opening behavior at runtime. Upstream v0.3.1 rejects Intel macOS before its
source-build fallback and publishes no Intel release asset, so dots does not
select Pluck or its specific Dependencies on Intel. The two shared config files
still contain the action bindings, but without an installed Pluck action they
provide no capability on Intel.
The `herdr` Tag still selects no surface on Linux.

`dots plan --tag herdr --output json` previews exact Provisioner commands without
running them. Each entry invokes Herdr directly with `plugin install`, a full
commit pin, and `--yes`. Herdr runs the plugin's declared build step and registers
it enabled. Repeating installation refreshes the managed checkout and may repeat
download/build work; it is not a network-free no-op.

## Pluck actions

Pluck labels visible terminal values with short keyboard hints. The two actions
are deliberately separate:

| Binding | Action |
|---|---|
| `prefix+t` | Select a visible token and copy it through macOS `pbcopy` |
| `prefix+o` | Select a visible `http://`, `https://`, or `file://` URL and open it through macOS `open` |

Pluck has no startup hook. A fresh session reads the registered plugin. For an
existing session, reload the config and confirm the two registered actions:

```sh
herdr --session work server reload-config
herdr --session work plugin action list --plugin rmarganti.herdr-pluck
```

Then exercise `prefix+t` against a harmless visible token and `prefix+o`
against a test URL. Token selection does not open URLs, and URL selection does
not copy its result. dots does not manage custom Pluck pattern files.

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
herdr --session work plugin action invoke refresh --plugin yersonargotev.tab-git
herdr --session work plugin log list --plugin yersonargotev.tab-git --limit 3
```

An action response may mean only that execution started. Confirm `succeeded` in
the logs. Multiple plugins call their action `refresh`; always qualify its ID.

On an existing installation, the retired `herdr-space-tab-metadata` plugin may
remain registered after dots stops provisioning it. Once `$tab_name` is visible,
remove the unused plugin with `herdr plugin uninstall herdr-space-tab-metadata`.

When upgrading an existing installation from `hasuwini77.tab-git`, the old
registration remains in Herdr even though the Install Manifest no longer selects
it. After installing the new pin, clear the old metadata, confirm its action
completed, then uninstall the old plugin before refreshing the new one:

```sh
herdr --session work plugin action invoke clear --plugin hasuwini77.tab-git
herdr --session work plugin log list --plugin hasuwini77.tab-git --limit 3
herdr plugin uninstall hasuwini77.tab-git
herdr --session work plugin action invoke refresh --plugin yersonargotev.tab-git
```

Use the intended session in place of `work`. The old plugin's `$gitbranch`
token shares a name with the new plugin's token, so leaving both active can
produce competing metadata. This is a one-time Herdr registry migration;
routine dots installs do not uninstall external plugins.

## Approved Spaces layout

Each Space uses three lines, with one blank row between Spaces:

1. Semantic state icon and bold active tab name (`$tab_name`, `#cdd6f4`), without
   a prefix. The state remains a workspace-wide rollup so an inactive blocked
   agent stays visible.
2. Active-tab Git branch (`$gitbranch`, `#89b4fa`).
3. Independently styled active-tab Git status tokens:
   `$gitconflicted` (`!N`), `$gitadded` (`+N`), `$gitmodified` (`~N`),
   `$gitdeleted` (`−N`), `$gituntracked` (`?N`), `$gitahead` (`↑N`),
   `$gitbehind` (`↓N`), and `$gitclean` (`clean`). Each category has its own
   Mocha color, and zero categories are absent. The symbols retain meaning
   without color. The line stays compact because counts have no labels.

Styled text explicitly disables dimming. Agents, keybindings, and active/Navigate
background colors remain unchanged. Default and adaptive variants share the same
sidebar layout. Token foregrounds are fixed Mocha hex values: Herdr theme switching
does not translate these inline colors to Latte. The approved visual target is
dark Catppuccin; light-mode token contrast remains a limitation.

The expanded sidebar prefers 26 columns and adapts between 18 and 36 columns.
Herdr uses distinct semantic symbols for blocked, working, done, idle, and
unknown agent state. The tab row is at the bottom, disappears for a single tab,
and shows zoom state, server hostname, and 24-hour server-local time on its
right edge. Panes have no gaps, outer frame, or scrollbar. Herdr's automatic
split borders remain enabled, so a real split still has one divider while a
single pane has no frame. The default and adaptive variants own the same layout;
only their palette behavior differs.

Without plugin metadata, the corresponding tokens and empty lines disappear;
native first-tab Git values are not silently substituted. A non-Git active pane
also has no Git line. Long names may still be truncated by the sidebar width.

Each path contributes once, even with staged and unstaged changes. Priority is
conflict, deletion, addition, then modification; a rename counts as one modified
destination path. A missing upstream yields zero ahead/behind. The `clean` token
requires a successful Git status and all zero counts. Git failures and timeouts
clear the tokens instead of presenting `clean`.

## Popup workflows

Herdr's native session-modal popups keep the tiled tab and pane layout intact.
Each command opens at 80 percent from the focused pane's working directory and
closes when its process exits:

| Binding | Workflow |
|---|---|
| `prefix+alt+g` | LazyGit in the focused directory |
| `prefix+alt+t` | A scratch `$SHELL`, falling back to `/bin/sh` |
| `prefix+alt+f` | Repository file finder with bat preview |

The file finder resolves the current Git root and falls back to the focused
directory outside a repository. fd includes hidden files while excluding
`.git`, `node_modules`, and `target`; fzf previews with bat. An accepted path is
opened through `VISUAL`, then `EDITOR`, then `vi`, in that order. Cancelling fzf
exits without opening an editor or changing the underlying layout. `VISUAL` and
`EDITOR` may include ordinary whitespace-separated arguments such as
`code --wait`; the popup passes them as argv and never evaluates them as shell
syntax.

Every workflow checks its commands before starting. A missing command prints a
short error inside the popup and waits for Enter; it does not install software
or access the network. `dots deps check --tag herdr` reports missing declared
tools before launch, while the runtime checks keep `--skip-deps` behavior safe.

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

To roll back Pluck temporarily, restore the previous Herdr config (or remove the
two Pluck action entries), disable the plugin, and reload the intended session:

```sh
herdr --session work plugin disable rmarganti.herdr-pluck
herdr --session work server reload-config
```

After removing the Pluck Provisioner from the Source of Truth for a permanent
rollback, `herdr plugin uninstall rmarganti.herdr-pluck` also removes Herdr's
external plugin checkout. A later dots install will reapply any declarations
that remain in the Install Manifest.

For a temporary rollback, restore your backed-up sidebar configuration, clear
metadata, and disable the display plugin in the intended session:

```sh
herdr --session work plugin action invoke clear --plugin yersonargotev.tab-git
herdr --session work plugin disable yersonargotev.tab-git
herdr --session work server reload-config
```

Tabby can remain enabled because it owns tab labels independently. A later dots
install reapplies the declared layout and plugin installations; change the
Source of Truth if rollback should be permanent.

## Plugin refresh and limits

Tab Git Tokens refreshes immediately on focus/creation events and starts one
watcher per Herdr socket. The watcher checks only the focused Space every three
seconds, so file and branch changes update without a tab switch. Inactive Spaces
refresh on startup, focus, or the qualified `refresh` action; their file changes
may remain stale until then. The `clear` action pauses automatic updates until
`refresh` resumes them. Ahead/behind uses local tracking refs and performs no
fetch. The watcher exits if Herdr snapshots repeatedly fail; a later focus
event or manual refresh restarts it. The poll costs a snapshot, up to five local
Git commands, and one metadata write per cycle.

Pluck v0.3.1 supports dots only on Apple Silicon macOS. Its installer does not
verify the release checksum sidecar and its Cargo fallback does not pass
`--locked`; the full reviewed commit pin makes the checkout and build command
reviewable but does not turn the downloaded release payload into a
checksum-verified artifact. The plugin runs as ordinary user code and can read
visible pane contents. Custom global or project pattern files remain outside
dots ownership.

See [the initial research](herdr-active-tab-research.md) for historical upstream
evidence. The current fork's source and tests are pinned in the Install Manifest;
dots itself adds no polling process.
