# Keep application-writable targets outside the Installed Repository

Native configuration paths that a supported application flow or configuration
command can modify while the target exists must never resolve inside the
Installed Repository. Those Application-Writable Targets are materialized as
regular files or application-owned state; symlinks remain available for
exclusively dots-owned targets that are read under ordinary use or written only
to an operator-selected output destination. This protects the Source of Truth
from normal application writes without imposing reconciliation on every
Managed Entry. The evidence behind the current classification is recorded in
the [symlink ownership audit](../application-writable-target-research.md) and
implements the architecture slice in [#384](https://github.com/yersonargotev/dots/issues/384)
of the parent specification [#383](https://github.com/yersonargotev/dots/issues/383).

Materialization and Entry Ownership are separate decisions. A regular target
may remain wholly dots-owned, expose a structured subset or marked loader block,
or become Seeded Runtime State according to its real lifecycle. Zsh and Git use
marked loader blocks; Herdr and Atuin use TOML Subset Ownership; bat and Zellij
configuration remain Whole-Target Ownership; Zed settings use JSONC Subset
Ownership; Zed keybindings and the lazy.nvim lockfile use Seeded Runtime State;
and Neovim uses a regular native loader for separately Managed Configuration.
Whole-target application rewrites are Drift, while local evolution of Seeded
Runtime State remains aligned.

**Considered Options**: Universal symlinks were rejected because supported
writers can dirty the Installed Repository. Universal regular targets or one
universal ownership adapter were rejected because read-under-ordinary-use
targets need no reconciliation and configuration formats have different
composition, ordering, and lifecycle semantics. Explicit operator outputs do
not alone make a target application-writable: selecting a destination such as
Starship's preset output is treated as a deliberate Source of Truth edit.

**Consequences**: A provenance-backed legacy writable symlink may migrate only
after a Backup Set is created and its live content is reconciled without
ambiguity. Manual, stale, ambiguous, or concurrently changed targets remain
Conflicts and are never silently adopted. Uninstall removes only the proven
dots-owned contribution, retains Seeded Runtime State and external content, and
does not let force broaden partial ownership. Local state can enter the Source
of Truth only through explicit review; no generic promotion command is added.
The audit is dated evidence rather than a promise of immutability, so new
first-party writer behavior requires reclassification and migration.

**Non-goals**: This decision does not require a generic configuration adapter,
automatic promotion or capture, purging application-owned state, migration of
explicit-output-only targets, or replacement of symlinks without writer
evidence.
