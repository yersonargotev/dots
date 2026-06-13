# Aliases. Guarded where they depend on an optional tool.

# Prefer eza for `ls` when available. eza is an alias-only slice with no managed
# config file; see configs/eza/README.md for the rationale.
if command -v eza >/dev/null 2>&1; then
  alias ls='eza -a --icons=always --color=always --grid --group-directories-first'
fi
