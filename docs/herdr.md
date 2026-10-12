# Herdr workspace setup

The `herdr` Tag installs Herdr configuration and compatible pinned plugins on
macOS: four on Apple Silicon, and Git Tokens plus Herdr Tasks on Intel.
It is already part of the `core` and `workstation` Profiles. Select it explicitly
with `dots install --tag herdr` on a new installation, or include it in the
selection you intend to retain. Review selection reconciliation before changing
an existing installation's Tags.

## Plugins and prerequisites

| Plugin | Source | Pinned commit |
|---|---|---|
| Tab Git Tokens | `yersonargotev/herdr-tab-git` | `48fe5a66c17970a77919dc50a6ab3b6512bc7300` |
| Tabby | `yersonargotev/tabby` | `19031d668f0f8a79b58591db4ce8bc8f8846e329` |
| Pluck | `rmarganti/herdr-pluck` | `d1eacb80956c3a23ab6f7428a9e83961fb86ba28` |
| Herdr Tasks | `yersonargotev/herdr-tasks` | `66a829c4b2793cb11300d865d83245432268c42e` |

The previous Herdr configuration and Spaces/Tabby bundle were verified with
Herdr 0.9.1 on Apple Silicon macOS; Pluck declares Herdr 0.7.4 or newer but has
not been exercised in a live 0.9.1 session. Herdr, Git, Go, LazyGit, fzf, fd, bat,
curl, tar, pbcopy, open, Python 3, Node LTS through fnm, and Rust stable through
rustup are declared Dependencies. The popup tools belong directly to the `herdr`
Tag, so selecting that atomic surface does not rely on `core` or another Profile
to make its keybindings work. Node runs Tab Git Tokens. Tabby's pinned installer
downloads a checksum-verified Apple Silicon binary and falls back to
`cargo build --release --locked` if that artifact is missing, so its build
toolchain is declared as well. The pinned Tabby installer rejects Intel Macs,
so its Provisioner and Rust fallback Dependency are filtered to `arm64`. Intel
Macs receive Git Tokens and Herdr Tasks and retain their normal tab labels. Tabby is
not needed to read the active tab's label.

Pluck is also filtered to Apple Silicon. Its reviewed installer prefers the
`aarch64-apple-darwin` v0.3.1 release and uses `cargo build --release` when the
release download cannot be used. curl and tar support the release path; Rust
stable and Cargo support the fallback; pbcopy and open provide token-copy and
URL-opening behavior at runtime. Upstream v0.3.1 rejects Intel macOS before its
source-build fallback and publishes no Intel release asset, so dots does not
select Pluck or its specific Dependencies on Intel. All three shared config variants
still contain the action bindings, but without an installed Pluck action they
provide no capability on Intel.
The `herdr` Tag still selects no surface on Linux.

`dots install --dry-run --tag herdr` previews exact Provisioner commands without
running them. Each entry invokes Herdr directly with `plugin install`, a full
commit pin, and `--yes`. Herdr runs the plugin's declared build step and registers
it enabled. Repeating installation refreshes the managed checkout and may repeat
download/build work; it is not a network-free no-op.

## Task naming and activation

Herdr Tasks requires **Herdr 0.9.3 / protocol 22** and Go 1.24 or newer for its
native build. The plugin supports macOS and Linux upstream; the current dots
`herdr` Tag remains macOS-only. Installing this atomic Tag brings the reviewed
plugin, Herdr/Git/Go Dependencies, layout, and setup/correction actions together.
It does not write engine hooks during a build or silently trust them.

Start the intended Herdr session after installation, or reload its configuration
and restore task names explicitly in an existing session:

```sh
herdr --session work server reload-config
herdr --session work plugin action invoke restore --plugin yersonargotev.herdr-tasks
```

Then press `prefix+alt+s` (**Set Up Task Producers**). The plugin opens an
interactive terminal popup showing the Codex/Claude hook preview before asking
to apply it. Its equivalent qualified action is:

```sh
herdr --session work plugin action invoke setup --plugin yersonargotev.herdr-tasks
```

The initial action result acknowledges the request; it does not prove that a
client displayed the popup or that configuration or activation completed.
The popup's completed JSON reports configured hooks with
`codex_activation: "trust_review_required"` and
`claude_activation: "reload_or_restart_required"`. Reload Codex configuration
and review/trust the new command hooks through native `/hooks`; reload/restart
Claude to consume its settings. Until those steps are complete, an absent task
row is an activation condition. A successful dots Provisioner/status only records
the plugin installation result and cannot prove native hook trust.

Once activated, Codex and Claude propose a stable human Task Name through the
same semantic contract. Questions, tests, reviews, compaction, and resume retain
it; a distinct task begins a new generation. `prefix+alt+r` opens a correction
popup for the current task. The corrected name wins until the next generation.
The engine and Herdr state remain visible separately. Tabby continues to own tab
command/directory labels, and Git Tokens continues to own Git metadata.

Task/correction records and the owned integration registry are
Application-Owned Runtime State under Herdr's plugin state directory. They are
not Managed Entries or stored in dots/the plugin checkout. Native actions,
generated hooks, and direct calls share the exact captured state root. Setup adds
owned hooks alongside Herdr/user hooks and preserves unrelated settings. Before
moving or uninstalling the plugin, run its `remove` command with the same state
and native configuration roots, then reload both engines; plugin uninstall alone
does not subtract those hooks. See the pinned plugin's
[producer guide](https://github.com/yersonargotev/herdr-tasks/blob/66a829c4b2793cb11300d865d83245432268c42e/docs/producers.md)
and [operator guide](https://github.com/yersonargotev/herdr-tasks/blob/66a829c4b2793cb11300d865d83245432268c42e/docs/operator.md).

For a safe installation preview, keep dots and producer roots explicit and
separate. Set `PLUGIN_BINARY` to the reviewed installed plugin's
`.herdr/bin/herdr-tasks` executable; do not execute a guessed PATH match:

```sh
TASK_SANDBOX="$(mktemp -d)"
dots install --dry-run --tag herdr --file dots.yaml --source-root "$PWD" \
  --home "$TASK_SANDBOX/home" --state-root "$TASK_SANDBOX/dots-state"
"$PLUGIN_BINARY" setup --dry-run --state-root "$TASK_SANDBOX/task-state" \
  --codex-home "$TASK_SANDBOX/codex" --claude-home "$TASK_SANDBOX/claude"
```

The producer preview validates inputs without writing files. Applying setup to
those temporary roots tests emitted settings without activating real agents or
writing to a live home. No model request or transcript parsing is needed.
Acceptance verified this preview/setup path with existing native/user hook entries
and operator-only settings preserved, plus all three layouts through native
Herdr 0.9.3 `config check`. This is temporary-environment evidence, not a claim
that live agents have trusted or loaded the hooks.

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
TOML Subset Ownership. The baseline sets configuration `version = 1`, adds `pi`,
`agent`, and `opencode` to `commands.additional_significant`, maps the `agent`
command to `cursor` in `commands.aliases`, sets `labels.command_format` to
`directory_and_command`, and uses `  ` as `labels.separator`. Once managed,
target-only settings remain intact. An existing unmanaged file that differs from
the baseline uses normal Conflict handling; review its contents and backup before
choosing replacement. Changes to owned values also use normal Conflict handling.
Intel Macs receive the baseline but still do not install Tabby through its Apple
Silicon-only Provisioner.

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

## Approved sidebar layout

Spaces use two contiguous rows:

1. Semantic state icon and bold active tab name (`$tab_name`), without a prefix.
   Tabby presents contextual labels such as `dots > codex`; Git Tokens copies
   the active tab label. State remains Herdr's workspace-wide rollup.
2. Active-tab Git branch followed by independently styled Git categories:
   `$gitconflicted` (`!N`), `$gitadded` (`+N`), `$gitmodified` (`~N`),
   `$gitdeleted` (`−N`), `$gituntracked` (`?N`), `$gitahead` (`↑N`),
   `$gitbehind` (`↓N`), and `$gitclean` (`clean`). Zero categories are absent and
   symbols preserve their meaning without color.

Agents use three contiguous rows: native state icon/engine/state, the bold human
`$herdr_task_name`, and workspace location. The task row inherits the active
native foreground and explicitly disables dimming; it disappears until metadata
is available. Command/directory labels stay in the tab bar. The default,
adaptive, and Carbonfox variants share this layout and preserve their existing
Git category colors and other keybindings. Inline Git colors are authored for
their dark palettes; automatic light switching does not translate them.

The expanded sidebar prefers 26 columns and adapts between 18 and 36 columns.
Herdr uses distinct semantic symbols for blocked, working, done, idle, and
unknown agent state. The tab row is at the top and stays visible with a single
tab. Its right edge shows zoom state followed by a command status entry for
[hardware metrics](#hardware-metrics). The outer window title is the workspace
name alone, without the machine hostname. Panes have no gaps, outer frame, or
scrollbar. Herdr's automatic split borders remain enabled, so a real split still
has one divider while a single pane has no frame. The default, adaptive, and
Carbonfox variants own the same layout; only their palette behavior differs.

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
| `prefix+alt+m` | macmon hardware monitor |

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
macmon is not a `herdr` Dependency; the hardware monitor popup needs the
`herdr-metrics` Tag.

## Hardware metrics

The opt-in `herdr-metrics` Tag adds CPU, GPU, and RAM usage to the tab row on
Apple Silicon macOS. It is not part of any Profile; add it to the selection you
intend to retain:

```sh
dots install --profile workstation --tag herdr-metrics
```

The Tag installs Homebrew `macmon` and a dots-owned status script at
`~/.config/herdr/status.sh`. Every five seconds Herdr runs the script, which
takes one `macmon pipe -s 1` sample and prints a single unstyled line such as
`CPU 9% GPU 4% RAM 19.5GiB`. CPU and GPU use macmon's active residency ratios;
RAM is the used memory in GiB. The status entry has a two-second timeout.

The `herdr` Tag owns the tab-row entry and the `prefix+alt+m` popup in every
theme variant, so they are present even without `herdr-metrics`. A missing
script, missing `macmon`, failed sample, malformed output, or timeout makes the
script print nothing, and Herdr clears the entry instead of showing an error.
The popup explains that macmon is required and waits for Enter.

macmon is declared `optional`, so a missing or unavailable formula never blocks
Managed Configuration. The Homebrew formula supports Apple Silicon only; on
Intel Macs the Tag still installs the script, dependency checks report macmon
as missing, and the status entry stays empty. The Tag selects nothing on Linux.
Selecting `herdr-metrics` without `herdr` installs only the script and macmon,
with no visible Herdr change. Deselecting the Tag stops managing the script but
does not uninstall macmon.

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
