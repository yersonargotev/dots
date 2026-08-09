# Application-writable target research

Date: 2026-08-08

## Question

For every current `strategy: symlink` Managed Entry in `dots.yaml`, which
target is an **Application-Writable Target** because ordinary application
behaviour or a documented configuration command can write it; which is read
under ordinary use; and which can change only through an explicitly selected
operator output?

## Operational criterion

This note uses three deliberately narrow classifications:

- **Application-Writable Target**: a first-party source documents either an
  ordinary application/onboarding write to this exact target, or a supported
  application configuration operation that persists to it. An interactive
  configuration UI counts when the application saves its result to the target.
- **Read under ordinary use**: first-party documentation or this repository
  establishes that the program loads/reads the target, and this research found
  no documented normal writer for that exact target. This is evidence about
  ordinary use, *not* proof that no writer exists.
- **Explicit operator output**: the documented writer is a save/export action
  whose output name/location is chosen by the operator. It is not an automatic
  normal-use write to the managed path.

“No documented writer found” is stated explicitly where applicable; it is not
treated as evidence of immutability. Sources below are first-party product
documentation, official project documentation, or the repository under review.

## Complete inventory

| # | Source | Target | Classification | Evidence |
|---:|---|---|---|---|
| 1 | `configs/zsh/zshrc` | `~/.zshrc` | Application-Writable Target | The official Zsh `zsh-newuser-install` guided configuration supports `~/.zshrc`; after choices, it offers to save the file and backs up an existing one. [Zsh User Configuration Functions](https://zsh.sourceforge.io/Doc/Release/User-Contributions.html#User-Configuration-Functions) |
| 2 | `configs/zsh/zimrc` | `~/.zimrc` | Read under ordinary use | Zimfw calls this its plugin-manager configuration and uses it to build files in `$ZIM_HOME`; its documented install/update commands write modules and `init.zsh` there, not `.zimrc`. No first-party normal writer to `.zimrc` was found. [Zimfw commands](https://zimfw.sh/docs/commands/) |
| 3 | `configs/zsh/zshenv` | `~/.zshenv` | Read under ordinary use | Zsh documents `.zshenv` as a startup file it tests/loads. The documented new-user configurator handles only `.zshrc`; no normal writer to `.zshenv` was found. [Zsh modules](https://zsh.sourceforge.io/Doc/Release/Zsh-Modules.html), [new-user function](https://zsh.sourceforge.io/Doc/Release/User-Contributions.html#User-Configuration-Functions) |
| 4 | `configs/git/gitconfig` | `~/.config/dots/git/gitconfig` | Read under ordinary use | The regular native `~/.gitconfig` loads this portable baseline through its dots-owned initial block. Git reads the fragment, while supported global writes remain in the native file. [Git user manual](https://git-scm.com/docs/user-manual#telling-git-your-name) |
| 5 | `configs/dots/theme.sh` | `~/.config/dots/theme.sh` | Read under ordinary use | This repository documents it as the helper read by the adaptive-theme setup; `tmux.conf` and statusline scripts source it. It has no external owning application or documented writer in scope. [Repository adaptive-theme audit](adaptive-theme-audit.md) |
| 6 | `configs/dots/adaptive-theme` | `~/.config/dots/adaptive-theme` | Read under ordinary use | The repository documents it as an opt-in marker read by `theme.sh`; installing the tag creates it. No application writer in scope was found. [Repository adaptive-theme audit](adaptive-theme-audit.md) |
| 7 | `configs/starship/starship.toml` | `~/.config/starship.toml` | Explicit operator output | Starship documents presets with an explicit output target, e.g. `starship preset no-runtime-versions -o ~/.config/starship.toml`. This can overwrite through the symlink only when the operator selects that path; it is not a normal prompt-runtime write. [Starship preset](https://starship.rs/presets/no-runtimes) |
| 8 | `configs/tmux/tmux.conf` | `~/.tmux.conf` | Read under ordinary use | `tmux` uses the file as its user configuration. This audit found no documented tmux command that persists configuration changes to it during ordinary use. The negative result is not a claim that a script cannot edit it. [tmux manual](https://man.openbsd.org/tmux.1#FILES) |
| 9 | `configs/herdr/config.toml` (or adaptive override) | `~/.config/herdr/config.toml` | Application-Writable Target | Herdr’s onboarding writes `onboarding = false` here. `herdr config reset-keys` backs up this file and removes key sections. Both are supported configuration flows. [Herdr configuration](https://herdr.dev/docs/configuration/) |
| 10 | `configs/zellij/config.kdl` (or adaptive override) | `~/.config/zellij/config.kdl` | Application-Writable Target | Zellij’s documented Plugin API offers `reconfigure(..., save_configuration_file: true)` and `rebind_keys(..., write_config_to_disk: true)`, explicitly persisting configuration. [Zellij configuration commands](https://zellij.dev/documentation/plugin-api-commands) |
| 11 | `configs/zellij/layouts/default.kdl` | `~/.config/zellij/layouts/default.kdl` | Explicit operator output | Zellij’s `save_layout(layout_name, layout_kdl, overwrite)` saves to the user layout directory; the caller supplies the layout name and overwrite choice. It can reach `default.kdl` only when that output is selected. [Zellij layout commands](https://zellij.dev/documentation/plugin-api-commands) |
| 12 | `configs/ghostty/config.ghostty` | `~/.config/ghostty/config.ghostty` | Read under ordinary use (conditional initializer) | Ghostty creates a default only when no non-empty configuration exists. Its official source creates the file with exclusive creation and treats an existing path as already present; therefore this initialisation does not mutate an installed symlink. The `+edit-config` flow opens the chosen file in the operator’s `$EDITOR` rather than Ghostty rewriting it. [Ghostty 1.0.1 notes](https://ghostty.org/docs/install/release-notes/1-0-1), [Ghostty source](https://github.com/ghostty-org/ghostty/blob/main/src/config/edit.zig), [configuration paths](https://ghostty.org/docs/config) |
| 13 | `configs/ghostty/adaptive/adaptive-theme.ghostty` | `~/.config/ghostty/adaptive-theme.ghostty` | Read under ordinary use | The repository makes this an optional `config-file = ?adaptive-theme.ghostty` include. Ghostty documents reading included config files; the automatic default-file behaviour concerns the main config, not this named fragment. No normal writer to this fragment was found. [Repository Ghostty config](../configs/ghostty/config.ghostty), [Ghostty configuration](https://ghostty.org/docs/config) |
| 14 | `configs/atuin/config.toml` | `~/.config/atuin/config.toml` | Application-Writable Target | `atuin config set <key> <value>` changes values in `config.toml`, preserving formatting and comments; Atuin documents this exact default path. [Atuin config command](https://docs.atuin.sh/main/reference/config/), [Atuin configuration path](https://docs.atuin.sh/main/configuration/config/) |
| 15 | `configs/atuin/themes/catppuccin-mocha.toml` | `~/.config/atuin/themes/catppuccin-mocha.toml` | Read under ordinary use | Atuin reads user theme TOML files from the themes directory, while its theme documentation instructs users to add or make themes. No Atuin theme-save/export command writing this target was found. [Atuin theming](https://docs.atuin.sh/18.18/guide/theming/) |
| 16 | `configs/bat/config` | `~/.config/bat/config` | Application-Writable Target | `bat --generate-config-file` writes the default-path config. Its official source detects an existing regular file, prompts for overwrite, and then calls `fs::write`, so a confirmed operation can write through the existing symlink. [bat configuration](https://github.com/sharkdp/bat#configuration-file), [bat source](https://github.com/sharkdp/bat/blob/master/src/bin/bat/config.rs) |
| 17 | `configs/nvim` | `~/.config/nvim` | Application-Writable Target (directory) | This repository contains `configs/nvim/lazy-lock.json`; lazy.nvim documents that after **every update** it updates local `lazy-lock.json`. With this directory symlink, that normal supported plugin-manager write traverses into the repository. Neovim’s documented auto-creation of its config directory is secondary evidence only, because an installed symlink already exists. [lazy.nvim lockfile](https://lazy.folke.io/usage/lockfile), [repository lockfile](../configs/nvim/lazy-lock.json), [Neovim standard paths](https://neovim.io/doc/user/starting/) |
| 18 | `configs/zed/settings.json` | `~/.config/zed/settings.json` | Application-Writable Target | Zed’s theme selector saves the selected theme to the settings file, and its settings UI changes settings directly. The Zed FAQ identifies this exact path. [Zed themes](https://zed.dev/docs/themes), [Zed FAQ](https://zed.dev/faq) |
| 19 | `configs/zed/keymap.json` | `~/.config/zed/keymap.json` | Application-Writable Target | Zed’s keymap editor says edits made there are reflected in `keymap.json`; the same page gives this exact macOS/Linux path. [Zed key bindings](https://zed.dev/docs/key-bindings) |
| 20 | `configs/zed/themes/catppuccin-blue.json` | `~/.config/zed/themes/catppuccin-blue.json` | Explicit operator output | Zed’s Theme Builder lets the operator export JSON for local use; local theme files are then placed in the themes directory. The documentation does not identify an automatic writer to this filename, so this is an explicit export destination rather than normal-use mutation. [Zed themes](https://zed.dev/docs/themes), [Theme Builder announcement](https://zed.dev/blog/theme-builder) |

## Special analysis

### Git

Git is unambiguous: `git config --global user.name ...` and `user.email ...`
write the home `.gitconfig` according to Git’s own manual. The native target is
therefore a regular co-owned file whose initial marked block loads the portable
symlink and local extension; normal Git writes no longer expose the repository
source.

### Atuin

Atuin is also unambiguous. Its current `atuin config set` command changes
`config.toml` without requiring an editor and preserves comments/formatting.
Atuin separately stores history, keys, sessions, and logs outside this target;
those paths do not reduce the direct writer evidence for `config.toml`.

### Starship

Starship’s preset documentation gives an explicit output form:
`starship preset no-runtime-versions -o ~/.config/starship.toml`. Thus it is
not correct to call this target read-only. The write is nonetheless a selected
operator output, rather than an automatic prompt-runtime mutation, and will
traverse the symlink only if that exact path is chosen.

### Herdr

Herdr is an Application-Writable Target twice over: first-run onboarding writes
`onboarding = false`, and `herdr config reset-keys` backs up and changes the
same file. The prior premise that configuration is operator-created only by
shell redirection is therefore false. This is the direct admission-blocking
fact for issue #384.

## Uncertainties and scope boundaries

- This is a documentation/source audit as of the date above, not a proof that
  every program, extension, shell hook, or third-party script is incapable of
  writing a target.
- The Zellij Plugin API is a first-party supported capability. Whether a
  particular installed plugin exposes it is deployment-specific; that does not
  remove the supported persistent writer.
- Neovim’s documented auto-creation applies when its config directory is
  absent. With a `dots` directory symlink already installed, that initial
  creation path will normally not run. The lazy.nvim lockfile is the direct
  writer evidence here: `:Lazy update` updates the existing file inside that
  directory.
- Ghostty’s documented automatic creation is for a main default configuration
  when no non-empty configuration exists. Its source uses exclusive creation,
  so an installed symlink prevents that creation path; it is not evidence of a
  write-through candidate. It is also not evidence that Ghostty writes the
  optional adaptive fragment.
- Zed documents export of a local theme JSON but does not promise that an
  export is named `catppuccin-blue.json`; hence the explicit-output category.

## Consequences for the Agent Brief of #384

The Agent Brief must be corrected before it is restored to `ready-for-agent`:

1. It must include Herdr as an Application-Writable Target. Excluding it on
   the claim that it lacks a normal writer contradicts current first-party
   documentation.
2. The brief’s evidence inventory should account for Git, Atuin, `bat`, Zellij
   config, Zed settings/keymap, and the lazy.nvim lockfile inside the Neovim
   directory. Each confirmed Application-Writable Target must migrate away from
   `symlink` so it cannot resolve inside the Installed Repository. The audit
   does not choose its regular-file ownership contract; that remains a separate
   compatibility and lifecycle decision.
3. Starship, Zellij layouts, and the Zed local theme must be described as
   explicit-output cases, not automatic normal writers. Ghostty’s conditional
   initial creation is not a write-through candidate once the target symlink
   exists. Entries classified read-only retain an uncertainty statement rather
   than a claim of impossibility.
4. Application-writable classification governs materialization, not Entry
   Ownership. A confirmed target must be regular, while its ownership may still
   be whole-target, subset, marked-block, or seeded according to its lifecycle.
5. Explicit-output and read-under-ordinary-use entries remain eligible for
   symlinks. The dated inventory is revisable: new first-party writer evidence
   requires reclassification and migration under the invariant.

## Chosen ownership contracts

The confirmed Application-Writable Targets use these regular-target contracts:

| Target | Entry Ownership contract |
|---|---|
| Zsh `.zshrc` | Marked loader block |
| Git `.gitconfig` | Marked loader block |
| Herdr `config.toml` | TOML Subset Ownership |
| Zellij `config.kdl` | Whole-Target Ownership |
| Atuin `config.toml` | TOML Subset Ownership |
| bat `config` | Whole-Target Ownership |
| Neovim entrypoint | Whole-Target Ownership loader for separate Managed Configuration |
| lazy.nvim `lazy-lock.json` | Seeded Runtime State |
| Zed `settings.json` | JSONC Subset Ownership |
| Zed `keymap.json` | Seeded Runtime State |

Whole-Target Ownership for bat and Zellij deliberately reports supported
application rewrites as Drift while keeping the Installed Repository clean.
No KDL or line-oriented partial-ownership adapter is introduced without a
concrete co-ownership requirement.

## Coverage check

The manifest inventory in `dots.yaml` has exactly 20 entries with
`strategy: symlink`. Rows 1–20 above cover each source/target pair exactly
once; no `copy` entry is included.
