# Portable PATH additions.
#
# Each block is guarded so a tool that is absent on this machine never breaks
# login. Machine-specific paths belong in ~/.zshrc.local, not here.

# pnpm. Honors an existing PNPM_HOME; otherwise defaults to the macOS location.
# Different pnpm versions place global binaries directly in PNPM_HOME or under
# PNPM_HOME/bin, so add whichever directories exist.
: "${PNPM_HOME:=${HOME}/Library/pnpm}"
if [[ -d "${PNPM_HOME}" ]]; then
  export PNPM_HOME
  for _pnpm_dir in "${PNPM_HOME}/bin" "${PNPM_HOME}"; do
    [[ -d "${_pnpm_dir}" ]] || continue
    case ":${PATH}:" in
      *":${_pnpm_dir}:"*) ;;
      *) export PATH="${_pnpm_dir}:${PATH}" ;;
    esac
  done
  unset _pnpm_dir
fi

# bun.
: "${BUN_INSTALL:=${HOME}/.bun}"
if [[ -d "${BUN_INSTALL}" ]]; then
  export BUN_INSTALL
  export PATH="${BUN_INSTALL}/bin:${PATH}"
fi
[[ -s "${BUN_INSTALL}/_bun" ]] && source "${BUN_INSTALL}/_bun"

# yarn global binaries. Use the static default location rather than
# `$(yarn global bin)`, which spawns a Node process on every shell startup.
[[ -d "${HOME}/.yarn/bin" ]] && export PATH="${HOME}/.yarn/bin:${PATH}"

# Generic local-bin env shim (rustup, uv, and similar installers write here).
[[ -r "${HOME}/.local/bin/env" ]] && source "${HOME}/.local/bin/env"
