---
status: accepted
---

# Select a shared theme independently of application capabilities

Issue [#538](https://github.com/yersonargotev/dots/issues/538) requires optional
Carbonfox colors across applications whose appearance dots already manages.
Users must be able to choose their own application Tags rather than install
a complete Workstation preset to get those colors. We select one global
`theme-carbonfox` Tag, composed with the user's normal selection. The
maintainer accepted this boundary during triage; application support and
delivery acceptance evidence remain separate from this architecture decision.
The implementation uses existing source overrides, a sourceable shared marker,
and native theme assets; it adds no Profile variants or template engine.

Application Tags continue to select their existing Managed Entries. The theme
Tag changes the appearance of selected supported consumers without selecting
additional applications. Profiles remain convenience presets and acquire no
implicit Carbonfox preference. Both explicit Tag selection and selection
through a Profile must produce the same themed Selected Surface when their
effective Tags are equivalent.

Carbonfox is dark-only, optional,
dominant over `adaptive-theme` regardless of Tag order, and limited to existing
appearance ownership. It applies uniformly to every selected supported
consumer. Per-application palette mixing is outside this design.

## Why this boundary

ADR 0018 already treats `adaptive-theme` as a global preference applying only
at selected consumer seams. A second preference can use the same separation
between application choice and appearance. It also fits the interactive
selector, which persists explicit Tags rather than Profile names.

We reject a dedicated `workstation-carbonfox` Profile and scoped Profile
Replacement for this feature. They introduce named-Profile
exclusivity, replacement behavior, and questions about preserving Profile
identity without making an arbitrary Tag selection easier to theme.
Per-application alternatives such as `ghostty-carbonfox` would duplicate
capability choices and create unnecessary selection combinations.

Selection responsibility stays with the operator, while dots retains its
existing planning, confirmation, Conflict Resolution, and ownership safety.
Explicit selection flags keep their complete-selection meaning from ADR 0014.
Removing the theme preference must resolve to the remaining selection's
appearance: adaptive when selected, otherwise the existing baseline.
Restoration uses the existing ownership and reconciliation contracts rather
than overwriting externally edited configuration or inventing another
replacement flow.

## Delivery constraints

- Cover Ghostty, Herdr including authored row colors, tmux, Zellij including
  its status layout, Neovim, Starship, Atuin, bat, tuicr, Zed, Warp, both
  Zsh/fzf configuration paths, Claude UI and statusline, and Copilot statusline.
  Use native themes or authored mappings of the canonical Nightfox Carbonfox
  palette. Preserve icon artwork and non-theme behavior.
- A consumer selected alone with the theme Tag must work independently of
  other application Tags. ANSI inheritance from an unselected terminal is
  not sufficient evidence of Carbonfox support. Native custom-theme assets
  and required user-local cache generation are within the theme change;
  generated caches remain application state, not Source of Truth.
- Prefer native theme assets and includes to share non-theme behavior.
  Source overrides select complete files; they do not merge fragments.
  Template rendering is not currently implemented and is not presumed by
  this decision.
- Current source override precedence depends on Tag order. Carbonfox's
  agreed precedence needs an explicit, tested solution that preserves
  unrelated override behavior. Do not introduce a general theme framework
  or a template engine for this feature.
- Review activation and removal through both the interactive selector and
  complete explicit selections, including both theme preferences together.
- Verify native syntax, declared version requirements, palette provenance,
  license notices, and activation/removal behavior during implementation.
  Application restart or reload instructions belong in documentation;
  delivery does not authorize reloading the operator's live sessions.

## Native integration evidence

The canonical palette and license are maintained by
[Nightfox](https://github.com/EdenEast/nightfox.nvim). Custom native themes
are documented for [bat](https://github.com/sharkdp/bat#adding-new-themes),
[tuicr](https://github.com/agavra/tuicr#configuration),
[Warp](https://docs.warp.dev/terminal/appearance/custom-themes), and
[Claude Code](https://code.claude.com/docs/en/terminal-config#create-a-custom-theme).
In particular, bat requires local theme cache generation, and Claude supports
named JSON themes with direct color overrides. These are feasibility findings,
not substitutes for sandboxed acceptance verification against supported
application versions.

Owned symlink source switches continue to use explicit Conflict Resolution and
Backup Sets. They do not gain an automatic update primitive. The action gate
requires the real per-target Replace decision before applying either direction;
exact prior contribution and link evidence distinguish that transition from an
externally changed or ambiguously owned target.
