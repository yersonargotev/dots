# Mutation safety gate

Build one reviewed safety case before implementing a Delivery Unit whose change
crosses a mutation boundary. Use the Delivery Contract by reference: bind the
safety case to its digest and the base commit, and record acceptance-criterion
identifiers instead of copying scope.

## Compatibility map

Inspect the affected public behavior, persisted formats, tests, documentation,
dependencies, and platform guarantees. Record every affected public or
persisted behavior as either preserved or explicitly authorized to change by
the Delivery Contract. For each behavior, name the evidence that will prove the
classification.

Account for compatibility distinctions that an implementation could otherwise
collapse, including:

- a confined source symlink versus a path that escapes its allowed root;
- captured empty bytes versus a missing or uncaptured value;
- current evidence versus legacy or compatibility projections; and
- external edits versus bytes produced by the Delivery Run.

Complete this part when every affected behavior has one classification, one
authority, and one planned proof. A missing product decision is not a local
implementation choice.

## Transaction boundary

State one operation invariant that connects validated authority, captured
inputs, mutation, and durable evidence. Then identify:

- the authority and exact version that permit the mutation;
- source, target, record, and receipt identity throughout the operation;
- captured bytes and metadata, preserving an explicit zero-length value;
- every observable state transition and its precondition;
- the commit point and the durable evidence that proves it;
- the state returned by each failure before and after that commit point; and
- the recovery or compensation rule for mutations that cannot commit atomically.

Checks that authorize mutation run at the same serialized boundary as the
mutation or use an equivalent compare-and-set primitive. Rollback is itself a
conditional mutation: restore only while the live state still matches the
postimage produced by the Delivery Run, and preserve later external edits.

Complete this part when every transition maps authority, captured input,
mutation, durable evidence, and safe retry or recovery.

## Threat and failure matrix

Challenge every applicable dimension before choosing the implementation:

- target state and ownership mode;
- current, missing, legacy, or contradictory evidence;
- source, target, record, or receipt change between temporal gates;
- symlink substitution, path escape, or identity change;
- empty content, partial write, short write, sync failure, rename failure, and
  persistence failure;
- cancellation or crash before, during, and after the commit point;
- rollback after an external edit; and
- platform or filesystem guarantees on every supported path.

Every applicable threat or failure has a mitigation, planned test, or specific
`not applicable` reason. Combine dimensions where interactions create different
outcomes; a list of independent happy paths is not a failure matrix.

Complete this part when every state transition and acceptance criterion is
covered and no matrix cell has an implicit outcome.

## Fault seams and acceptance evidence

Choose the narrowest observable seams before implementation. Reuse an existing
public seam when it can deterministically inject the required failure; otherwise
plan one small seam at the mutation or persistence boundary. Cover applicable
partial writes, sync, rename, metadata save, compare-and-set, rollback, and
source or target interleavings.

Map every acceptance criterion to focused automated evidence, manual evidence,
or both. Include the expected state after failure and after a repeated recovery,
not only the returned error. Keep the final CI-equivalent suite as a terminal
gate rather than using it as the first discovery loop.

Complete this part when every required fault is injectable and every acceptance
criterion has evidence that can disagree with the implementation.

## Independent design challenge

Give an independent context the Delivery Contract, base commit, compatibility
map, transaction boundary, matrix, and evidence plan. Require it to challenge:

- authority or identity checked outside the operation that consumes it;
- public behavior missing from the compatibility map;
- mutation visible without matching durable evidence;
- recovery that can overwrite external state;
- failure points without deterministic injection; and
- tests that prove only the planned implementation shape.

Resolve every actionable finding inside the safety case. Pass only with zero
actionable findings. This challenge does not replace final Spec and Standards
review of the implemented commit.

## Gate results and invalidation

Use exactly one result from this matrix:

| Scenario | Evidence | Result |
| --- | --- | --- |
| `required-mutation` | Change spans managed filesystem mutation, persisted metadata or receipts, recovery or rollback, or authority or identity that may change concurrently | Complete the safety case before implementation |
| `not-applicable` | Documentation, skill, CI, or metadata-only change with no mutation boundary | Record `not applicable` with direct evidence |
| `unauthorized-decision` | Material product or architecture decision is absent from the Delivery Contract | Return `needs-triage` with the decision and evidence required |
| `missing-capability` | A required gate capability is unavailable | Return `blocked` with the unavailable capability |
| `mutation-model-changed` | Implementation changes the approved mutation model | Invalidate the result and repeat the gate before further implementation |

Local, reversible design gaps remain inside the gate until repaired. A restart
before the PR exists repeats the gate because the workflow has no private state.

## Delivery evidence

Record the result in the PR's `Delivery Evidence` block. For a completed safety
case, include the base commit and Delivery Contract digest, the operation
invariant, compatibility and matrix coverage, planned fault seams and acceptance
evidence, and the independent challenge result. For `not applicable`, include
the direct evidence that no mutation boundary exists.
