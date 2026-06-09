# Portable PATH additions.
#
# Each block is guarded so a tool that is absent on this machine never breaks
# login. Machine-specific paths belong in ~/.zshrc.local, not here.

# pnpm. Honors an existing PNPM_HOME; otherwise defaults to the macOS location.
: "${PNPM_HOME:=${HOME}/Library/pnpm}"
if [[ -d "${PNPM_HOME}" ]]; then
  export PNPM_HOME
  case ":${PATH}:" in
    *":${PNPM_HOME}:"*) ;;
    *) export PATH="${PNPM_HOME}:${PATH}" ;;
  esac
fi

# bun.
: "${BUN_INSTALL:=${HOME}/.bun}"
if [[ -d "${BUN_INSTALL}" ]]; then
  export BUN_INSTALL
  export PATH="${BUN_INSTALL}/bin:${PATH}"
fi
[[ -s "${BUN_INSTALL}/_bun" ]] && source "${BUN_INSTALL}/_bun"

# Generic local-bin env shim (rustup, uv, and similar installers write here).
[[ -r "${HOME}/.local/bin/env" ]] && source "${HOME}/.local/bin/env"
