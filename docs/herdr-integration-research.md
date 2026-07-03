# Herdr integration research

Date: 2026-07-03

## Question

How should dots install and configure [Herdr](https://herdr.dev/) so it works with the same tmux/Zellij-oriented keymaps and shortcuts that dots already manages?

## Primary sources consulted

- Herdr install docs: <https://herdr.dev/docs/install/>
- Herdr configuration docs: <https://herdr.dev/docs/configuration/>
- Herdr keyboard docs: <https://herdr.dev/docs/keyboard/>
- Herdr GitHub release v0.7.1: <https://github.com/ogulcancelik/herdr/releases/tag/v0.7.1>
- dots manifest and config files in this repo: `dots.yaml`, `configs/tmux/tmux.conf`, `configs/zellij/config.kdl`
- Prior dots decisions from Engram: Zellij/tmux parity work for issue #261/#262, direct navigation issue #269/#270, and user-local provider issue #211/#236.

## Aligned implementation decisions for issue #297

The first implementation slice is Mac-first:

- Herdr is part of the `core` tag because it is terminal multiplexer configuration alongside tmux and Zellij.
- The managed entry is macOS-only for now: `os: [darwin]` with `brew: herdr`.
- Linux support and User-Local Provider support for Herdr raw binaries are deferred to a later issue.
- The dots-managed UX defaults intentionally keep Herdr sound enabled and popup notifications enabled. For macOS, the first config uses `ui.toast.delivery = "system"`.
- The theme uses Herdr's built-in dark Catppuccin theme, `theme.name = "catppuccin"`, which is the Mocha-style variant; `catppuccin-latte` remains the light variant.
- Keybindings use `prefix = "ctrl+a"`, tmux/Zellij-compatible prefix aliases, and direct shortcuts for pane focus (`ctrl+alt+h/j/k/l`) plus previous/next tab navigation (`ctrl+alt+[` and `ctrl+alt+]`).

## Installation facts

Herdr supports stable binaries on Linux and macOS; native Windows is still preview-only beta. The recommended direct install for Linux/macOS is:

```sh
curl -fsSL https://herdr.dev/install.sh | sh
```

The docs also document package-manager installs:

```sh
brew install herdr
mise use -g herdr
nix profile install github:ogulcancelik/herdr/v0.x.y
```

Manual downloads are published on GitHub releases. The official asset names are:

| Platform | Release asset |
| --- | --- |
| Linux x86_64 | `herdr-linux-x86_64` |
| Linux aarch64 | `herdr-linux-aarch64` |
| macOS Intel | `herdr-macos-x86_64` |
| macOS Apple silicon | `herdr-macos-aarch64` |

For direct installs, Herdr's built-in updater uses `herdr update`. Homebrew, mise, and Nix installs must be updated through their package managers instead. Herdr uses stable update channel by default on Linux/macOS; preview can be selected with `herdr channel set preview`.

As of the GitHub latest release lookup during this research, the latest stable release is `v0.7.1` (released 2026-06-24). dots' existing Linux User-Local Provider model requires checksums. Herdr's release page exposes raw binaries, not checksum sidecar assets, so the checksums below were computed locally from the downloaded v0.7.1 Linux assets with `shasum -a 256`:

| Asset | SHA-256 |
| --- | --- |
| `herdr-linux-x86_64` | `b965acaffc2c22f54b6e6c64af7cf8e98a3f4ac2622630a0599c67a4b9d8a654` |
| `herdr-linux-aarch64` | `3d757ac30c631e79dc45038c3ecc6423fe13a89f9cffa0f415aedd2c27f1576c` |

## Configuration facts

Herdr works without a config file, but custom keys/themes/UI settings live in:

```text
~/.config/herdr/config.toml
```

The default config can be printed with:

```sh
herdr --default-config
```

and saved with:

```sh
herdr --default-config > ~/.config/herdr/config.toml
```

Most running config changes can be applied with:

```sh
herdr server reload-config
```

Herdr falls back to safe defaults and shows a startup warning when config values are invalid.

## Keybinding model

Herdr has a tmux-like prefix mode. The default prefix is `ctrl+b`, and `prefix+n` means pressing the configured prefix and then `n`. Herdr's docs explicitly say users coming from tmux or Zellij already know the model.

Every binding is configurable, including the prefix. The docs show:

```toml
[keys]
prefix = "ctrl+a"
```

This matches dots' existing tmux/Zellij convention:

- `configs/tmux/tmux.conf` unbinds `C-b` and sets `prefix C-a`.
- `configs/zellij/config.kdl` binds `Ctrl a` into its tmux-like mode and mirrors the tmux workflow where Zellij has equivalent actions.

Herdr also supports multiple bindings for one action:

```toml
[keys]
next_tab = ["prefix+n", "ctrl+alt+]"]
```

This is useful for preserving prefix-first tmux parity while optionally adding direct shortcuts.

## Proposed dots-managed Herdr config

Create:

```text
configs/herdr/config.toml
```

Install it to:

```text
~/.config/herdr/config.toml
```

Recommended first version:

```toml
# dots-managed Herdr config.
# Herdr's built-in dark Catppuccin theme corresponds to the Mocha-style palette;
# the light variant is exposed separately as catppuccin-latte.
onboarding = false

[update]
channel = "stable"
version_check = true
manifest_check = true

[terminal]
shell_mode = "auto"
new_cwd = "follow"

[theme]
name = "catppuccin"

[ui]
confirm_close = true
prompt_new_tab_name = true
pane_borders = true
pane_gaps = true

[ui.toast]
delivery = "system"
delay_seconds = 1

[ui.toast.herdr]
position = "bottom-right"

[ui.toast.clipboard]
enabled = true
position = "bottom-center"

[ui.sound]
enabled = true

[keys]
prefix = "ctrl+a"

# Session/workspace
# Keep Herdr's default detach plus dots' tmux/Zellij capital-D muscle memory.
detach = ["prefix+q", "prefix+shift+d"]
workspace_picker = "prefix+w"
goto = "prefix+g"
new_workspace = "prefix+shift+n"
rename_workspace = "prefix+shift+w"
close_workspace = "prefix+alt+d"
toggle_sidebar = "prefix+b"

# Tabs: prefix-first tmux-like core plus explicit direct tab navigation.
new_tab = "prefix+c"
rename_tab = ["prefix+shift+t", "prefix+comma"]
previous_tab = ["prefix+p", "ctrl+alt+["]
next_tab = ["prefix+n", "ctrl+alt+]"]
switch_tab = "prefix+1..9"
close_tab = "prefix+shift+x"

# Panes: mirror dots tmux/Zellij intent and add direct pane focus chords.
focus_pane_left = ["prefix+h", "ctrl+alt+h"]
focus_pane_down = ["prefix+j", "ctrl+alt+j"]
focus_pane_up = ["prefix+k", "ctrl+alt+k"]
focus_pane_right = ["prefix+l", "ctrl+alt+l"]
swap_pane_left = "prefix+shift+h"
swap_pane_down = "prefix+shift+j"
swap_pane_up = "prefix+shift+k"
swap_pane_right = "prefix+shift+l"
split_vertical = "prefix+v"
split_horizontal = ["prefix+minus", "prefix+d"]
close_pane = "prefix+x"
zoom = "prefix+z"
resize_mode = "prefix+r"
copy_mode = "prefix+["
```

Notes on this proposed map:

- `prefix = "ctrl+a"` is the key decision: it makes Herdr follow dots' tmux/Zellij prefix muscle memory instead of Herdr's default `ctrl+b`.
- Herdr defaults use `detach = "prefix+q"`; dots' tmux/Zellij config uses capital `D` for detach, so the proposal keeps the Herdr default and adds the equivalent explicit `prefix+shift+d` for dots parity. Close workspace moves to `prefix+alt+d` to avoid disabling the detach alias.
- Herdr defaults use `split_horizontal = "prefix+minus"`; dots' tmux/Zellij config uses `prefix+d` for down split. The proposal keeps the documented Herdr default and adds `prefix+d` as the dots-parity alias.
- Herdr's documented tab rename default is `prefix+shift+t`; tmux commonly uses prefix comma. The proposal keeps the Herdr default and adds `prefix+comma` as a parity alias.
- Herdr's docs recommend explicit modified chords for intentional direct shortcuts and warn that some chords are terminal/OS-dependent. The implementation keeps plain keys behind prefix mode and adds only `ctrl+alt+h/j/k/l` pane focus plus `ctrl+alt+[` / `ctrl+alt+]` tab navigation.
- Herdr's docs warn that plain printable direct bindings can intercept normal typing; keep most dots parity behind `prefix+` unless a direct binding is intentionally chosen.

## Proposed `dots.yaml` entry

Add a core managed entry near tmux/Zellij:

```yaml
  - source: configs/herdr/config.toml
    target: ~/.config/herdr/config.toml
    strategy: symlink
    tags:
      - core
    os:
      - darwin
    dependencies:
      - name: herdr
        command: herdr
        brew: herdr
```

Package-manager notes:

- Homebrew is documented by Herdr, so `brew: herdr` is appropriate.
- I did not find apt/dnf/pacman package names in Herdr's official install docs; leave those unset until verified from distro package sources.
- `mise` and `nix` are documented by Herdr, but dots currently models package managers and user-local recipes rather than arbitrary mise/Nix workflows for each dependency.

## Required code change for Linux user-local install

If dots should support Linux user-local Herdr install, add a `herdr` recipe to `internal/deps/user_local.go`:

```go
"herdr": {
    archiveName: func(version, goarch string) (string, bool) {
        switch goarch {
        case "amd64":
            return "herdr-linux-x86_64", true
        case "arm64":
            return "herdr-linux-aarch64", true
        default:
            return "", false
        }
    },
    url: func(version, archive string) string {
        return fmt.Sprintf("https://github.com/ogulcancelik/herdr/releases/download/%s/%s", version, archive)
    },
    layout:      userLocalLayoutSingle,
    command:     "herdr",
    archiveType: "raw",
    binaryPath:  func(_ string, command string) string { return command },
    links:       []string{"herdr"},
},
```

However, the current user-local installer only extracts `tar.gz`/`zip`-style archives for existing recipes. Herdr's official assets are raw executable binaries. Therefore the implementation has two options:

1. Extend the user-local installer with a reviewed `raw` artifact layout/type that verifies checksum and copies the downloaded bytes to `~/.local/bin/herdr` with executable permissions.
2. Do not add user-local Herdr yet; rely on Homebrew for macOS/Linuxbrew and show manual install guidance on Linux.

Option 1 fits dots' existing user-local policy better, but it is a small code change and needs tests. Option 2 is smaller but less complete for Linux hosts without Homebrew.

## Validation plan

Before shipping:

1. Validate Herdr config syntax if Herdr provides a non-mutating config check. If not, use a sandboxed `HOME` and `HERDR_CONFIG_PATH` with `herdr --default-config` / startup smoke tests only; never write to the real `~/.config/herdr`.
2. Run manifest validation:

   ```sh
   go run ./cmd/dots manifest validate --file dots.yaml
   ```

3. Run sandbox install/status with temporary home/state roots, not the operator's real home:

   ```sh
   SANDBOX="$(mktemp -d)"
   mkdir -p "$SANDBOX/home" "$SANDBOX/state"
   go run ./cmd/dots install --file dots.yaml --source-root "$PWD" --home "$SANDBOX/home" --state-root "$SANDBOX/state" --profile core --yes
   go run ./cmd/dots status --file dots.yaml --source-root "$PWD" --home "$SANDBOX/home" --state-root "$SANDBOX/state" --profile core --output json
   ```

4. If adding user-local raw binary support, run focused dependency tests plus full CI-equivalent checks:

   ```sh
   go test ./internal/deps/...
   gofmt -l .
   go vet ./...
   go build ./...
   go test ./...
   ```

## Resolved alignment for issue #297

For the Mac-first issue #297 implementation, these questions resolved as:

1. Herdr enters `core`.
2. Raw `user_local` support is deferred; this slice uses Homebrew on macOS only.
3. Sound and notifications/toasts are on.
4. Direct shortcuts are included for pane focus and tab prev/next, alongside prefix aliases.

## Historical alignment questions

Use `grill-with-docs` before implementation if any of these are not obvious:

1. Should Herdr be part of the `core` profile immediately, or should it start behind a narrower tag/profile such as `agent-tools`?
2. Should Linux user-local raw binary support be implemented now, or should the first slice rely on Homebrew/manual install only?
3. Should dots keep Herdr sound disabled by default because this repo's terminal configs are generally quiet, or should it preserve Herdr's default sound behavior?
4. Should Herdr direct shortcuts include `alt+h/l` tab navigation to mimic tmux's current no-prefix `M-h/M-l`, use Herdr's documented safer `ctrl+alt+[/]` tab aliases, or stay fully prefix-first except for `ctrl+alt+h/j/k/l` pane focus?
