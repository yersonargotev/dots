# Zim module tuning.
#
# These settings are read by modules at load time, so they MUST run before Zim
# initializes (this file is part of the pre-init phase in ~/.zshrc).

# zsh-autosuggestions: skip widget re-binding on every prompt for performance.
ZSH_AUTOSUGGEST_MANUAL_REBIND=1

# zsh-syntax-highlighting: enable only the highlighters we use.
ZSH_HIGHLIGHT_HIGHLIGHTERS=(main brackets)

# fzf-tab: keep completion grouped and contained in the lower part of the
# terminal. The plugin intentionally has its own options because it does not
# inherit FZF_DEFAULT_OPTS by default.
zstyle ':completion:*:descriptions' format '[%d]'
zstyle ':completion:*' menu no
zstyle ':fzf-tab:*' switch-group '<' '>'
zstyle ':fzf-tab:*' fzf-flags \
  '--height=40%' \
  '--min-height=12' \
  '--layout=reverse' \
  '--border=top' \
  '--color=bg:#1e1e2e,bg+:#313244,fg:#cdd6f4,fg+:#cdd6f4,hl:#f38ba8,hl+:#f38ba8,info:#cba6f7,marker:#b4befe,pointer:#f5e0dc,prompt:#cba6f7,spinner:#f5e0dc,border:#6c7086'
zstyle ':fzf-tab:complete:*:*' fzf-preview \
  'if [[ -d $realpath ]]; then if command -v eza >/dev/null 2>&1; then eza -a --color=always --icons=always --tree --level=2 -- "$realpath"; else print "dots: eza is required for directory previews."; fi; elif [[ -f $realpath ]]; then if command -v bat >/dev/null 2>&1; then bat --color=always --style=numbers --line-range=:500 -- "$realpath"; else print "dots: bat is required for file previews."; fi; fi'
