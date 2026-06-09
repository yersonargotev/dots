# Aliases. Guarded where they depend on an optional tool.

# Prefer eza for `ls` when available.
if command -v eza >/dev/null 2>&1; then
  alias ls='eza -a --icons=always --color=always --grid --group-directories-first'
fi
