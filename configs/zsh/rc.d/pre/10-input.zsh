# Input: keymap and word boundaries.

# Use the emacs keymap for line editing.
bindkey -e

# Treat the path separator as a word boundary (e.g. for Ctrl-W deletion).
WORDCHARS=${WORDCHARS//[\/]}
