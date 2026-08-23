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

Resolve every actionable finding inside the safety case and return the challenge
result to the parent workflow's completion gate.

## Independent implementation-conformance challenge

For `required-mutation` only, run a second independent challenge after the
implementation passes focused checks and before complete CI-equivalent or manual
verification. Give an independent context the Delivery Contract snapshot by
reference, base commit, approved safety case, current implementation diff, its
SHA-256 mutation-boundary diff digest against that base, and focused-test
evidence. The approved safety case includes its compatibility map, transaction
boundary, threat and failure matrix, fault seams and acceptance evidence, and
operation invariant. Do not duplicate the Delivery Contract in the challenge
input. `not-applicable` Delivery Units skip this challenge.

Compare the implemented mutation boundary with the approved safety case and
explicitly challenge:

- whether public and persisted behavior still matches the compatibility map;
- whether every transition preserves the transaction boundary and operation
  invariant from validated authority through durable evidence;
- code-level authority and resource lifecycle behavior, including temporal
  ordering, cancellation, bounded concurrency and resource lifetime, exact
  identity consumption at the operation that uses it, observable cleanup-error
  handling, and safe recovery or compensation;
- whether the implemented failure paths and tests cover the approved threat and
  failure matrix and use the planned fault seams; and
- whether the implementation remains within the approved mutation model.

The challenge completes only with zero actionable findings. Actionable local
implementation gaps return to focused implementation in the parent workflow;
after a fix, repeat focused tests and this challenge without rebuilding the
safety case. An actual change to the approved mutation model returns
`mutation-model-changed`, invalidates the safety case, and must repeat the
complete pre-implementation safety gate, including the independent design
challenge. Continue to use the existing `unauthorized-decision` and
`missing-capability` results when their matrix scenarios apply.

Bind the result to the base commit and mutation-boundary diff digest. A later
mutation-boundary code change invalidates the implementation-conformance result
and requires another challenge before expensive gates continue. Documentation
or unrelated artifact changes do not invalidate the implementation-conformance
result when comparison with the final reviewed head proves that digest is
unchanged. They still invalidate automated, manual, and final review evidence
under the parent workflow's general rule and do not by themselves change the
approved mutation model. If the unchanged mutation boundary cannot be proved,
fail closed and repeat the implementation-conformance challenge.

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
evidence, and the early independent design challenge result. For a completed
implementation-conformance challenge, include the base commit and
mutation-boundary diff digest, input references and focused-test evidence, the
questions covered, every finding and resolution, and the
implementation-conformance challenge result. Record the final reviewed head and
the comparison proving its mutation-boundary digest matches. These records
preserve both independent challenges for the final reviewed head.
For `not applicable`, include the direct evidence that no mutation boundary
exists.
