# Claude Code configuration

`settings.json` keeps Claude's app-level theme at `auto` so Claude can follow
its own light/dark appearance support while dots keeps owning only the JSON
subset. `statusline-command.sh` uses Catppuccin Mocha by default and switches
its ANSI palette to Catppuccin Latte only when the shared dots `adaptive-theme`
marker is installed and macOS light appearance is proven by
`~/.config/dots/theme.sh`.

Both settings variants request `permissions.defaultMode: auto`. Installation
retains user-owned keys and permission rules through JSON Subset Ownership.
The Carbonfox variant still selects the application's Carbonfox theme, while
the status line always uses Catppuccin, including when `theme-carbonfox` is
selected. This is the status-line exception to ADR 0020's shared preference.

The line shows the model with optional effort, Vim mode, recognized permission
mode (including `auto`), context tokens in thousands and percentage, then the
five-hour and seven-day limits as rounded percentages remaining. It accepts
both `permission_mode` and `permissionMode`. Missing values are omitted without
empty separators; zero values remain visible. Five-hour resets show hours and
minutes plus the local reset time. Seven-day resets show days and hours (hours
alone below one day) plus the local reset date. Elapsed countdowns stop at zero;
missing reset timestamps retain the `5h` / `7d` labels.

The script uses only the JSON supplied on stdin, `jq`, and local shell tools;
it does not query a Claude session or an API. See Claude's
[status-line input reference](https://code.claude.com/docs/en/statusline) and
[permission modes](https://code.claude.com/docs/en/permissions).

The script is copied into `~/.claude/statusline-command.sh` and must remain
executable. Validate it with a sandboxed `HOME`; never run install/status checks
against the operator's real Claude config.
