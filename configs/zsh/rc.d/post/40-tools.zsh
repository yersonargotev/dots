# Interactive tool integrations.
#
# Each is guarded by command -v so the shell stays usable when a tool is not
# installed on this machine.

command -v starship >/dev/null 2>&1 && eval "$(starship init zsh)"
command -v zoxide   >/dev/null 2>&1 && eval "$(zoxide init zsh)"
command -v fnm      >/dev/null 2>&1 && eval "$(fnm env)"

# atuin ships an env shim plus a shell init step.
[[ -r "${HOME}/.atuin/bin/env" ]] && source "${HOME}/.atuin/bin/env"
command -v atuin >/dev/null 2>&1 && eval "$(atuin init zsh)"
