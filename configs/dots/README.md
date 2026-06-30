# dots runtime configuration

`configs/dots/theme.sh` is installed for every `core` profile as
`~/.config/dots/theme.sh`. Shell-readable managed configs source it to answer one
question: should this session use Catppuccin Latte or Mocha?

`configs/dots/adaptive-theme` is installed only when the user explicitly selects
`--tag adaptive-theme`. The helper returns `latte` only when that marker exists
and macOS light appearance is proven through `defaults read -g AppleInterfaceStyle`.
Every other case — tag absent, macOS dark mode, Linux/non-macOS, missing
`defaults`, or unknown output — returns `mocha`.

This marker avoids duplicate managed targets in the Install Plan while giving
symlinked configs and copied statusline scripts a shared opt-in seam.
