# GitHub Copilot CLI configuration

Copilot CLI reads global custom instructions from
`~/.copilot/copilot-instructions.md`. The native Agent CLI Baseline does not
manage that user-owned file.

`statusline-command.sh` uses Catppuccin Mocha by default and switches its ANSI
palette to Catppuccin Latte only when the shared dots `adaptive-theme` marker is
installed and macOS light appearance is proven by `~/.config/dots/theme.sh`.

The script is copied into `~/.copilot/statusline-command.sh` and must remain
executable. Validate it with a sandboxed `HOME`; never run install/status checks
against the operator's real Copilot config.
