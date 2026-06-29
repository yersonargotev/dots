# Aliases. Guarded where they depend on an optional tool.

# Prefer eza for common directory listings when available. eza is an alias-only
# slice with no managed config file; see configs/eza/README.md for the rationale.
if command -v eza >/dev/null 2>&1; then
  alias ls='eza -a --icons=always --color=always --grid --group-directories-first'
  alias ll='eza -la --icons=always --color=always --group-directories-first --git'
  alias lt='eza -a --icons=always --color=always --tree --level=2 --group-directories-first'
fi
