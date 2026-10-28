---
name: tdd
description: Test-driven development. Use when the user wants to build features or fix bugs test-first, mentions "red-green-refactor", or wants integration tests.
---

## On-demand context

Use the initiating retrieval result's explicit `surface` and `snapshot_id` for
this procedure and every nested request. Read and follow the linked Matty entry
at each authored call below; these links identify prepared Catalog Resources,
without requiring native Skill-tool registration or an installed sibling copy.
For a fresh nested descriptor, use:

```sh
packy get skill matty:<skill> --surface <initiating-surface> --snapshot <initiating-snapshot-id> --json
```

Keep that host and snapshot even if catalog selection changes. If retained
content is unavailable, stop and restore that exact revision rather than use a
newer snapshot or another Pack's same-name skill. Obtaining instructions does
not execute the procedure or satisfy its runtime or human prerequisites.

Resolve skill references and templates relative to this returned entry and
`resource_root`. Resolve project paths, repository inspection, and configuration
against the user's intended target project; choose writable scratch/output
separately from immutable Catalog resources. Preserve the procedure's invocation
policy, user decisions, and output destinations. Read only the references needed
for the active branch.

Pass the initiating host, snapshot, target directory, writable scratch/output,
and returned entry/resource locations to every delegated agent or handoff. Read
Catalog instructions for the user's selected Matty workflow; preserve its own
policies even when another Pack is active. Availability does not authorize a
user-only invocation or bypass a decision required by the procedure.

# Test-Driven Development

TDD is the red → green loop. This skill is the reference that makes that loop produce tests worth keeping: what a good test is, where tests go, the anti-patterns, and the rules of the loop. Every section applies on every cycle: consult them before and during the loop, not after.

When exploring the codebase, read `GLOSSARY.md` (if it exists) so test names and interface vocabulary match the project's domain language, and respect ADRs in the area you're touching.

## What a good test is

Tests verify behavior through public interfaces, not implementation details. Code can change entirely; tests shouldn't. A good test reads like a specification: "user can checkout with valid cart" tells you exactly what capability exists, and it survives refactors because it doesn't care about internal structure.

See [tests.md](tests.md) for examples and [mocking.md](mocking.md) for mocking guidelines.

## Seams: where tests go

A **seam** is the public boundary you test at: the interface where you observe behavior without reaching inside. Tests live at seams, never against internals.

**Test only at pre-agreed seams.** Before writing any test, write down the seams under test and confirm them with the user. No test is written at an unconfirmed seam. You can't test everything, so agreeing the seams up front is how testing effort lands on the critical paths and complex logic instead of every edge case.

Ask: "What's the public interface, and which seams should we test?"

When the shape of that interface is itself in question (how deep the module is, where the seam belongs, what the interface should expose), read and follow [matty:codebase-design]({{packy.access:matty:codebase-design}}) for the vocabulary. It is the shared source of the module, interface, depth, seam, adapter, leverage and locality terms, and it is a reference to consult, not a session to run.

## Anti-patterns

- **Implementation-coupled**: mocks internal collaborators, tests private methods, or verifies through a side channel (querying the database instead of using the interface). The tell: the test breaks when you refactor but behavior hasn't changed.
- **Tautological**: the assertion recomputes the expected value the way the code does (`expect(add(a, b)).toBe(a + b)`, a snapshot derived by hand the same way, a constant asserted equal to itself), so it passes by construction and can never disagree with the code. Expected values must come from an independent source of truth: a known-good literal, a worked example, the spec.
- **Horizontal slicing**: writing all tests first, then all implementation. Bulk tests verify _imagined_ behavior: you test the _shape_ of things rather than user-facing behavior, the tests go insensitive to real changes, and you commit to test structure before understanding the implementation. Work in **vertical slices** instead: one test → one implementation → repeat, each test a **tracer bullet** that responds to what the last cycle taught you.

## Rules of the loop

- **Red before green.** Write the failing test first, then only enough code to pass it. Don't anticipate future tests or add speculative features.
- **One slice at a time.** One seam, one test, one minimal implementation per cycle.
- **Refactoring is not part of the loop.** It belongs to the review stage (see the `code-review` skill), not the red → green implementation cycle.
