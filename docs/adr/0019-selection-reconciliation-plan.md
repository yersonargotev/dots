# Plan explicit selection reconciliation from proven contributions

An explicit complete Profile or Tag request may select a different surface from
the authoritative Installed Selection. A simple selection delta cannot say
whether a removed Managed Entry contribution can be retired safely, whether a
shared target must be reconciled, or whether current workstation state makes
ownership ambiguous. We therefore produce one deterministic, read-only
Selection Reconciliation Plan from the previous and requested intent, both
Selected Surfaces, per-contribution Installation Metadata, and inspected target
state. `dots plan` and `dots install --dry-run` expose the same report and
ordering; the install dry run embeds that report in its Install Plan rather than
recomputing a second interpretation.

The plan is driven by the difference between the authoritative previous
Selected Surface and a complete explicitly requested Selected Surface. Each
contribution is classified as `create`, `update`, `preserve`, `reconcile`,
`remove`, `retain`, `blocked`, or `retained-external-state`. Actions and reasons
have deterministic ordering so text and JSON describe the same semantics.
Creation and update describe requested contributions, preservation describes
shared or already aligned contributions, reconciliation describes a safe
ownership-aware change to a co-owned target, removal requires exact evidence
that dots can subtract the retired contribution, retention preserves state that
must not be removed, and blocking records insufficient or contradictory proof.

Whole-target ownership and partial ownership fail closed in different ways. A
whole-target contribution with Drift or lost ownership is a finding and cannot
be retired safely. A subset-owned contribution is removable or reconcilable
only when its ordered per-contribution Installation Metadata and the live target
prove exactly what dots owns; missing, legacy target-wide, overlapping, or
otherwise inconclusive evidence is reported as ambiguous partial ownership.
This distinction lets an operator choose remediation without interpreting every
unsafe case as generic Drift.

Dependencies and Provisioners are outside Managed Entry ownership. When their
selecting Tags are removed, the plan reports the selection reduction and a
`retained-external-state` action, while preserving installed packages,
executables, receipts, and provisioned effects. A Dependency-only or
Provisioner-only reduction therefore has no Managed Entry removal. Reversing a
Dependency or Provisioner remains a separate, explicitly authorized concern.
Distinct Provisioner effects are compared by exact rendered command identity,
not only by tool name; reports expose a digest rather than raw command arguments.

Install Manifest evolution is also report-only: a changed manifest may explain
how the current surface differs, but it never supplies the operator intent
needed to retire a previously selected contribution. This slice performs no
filesystem removal, reconciliation, retirement, dependency reversal,
Provisioner reversal, or Installation Metadata update. A later mutating design
must define separate authorization and last-moment safety checks rather than
turning these preview actions into implicit cleanup.

**Considered options**: Extending the forward-only Install Plan without a
selection comparison was rejected because it cannot represent removed or
shared contributions. Treating the Uninstall Plan as the inverse was rejected
because uninstall operates on global recorded ownership rather than a mixed
previous/requested selection. Inferring retirement from Install Manifest
evolution was rejected because repository changes are not operator
authorization to remove workstation state.

**Consequences**: Machine consumers receive one stable reconciliation report
through both planning commands, and read-only planning treats unsafe ownership
findings as divergence. Install dry-run remains a successful action preview
while carrying the identical findings. No action in this report promises that a
future mutating command can apply it without revalidating ownership and target
state.

## Whole-target retirement amendment

Issue #466 authorizes `dots install` to consume the Selection Reconciliation
Plan only for an acknowledged, explicit Installed Selection reduction. The
plan remains pure and read-only; a separate retirement adapter validates the
complete report before any dependency or filesystem mutation, then revalidates
Installation Metadata, target ownership, and home confinement immediately
before applying each action. Retirement and the later terminal commit use the
same state-layer lock for serialized Installation Metadata transactions, so
unrelated concurrent records and receipts cannot be overwritten by a stale
read-modify-write cycle.

Known non-interactive Conflict decisions and interactive Conflict Resolution
are resolved before dependency mutation. The complete forward plan, including
the selected replace/adopt policy, and the complete retirement plan must both
validate before a package manager, Managed Entry, or Provisioner can mutate.

When a whole target leaves the requested Selected Surface, exact ownership
evidence produces `remove`. Drift, a missing target, changed target type, or
lost ownership produces `retain`: dots preserves the live target and releases
its Installation Metadata record without claiming deletion. Seeded Runtime
State likewise remains physically present while its ownership record is
released. Backup Sets are preserved and never restored by selection
retirement. Dependencies, Dependency Installation Metadata, Provisioner
receipts and effects, and user-owned state remain untouched; their known
selection residue stays `retained-external-state` in the shared report.

An entirely deselected structured target can be removed when exact subset
evidence proves that subtraction leaves it empty; if external content remains,
the target is retained and its ownership record is released. The contribution
reconciliation amendment below governs partial retirement from a still-selected
target and source-override fallback. Install Manifest evolution remains
report-only and never authorizes retirement.

`--clear-selection` supplies explicit empty-selection authority. Interactive
execution requires the literal `clear` acknowledgement. Non-interactive
execution, including a dry run, requires `--clear-selection`, `--yes`, and
`--acknowledge-selection-change`; omitted Profile/Tag flags never mean an empty
selection. A successful terminal run records empty ordered intent, while any
decline, block, cancellation, or failure preserves the prior authoritative
Installed Selection. Rerunning after a partial removal converges from current
filesystem and metadata evidence without manual repair.

## Contribution reconciliation amendment

Issue #467 authorizes an acknowledged explicit Installed Selection reduction to
reconcile a target that remains selected when exact ordered per-contribution
Installation Metadata proves both the retired and retained Source of Truth
inputs. The read-only Selection Reconciliation Plan remains the authority for
the semantic outcome. Before any dependency or filesystem mutation, the
retirement gate requires every safe `create`, `update`, `preserve`, or
`reconcile` outcome to have a matching forward Managed Entry action; `blocked`,
ambiguous, incompatible, or mismatched forward behavior stops the entire run.

The forward Managed Entry plan owns each still-selected target and applies it
exactly once. Shared JSON targets compose only the selected contributions before
three-way reconciliation. JSON, JSONC, TOML, and marked-block updates subtract
retired values only from exact compatible evidence and preserve target-only
content according to their existing ownership contracts. Target-wide
compatibility fields must equal the projection of exact ordered contribution
evidence; contradictory metadata fails closed. A whole-target source
override may return to its selected base source only when the live target still
matches the exact previously recorded contribution hash. Missing or legacy
target-wide attribution, changed retired values, unsafe target types, and stale
forward classifications fail closed before mutation.

Source override retirement replaces the prior override contribution with the
selected base contribution. Successful shared reconciliation records only the
retained ordered contributions and commits the requested Installed Selection at
terminal success. A failure after the filesystem update preserves the previous
Installed Selection and contribution evidence. Installation Metadata version 8
adds a recovery-only reconciliation receipt containing the exact resulting
target hash plus the ordered current source identities and hashes. A rerun may
recognize already removed retired values only when that receipt matches every
byte and source; absent or mismatched receipts remain ambiguous and fail closed.
The receipt is persisted immediately after each applied reconciliation, before
the next Managed Entry action; terminal metadata commit replaces the old
contribution evidence and clears the receipt. Seeded Runtime State keeps its
existing whole-retirement rule: its
physical bytes remain while dots releases only the ownership record.
