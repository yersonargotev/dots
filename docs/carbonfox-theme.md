# Carbonfox theme

`theme-carbonfox` is an optional, dark-only global preference. It changes the
appearance of supported applications that are already selected; it does not
select applications, add a Profile, or change any existing Profile. Selecting
the preference by itself installs only `~/.config/dots/theme-carbonfox`.

The palette is pinned to
[EdenEast/nightfox.nvim@4dacd3f](https://github.com/EdenEast/nightfox.nvim/blob/4dacd3f0185a2227bdf3b6c0975a8f0bf87cac9a/lua/nightfox/palette/carbonfox.lua).
Repository adapters and generated assets retain the upstream notice in
[`configs/themes/LICENSE-nightfox`](../configs/themes/LICENSE-nightfox).

## Select Carbonfox

Start with a dry run, then apply the same complete selection:

```bash
dots install --profile workstation --tag theme-carbonfox --dry-run
dots install --profile workstation --tag theme-carbonfox
```

Any explicit `--profile` or `--tag` flag makes the flags on that invocation the
complete selection. They are not added to the recorded Installed Selection.
Repeat every Profile and Tag you intend to keep. For example, Carbonfox can be
combined with arbitrary application Tags without using a Profile:

```bash
dots install --tag ghostty --tag zellij --tag theme-carbonfox
```

Plain `dots install` in an interactive terminal starts from the recorded
selection. Add `theme-carbonfox` in the selector when that is easier than
repeating the complete selection. The Install Plan shows only the selected
consumers and their Carbonfox variants.

The preference does not install or configure unselected applications. For
example, selecting only `theme-carbonfox` does not add Ghostty, Warp, or Zed.
Applications outside the table below do not gain a dots-owned Carbonfox theme;
they may still inherit terminal ANSI colors according to their own behavior.
In particular, this preference adds no application-level theme ownership for
OpenCode, Codex, VS Code, or Antigravity.

Use `dots install` or its interactive selector to change the preference.
`update` and `upgrade` retain removed Managed Entries by design; when an
Installed Selection exists, they reject adding or removing `theme-carbonfox`
and direct you to install. Their dry runs still show the proposed selection.
Refreshing an already selected Carbonfox setup remains supported. Explicit
preference changes stop upgrade before binary replacement; a change discovered
in incoming Source of Truth stops the later repository/application phase.

### Existing symlink configurations

Ghostty's main config, Starship's config, and Zellij's default layout are
symlinks. Changing their selected source is an explicit Conflict Resolution:
review the plan and choose **Replace** in the interactive resolver. For an
unattended run, use the existing `--backup-and-replace` flag after reviewing all
reported conflicts. dots creates a Backup Set before replacement.

An exact owned source switch cannot be skipped while committing the changed
selection: choosing Skip or Adopt returns actionable guidance before any apply.
Missing ownership evidence or an externally retargeted link remains blocked.
This applies both when adding and when removing Carbonfox; unrelated conflicts
keep their normal behavior.

### Interaction with adaptive and local settings

Carbonfox wins over `adaptive-theme` for every Managed Entry that has a
Carbonfox source override, regardless of the order of the two Tags:

```bash
dots install --profile workstation --tag adaptive-theme --tag theme-carbonfox
dots install --profile workstation --tag theme-carbonfox --tag adaptive-theme
```

Both selections resolve those entries to Carbonfox. This exception is limited
to `adaptive-theme`. Other source overrides keep their ordinary precedence, so
an unrelated override selected after Carbonfox still wins. Machine-local
extension points also keep their normal authority. In Ghostty, for example,
the adaptive include loads first, Carbonfox is applied next, and the optional
`config.local.ghostty` include remains last.

## Supported consumers

The mappings below apply on macOS and Linux unless a platform is stated. dots
does not reload live applications or sessions. After a successful install, use
the reload action in the last column.

| Selected consumer | Carbonfox mapping | Native support | Reload after install |
| --- | --- | --- | --- |
| `ghostty` | Selects `config-carbonfox.ghostty` for `~/.config/ghostty/config.ghostty` and installs `~/.config/ghostty/themes/Carbonfox`. All non-theme directives and the final local include are preserved. | No numeric floor is enforced. The native config and effective palette were tested with Ghostty 1.3.1. | Reload the Ghostty configuration or restart Ghostty. |
| `herdr` (macOS) | Selects `config-carbonfox.toml` for `~/.config/herdr/config.toml`, including the full palette and authored sidebar row colors. | No numeric floor is enforced. Native `config check` was tested with Herdr 0.9.1. | Run `herdr --session NAME server reload-config` for the relevant session or restart Herdr. |
| `tmux` | The existing `~/.tmux.conf` reads the marker and sources `~/.config/tmux/carbonfox.conf`. Catppuccin status layouts, modules, icons, and other tmux behavior remain unchanged. | No added numeric floor. The adapter is covered by tmux source and reload tests. | Run `tmux source-file ~/.tmux.conf` or restart the tmux server. |
| `zellij` | Selects Carbonfox variants for `~/.config/zellij/config.kdl` and `layouts/default.kdl`, and installs `themes/carbonfox.kdl`. The status layout changes colors only. | No numeric floor is enforced. Native `setup --check` was tested with Zellij 0.45.1. | Start a new Zellij session or restart the current session. |
| `neovim` | The existing loader reads the marker, selects `EdenEast/nightfox.nvim`, and activates `carbonfox`; `lazy-lock.json` pins the same Nightfox revision. | No added numeric floor. | Run the normal Lazy plugin sync on first use, then restart Neovim. |
| `starship` | Selects `starship-carbonfox.toml` for `~/.config/starship.toml`. Prompt modules, symbols, and layout are preserved. | No added numeric floor. | Start a new shell or reload the shell configuration. |
| `atuin` | Selects `config-carbonfox.toml` and installs `~/.config/atuin/themes/carbonfox.toml`. Non-theme settings are preserved. | No added numeric floor. | Restart the Atuin UI or start a new shell. |
| `bat` or `zsh` | Selects the Carbonfox bat config, copies `Carbonfox.tmTheme`, and runs the native cache builder when `theme-carbonfox` is also selected. | No numeric floor is enforced; installation succeeds only after native theme inventory and colored-render checks pass. | No live process reload is needed after a successful cache build; later `bat` invocations use the new cache. |
| `tuicr` | Selects `config-carbonfox.toml` and installs native and TextMate theme assets under `~/.config/tuicr/themes/`. | **tuicr 0.16.1 or newer is required and checked before apply.** | Restart or reopen tuicr. |
| `zed` | Selects `settings-carbonfox.json` for `~/.config/zed/settings.json` and installs `themes/carbonfox.json`. Extensions, fonts, icons, and other settings are preserved. | No added numeric floor. | Reload the Zed window or restart Zed. |
| `warp` | Selects `settings-carbonfox.toml` and installs the theme at the platform-native root described below. Font and other portable settings are preserved. | **Warp v0.2026.06.03.09.49.stable_00 or newer is required and checked before apply.** | Restart Warp so it rediscovers the custom theme. |
| `zsh` / fzf | Both the fzf-tab module path and direct fzf integration read the marker and apply the shared Carbonfox fzf colors. | No added numeric floor. | Start a new shell or run `exec zsh`. |
| `claude` | Selects `settings-carbonfox.json`, installs `~/.claude/themes/carbonfox.json`, and applies the shared ANSI palette in the statusline. Other Claude settings and statusline content remain unchanged. | **Claude Code 2.1.220 or newer is required and checked before apply.** | Restart Claude Code. |
| `copilot` | The existing statusline reads the marker and applies the shared ANSI palette without changing its content. | No added numeric floor. | Restart the Copilot CLI session. |

Whole-file variants are parity-tested outside their theme fields. Fonts,
keybindings, layouts, metrics, plugin behavior, status content, and icon artwork
remain the same as the corresponding baseline.

The three bold versions are installation floors, not merely versions used in
tests. dots checks them only when the corresponding Carbonfox source is in the
Install Plan. A missing, unparseable, or older native application stops the
install before Managed Entries are applied and gives an upgrade instruction.
`--skip-deps` does not skip this native-support check.

Ghostty 1.3.1, Herdr 0.9.1, and Zellij 0.45.1 are the versions used for the
native validation claims above. They are evidence versions, not minimum
versions enforced by dots. Other adapters are checked with their native parser
where available and with format, palette, and preservation tests.

### Warp's portable custom-theme path

Warp stores the selection in its native settings representation:

```toml
[appearance.themes]
system_theme = false
theme = { custom = { name = "Carbonfox", path = "carbonfox/carbonfox.yaml" } }
```

The relative path resolves beneath Warp's platform-native theme root:

- macOS: `~/.warp/themes/carbonfox/carbonfox.yaml`
- Linux: `~/.local/share/warp-terminal/themes/carbonfox/carbonfox.yaml`

[Warp commit 6ab1c167](https://github.com/warpdotdev/Warp/commit/6ab1c167ce939e57c02baa1bf1d104a1a85a2f8c)
introduced storage and loading of portable relative custom-theme paths on May
29, 2026. The enforced
[v0.2026.06.03.09.49.stable_00 release floor](https://github.com/warpdotdev/Warp/releases/tag/v0.2026.06.03.09.49.stable_00)
contains that change. Repository validation covers Warp's `SettingsValue`
shape, clean relative path, both native roots, YAML palette, and version
boundary. No Warp executable or Warp.app was available in the validation
environment, so this repository makes no local Warp runtime-validation claim;
the loader evidence comes from the upstream source and its relative-path tests.

## bat cache state and retries

When `bat` or `zsh` and `theme-carbonfox` are selected together, dots captures
the selected Source of Truth bytes for the bat config and theme before apply,
then snapshots all custom bat assets into a private staging directory and runs
`bat cache --build` there. It verifies that
Carbonfox appears in bat's theme inventory and renders ANSI-colored output
before publishing only these generated files under `~/.cache/bat/`:

- `metadata.yaml`
- `syntaxes.bin`
- `themes.bin`

Stage cleanup removes all captured and generated contents through the pinned
stage directory descriptor. The operating-system temporary-file lifecycle may
retain the resulting empty, randomized `0700` container; it contains no config,
theme, cache, or credential bytes.

Those files are application-owned local state, not Managed Entries. Other
custom themes, syntaxes, and unrelated cache files are retained. A failure
before publication leaves the prior generated files untouched. Publication is
atomic per file rather than a transaction across all three, so a failure during
or after publication can leave partial cache state.

On any Provisioner failure, already applied Managed Entries remain installed,
the previous Installed Selection remains authoritative, and dots reports the
failure. Fix the reported cause and rerun the same complete install command;
the cache builder regenerates and revalidates all three files. Do not edit or
delete the managed bat config or theme between attempts.

Activation clears inherited `BAT_*` and `XDG_*` overrides and verifies bat's
native paths under the selected home. A later interactive process with different
runtime overrides may read another config or cache. Unset or align
`BAT_CONFIG_DIR`, `BAT_CONFIG_PATH`, `BAT_CACHE_PATH`, and XDG overrides before
expecting that process to use the installed theme.

## Remove Carbonfox

Remove `theme-carbonfox` from the complete desired selection. Preview the
reconciliation first; for a workstation that has no other extra Tags:

```bash
dots install --profile workstation --dry-run
dots install --profile workstation --yes --acknowledge-selection-change --backup-and-replace
```

If `adaptive-theme` or other Tags should remain, include them in both commands.
Interactive users can instead deselect `theme-carbonfox` in plain
`dots install` and approve the selection reduction after reviewing the plan.

Removal reconciles each supported config to its remaining source: adaptive
when `adaptive-theme` is still selected, otherwise the existing baseline.
Conflict Resolution still protects externally changed targets. Consumer-scoped
theme assets remain installed while their application Tag remains selected;
without the preference marker or Carbonfox config selection they are unused.
The generated bat cache is retained as application state, while the reconciled
bat config stops selecting Carbonfox. dots does not uninstall applications,
delete application caches, or reload running applications during removal.
