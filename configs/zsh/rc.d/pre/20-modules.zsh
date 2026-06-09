# Zim module tuning.
#
# These settings are read by modules at load time, so they MUST run before Zim
# initializes (this file is part of the pre-init phase in ~/.zshrc).

# zsh-autosuggestions: skip widget re-binding on every prompt for performance.
ZSH_AUTOSUGGEST_MANUAL_REBIND=1

# zsh-syntax-highlighting: enable only the highlighters we use.
ZSH_HIGHLIGHT_HIGHLIGHTERS=(main brackets)
