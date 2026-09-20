# Interactive tool integrations.
#
# Each is guarded by command -v so the shell stays usable when a tool is not
# installed on this machine.

command -v starship >/dev/null 2>&1 && eval "$(starship init zsh)"
command -v fnm      >/dev/null 2>&1 && eval "$(fnm env)"

# fzf owns file insertion and directory navigation. Disable its history widget
# while sourcing the official integration so Ctrl+R has one owner: Atuin.
if command -v fzf >/dev/null 2>&1; then
  export FZF_DEFAULT_OPTS="${FZF_DEFAULT_OPTS:+${FZF_DEFAULT_OPTS} }--height=40% --min-height=12 --layout=reverse --border=top --color=bg:#1e1e2e,bg+:#313244,fg:#cdd6f4,fg+:#cdd6f4,hl:#f38ba8,hl+:#f38ba8,info:#cba6f7,marker:#b4befe,pointer:#f5e0dc,prompt:#cba6f7,spinner:#f5e0dc,border:#6c7086"
  export FZF_CTRL_T_OPTS="${FZF_CTRL_T_OPTS:+${FZF_CTRL_T_OPTS} }--walker-skip=.git,node_modules,target --preview 'if [ -d {} ]; then if command -v eza >/dev/null 2>&1; then eza -a --color=always --icons=always --tree --level=2 -- {}; else printf \"dots: eza is required for directory previews.\\n\"; fi; elif command -v bat >/dev/null 2>&1; then bat --color=always --style=numbers --line-range=:500 -- {}; else printf \"dots: bat is required for file previews.\\n\"; fi'"
  export FZF_ALT_C_OPTS="${FZF_ALT_C_OPTS:+${FZF_ALT_C_OPTS} }--walker-skip=.git,node_modules,target --preview 'if command -v eza >/dev/null 2>&1; then eza -a --color=always --icons=always --tree --level=2 -- {}; else printf \"dots: eza is required for directory previews.\\n\"; fi'"

  _dots_fzf_init="$(fzf --zsh 2>/dev/null)" || _dots_fzf_init=""
  if [[ -n "${_dots_fzf_init}" ]]; then
    FZF_CTRL_R_COMMAND= eval "${_dots_fzf_init}"
  else
    # fzf before 0.48 shipped its integration as package files rather than
    # embedding it in the binary. Cover the supported package layouts without
    # invoking a package manager or reaching the network.
    for _dots_fzf_bindings in \
      "${HOME}/.fzf/shell/key-bindings.zsh" \
      /opt/homebrew/opt/fzf/shell/key-bindings.zsh \
      /usr/local/opt/fzf/shell/key-bindings.zsh \
      /usr/share/fzf/key-bindings.zsh \
      /usr/share/doc/fzf/examples/key-bindings.zsh; do
      [[ -r "${_dots_fzf_bindings}" ]] || continue
      FZF_CTRL_R_COMMAND= source "${_dots_fzf_bindings}"
      break
    done
    unset _dots_fzf_bindings
  fi
  unset _dots_fzf_init

  # Older integration scripts predate the Ctrl+R opt-out. Remove any binding
  # they installed before Atuin initializes below.
  bindkey -r '^R' 2>/dev/null
else
  print -u2 'dots: fzf not found; fuzzy completion and navigation are unavailable.'
fi

# Use Zsh's standard external command editor through VISUAL/EDITOR.
autoload -Uz edit-command-line
zle -N edit-command-line
bindkey '^X^E' edit-command-line

# zoxide retains ownership of z and zi.
command -v zoxide >/dev/null 2>&1 && eval "$(zoxide init zsh)"

# atuin ships an env shim plus a shell init step.
[[ -r "${HOME}/.atuin/bin/env" ]] && source "${HOME}/.atuin/bin/env"
command -v atuin >/dev/null 2>&1 && eval "$(atuin init zsh)"
