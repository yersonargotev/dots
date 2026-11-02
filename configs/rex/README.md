# Rex configuration

The current `rex` Tag manages `~/.config/rex/init.lua` and
`~/.config/rex/themes/Catppuccin Mocha Blue.ghostty` on macOS and Linux.
Both `desktop` and `workstation` select it. Each Managed Entry is a regular
whole-target copy, so application writes cannot change the Source of Truth.
Existing files remain Conflicts under the normal install policy; edits after
installation are Drift.

## Manual prerequisite and theme

Install [Rex](https://www.superlogical.com/rex/) yourself. There is no Rex
installer or Provisioner in dots. The dependency is optional for configuration
management: missing Rex is reported with manual guidance, while dots still
installs both files. `dots deps check --tag rex` checks the `rex` command or,
on macOS, `Rex Beta.app`; `dots deps plan --tag rex --tier generic` reports manual
handling when absent.

The imported configuration contains only portable shortcuts. Host connections,
credentials, private paths, session state and client preference databases are
excluded. The old custom Herdr prefix shortcuts are removed. Rex's unlisted
client defaults remain available unless one of the bindings below replaces them.
App-level shortcut overrides can take precedence over `init.lua`.

Rex reads `init.lua` on its server. After intentionally installing the Managed
Configuration, reload it with `rex config reload` on that server. A non-default
`REX_CONFIG` or `XDG_CONFIG_HOME` changes where Rex reads configuration; reconcile
that local preference with the managed default path yourself.

The exact reviewed Catppuccin Mocha Blue palette is kept as a theme asset, with
no adaptive or Carbonfox override. Theme selection is client-side: copying the
asset does not select it. On the Mac running the Rex client, select that client
from `rex client ls`, then import the managed file:

```sh
rex -C <client-id> do client.theme.import \
  path="$HOME/.config/rex/themes/Catppuccin Mocha Blue.ghostty"
```

This deliberate client action imports and selects the theme. It is not part of
dots installation or validation. For a remote Rex server, the client needs its
own local copy of the asset.

## Shared shortcuts

Rex and Ghostty operate independently. Ghostty uses its native windows where
Rex uses sessions; there is no Ghostty session-search equivalent.

| Keys | Rex | Ghostty |
| --- | --- | --- |
| Ctrl+Alt+h/j/k/l | Focus left/down/up/right, without wrapping | Focus corresponding split |
| Cmd+Shift+h/l | Previous/next tab | Previous/next tab |
| Cmd+Shift+k/j | Previous/next session | Previous/next window |
| Cmd+k | Session search | No override |
| Cmd+Shift+n | New session | New window |
| Cmd+t | New tab | New tab |
| Cmd+1–8 / Cmd+9 | Tab 1–8 / last tab | Tab 1–8 / last tab |
| Cmd+d / Cmd+Shift+d | Split right/down | Split right/down |
| Cmd+Shift+Enter | Maximize/restore pane | Zoom/restore split |
| Cmd+r, then h/j/k/l | Persistent exclusive resize mode, 5% steps | Persistent resize key table, 10 pixel steps |
| Escape in resize mode | Exit mode | Exit key table |
| Cmd+w / Cmd+Shift+w | Close pane/tab | Close surface/tab |

Ghostty key tables require version 1.3 or later. Its Catppuccin and Carbonfox
configurations use the same bindings. Rex resize mode ignores unrelated keys;
Ghostty retains its normal table fallback behavior. Other unlisted application
shortcuts are retained.

References: [Rex configuration](https://www.superlogical.com/rex/docs/customize/config),
[Rex shortcuts](https://www.superlogical.com/rex/docs/customize/keyboard-shortcuts),
[Rex modes](https://www.superlogical.com/rex/docs/customize/key-modes),
[Ghostty key tables](https://ghostty.org/docs/config/reference#keybind), and
[Ghostty actions](https://ghostty.org/docs/config/keybind/reference).

## Safe validation

Use a temporary home and state root. The focused tests exercise installs with
an isolated PATH, preserve pre-existing config on repeat installs, check regular
copies, and report Drift after an edit without modifying Source of Truth:

```sh
go test ./internal/manifest ./internal/cli -run 'TestRex'
rex config check configs/rex/init.lua
```

The explicit path for `rex config check` validates the file without reloading
live configuration. Ghostty validation likewise uses a temporary home and
explicit copied config; never reload or import into the operator's application
as part of verification.
