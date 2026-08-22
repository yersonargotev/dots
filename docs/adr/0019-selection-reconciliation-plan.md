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
