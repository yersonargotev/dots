# Herdr workspace setup

The `herdr` Tag installs Herdr configuration and compatible pinned plugins on
Apple Silicon macOS: three compatible plugins. macOS Intel is unsupported by dots.
It is already part of the `core` and `workstation` Profiles. Select it explicitly
with `dots install --tag herdr` on a new installation, or include it in the
selection you intend to retain. Review selection reconciliation before changing
an existing installation's Tags.

## Retiring an existing custom installation

`herdr-custom` is retired from the offered Source of Truth. Catalog, completion,
the Install Manifest, dependency installation and Provisioners no longer offer
it. Explicit custom requests, including old manifests requesting the custom
provider or Provisioner, fail with retirement guidance before provisioning.
There is no replacement alias, implicit official installation, or uninstall.

Historical Installed Selections, contribution evidence, Provisioner inventory,
Dependency Installation Metadata and completed immutable receipts remain
readable. `dots installed` can inspect this history. Selection-aware read-only
commands using stale recorded custom intent return exit 1 with guidance; they
do not repair metadata. Mutating commands also stop before applying that intent.
Pending custom provider journals remain preserved and require manual remediation;
dots will not resume them, download custom artifacts, publish executables, repair
captured projections or acknowledge completion. Preserve their `dependencies.d/`
operation directories and private staging alongside public paths for inspection.
Do not delete a journal to bypass diagnosis. Recovery for retained providers
continues under [ADR 0021](adr/0021-recover-user-local-provider-receipts.md).

### Complete retained selection

First inspect the saved Profiles, extra Tags and exclusions with `dots installed`.
Submit the **complete** intent you want to retain, dropping custom. Repeat every
retained Profile, extra Tag and exclusion. Profiles such as `core` and
`workstation` expand to official `herdr`, so continue excluding it when preserving
the custom environment. Do not copy a workstation example over a different saved
selection. For an installation whose intended retained selection is exactly
`workstation` with official Herdr excluded:

```sh
dots plan --profile workstation --exclude-tag herdr --output json
dots install --dry-run --profile workstation --exclude-tag herdr
dots install --profile workstation --exclude-tag herdr \
  --yes --acknowledge-selection-change
```

On Linux, use the same complete-selection rule and keep official `herdr`
excluded from such Profiles. Retirement installs nothing in its place; the
retained official integration selects no Linux surface. On macOS ARM64, later
adoption of official Herdr remains an explicit operator decision. Other platforms
gain no official provisioning support through custom retirement. Existing
platform restrictions, official theme variants, plugin pins and opt-in
`herdr-metrics` remain unchanged.

Interactive reductions require the existing separate selection acknowledgement.
Non-interactive reductions require both `--yes` and
`--acknowledge-selection-change`. If no capability should remain selected, use
`--clear-selection` with its existing safeguards: type `clear` interactively, or
supply both acknowledgement flags non-interactively (also for a dry run).
A dry run, decline, error or cancellation preserves the prior Installed
Selection; only successful terminal completion records retained intent.

Removed custom declarations and source files provide no retirement authority.
Historical custom contributions recovered from inventory stay `retain` /
`manifest-evolution-report-only`, including changed or unproven targets. Their
ownership evidence remains historical. Completed custom dependency receipts and
Provisioner inventory produce applicable `retained-external-state` reports.
Installed packages, bundles, launchers, Provisioner effects, plugin configuration,
sessions and history remain in place. This policy does not bypass normal
acknowledged retirement for unrelated, still-declared Managed Entries.

### Later official adoption and local cleanup

Custom and official Herdr share `~/.config/herdr`, including configuration,
plugin registry and plugin configuration. Also preserve the actual application
state, sessions, history, caches and any paths selected by `HERDR_*` or `XDG_*`
environment overrides. Do not assume that a different binary or command name
isolates this environment. Before later adoption, inspect and preserve it, verify
plugin compatibility, and use matching client/server versions. A new client does
not replace a running server; switching a symlink does not migrate or downgrade
state. Session restarts and state migration require separate operator authority.

The dots custom-only paths are `~/.local/bin/herdr-custom` and versioned bundles
under `~/.local/opt/herdr-custom/<version>/`. Inspect the actual launcher target,
receipt evidence and references before deciding that a path is custom-only.
An unrelated PATH shadow, user-modified launcher or unproven bundle must not be
assumed to belong to dots. Shared `~/.config/herdr`, plugins and application data
are **not** custom-only cleanup targets. Manifest retirement and acknowledged
deselection remove none of these paths. Any cleanup is a separate, explicitly
authorized procedure after preserving the environment and checking live sessions.
Official installation must likewise be explicitly requested on a supported
platform. Switching different sources on a shared partially owned target remains
a separate concern tracked in #593; this transition does not fix or require it.

Reusable prompt for a later Codex workstation session:

> Inspecciona mi instalación real de Herdr, los launchers y bundles, los recibos,
> los journals pendientes y la Installed Selection guardada. `herdr-custom` fue
> retirado de dots. Conserva la configuración compartida de `~/.config/herdr`,
> plugins, sesiones, historial y estado de la aplicación, incluidos los paths
> efectivos de HERDR/XDG. Explica una transición concreta para mi plataforma con
> la selección completa retenida, sus exclusiones y la confirmación de reducción.
> No supongas que Herdr oficial está instalado ni que #593 está resuelto. La
> instalación oficial, la limpieza local y cualquier reinicio o migración de
> sesiones son decisiones separadas: presenta los paths y efectos concretos y
> solicita mi autorización explícita antes de eliminar artefactos custom o de
> reiniciar o migrar sesiones. No ejecutes esos pasos durante la inspección.

## Plugins and prerequisites

| Plugin | Source | Pinned commit |
|---|---|---|
| Tab Git Tokens | `yersonargotev/herdr-tab-git` | `21b2baa278f571fafa7a1247851a17754fbeedf0` |
| Tabby | `yersonargotev/tabby` | `19031d668f0f8a79b58591db4ce8bc8f8846e329` |
| Pluck | `rmarganti/herdr-pluck` | `d1eacb80956c3a23ab6f7428a9e83961fb86ba28` |

The shared Git Tokens pin is version 1.4.1 and requires Herdr 0.9.5 or newer.
After a plugin update, the next hook or explicit refresh replaces the previous
installation's watcher, including legacy watchers that did not renew pane
projects. This keeps Agent project labels from expiring without focus events;
no `$project` configuration change is needed. See the
[upstream fix](https://github.com/yersonargotev/herdr-tab-git/pull/12).

The project and Space/Git metadata integration was verified with official
Herdr 0.9.5 on macOS ARM64. Historical custom verification is not an
offered installation path; the transition above preserves that existing state.
The previous Spaces/Tabby bundle was verified with Herdr 0.9.1 on Apple Silicon
macOS; Pluck declares Herdr 0.7.4 or newer but has not been exercised in that
live session. Herdr, Git, Go, LazyGit, fzf, fd, bat,
curl, tar, pbcopy, open, Python 3, Node LTS through fnm, and Rust stable through
rustup are declared Dependencies. The popup tools belong directly to the `herdr`
Tag, so selecting that atomic surface does not rely on `core` or another Profile
to make its keybindings work. Node runs Tab Git Tokens. Tabby's pinned installer
downloads a checksum-verified Apple Silicon binary and falls back to
`cargo build --release --locked` if that artifact is missing, so its build
toolchain is declared as well. The pinned Tabby installer rejects Intel Macs,
so its Provisioner and Rust fallback Dependency are filtered to `arm64`. Intel
Macs receive Git Tokens and retain their normal tab labels. Tabby is
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
`agent`, and `yazi` to `commands.additional_significant`, maps the `agent`
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

## Official sidebar layout

The `herdr` Tag uses three contiguous Space rows:

1. Semantic state icon and bold active tab name (`$tab_name`), without a prefix.
   Tabby presents contextual labels such as `dots > codex`; Git Tokens copies
   the active tab label. State remains Herdr's workspace-wide rollup.
2. Active-tab Git branch (`$gitbranch`, without a symbol prefix) in bold mauve.
3. The same visible status categories as the Starship prompt: `$gitstaged`
   (`+N`, green), `$gitworkmodified` (`!N`, yellow), `$gituntracked` (`?N`, red),
   and either `$gitforward` (`⇡N`, teal), `$gitbackward` (`⇣N`, peach), or
   `$gitdiverged` (`⇕⇡N⇣N`, mauve). Zero categories are absent. Clean repositories
   retain `$gitclean` (`clean`) in the original muted color. Conflict, deletion, and rename
   markers are omitted because Starship's configured format omits them too.

Agents use two contiguous content rows: native state icon/engine/state, then
pane-scoped project (`$project`). The default, adaptive, and Carbonfox variants
share this layout and use their corresponding Starship palette roles.
Inline Git colors are authored for their dark palettes; automatic light switching does not translate them.

For the official Agent location, the plugin publishes the current checkout-root
basename: an agent in `herdr-tab-git` within Workspace `dots` shows
`herdr-tab-git`. Repository subdirectories retain that root name, and a worktree
uses its own root basename. Outside Git or when root lookup fails, the reported
directory basename is used; no directory clears the label. There is no Workspace
or title fallback, and identical basenames intentionally produce identical short
labels. Values are independent per agent pane, including panes in the same tab.
The plugin updates through events and nominal three-second polling, including
inactive tabs and Workspaces.

Independent Spaces are separated by one blank terminal row (`row_gap = 1`) in
the `herdr` Tag and all theme variants. This separates
entries without adding inner padding to each Space. Native worktree groups may
keep parents and children packed. Agent entries also have one blank row between them (`row_gap = 1`); their
two content rows remain contiguous.

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
also has no Git rows. Long names are truncated with an ellipsis at the sidebar
width; they do not wrap onto another row.

Prompt counts distinguish the index from the worktree: an index addition,
modification, or type change contributes to staged; a worktree modification or
intent-to-add path contributes to modified. A path with both changes contributes
to both counters, as in Starship. The plugin still publishes its original
path-category tokens for other consumers. A missing upstream yields no arrows.
Git failures and timeouts clear the tokens; a non-Git pane has no Git rows.

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

The official `herdr` Tag selects Pluck v0.3.1 only on Apple Silicon macOS.
Its installer does not
verify the release checksum sidecar and its Cargo fallback does not pass
`--locked`; the full reviewed commit pin makes the checkout and build command
reviewable but does not turn the downloaded release payload into a
checksum-verified artifact. The plugin runs as ordinary user code and can read
visible pane contents. Custom global or project pattern files remain outside
dots ownership.

See [the initial research](herdr-active-tab-research.md) for historical upstream
evidence. The current fork's source and tests are pinned in the Install Manifest;
dots itself adds no polling process.
