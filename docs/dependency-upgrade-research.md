# Safe activation of upgraded User-Local Provider bundles

Status: proposed design for [#592](https://github.com/yersonargotev/dots/issues/592),
researched 2026-11-07 against `70d8a3c`. This document does not amend an accepted
ADR or authorize workstation mutation. It separates documented operating-system
guarantees from design conclusions and proposed authority assumptions.

## Documented primitives

On Linux, a normal `rename` atomically replaces an existing destination and
keeps the destination name continuously present. `renameat2` adds two relevant
operations: `RENAME_NOREPLACE` fails if the destination exists, while
`RENAME_EXCHANGE` atomically exchanges two existing pathnames. The flags cannot
be combined, and support for `RENAME_NOREPLACE` depends on the filesystem.
Linux also documents an NFS ambiguity: a failed rename response does not prove
that the server did not perform the rename.
([Linux `rename(2)`](https://man7.org/linux/man-pages/man2/renameat2.2.html))

Darwin provides the analogous `renameatx_np` flags. `RENAME_EXCL` returns
`EEXIST` when the destination already exists; `RENAME_SWAP` atomically swaps
the source and destination. The flags cannot be combined, and unsupported
filesystems return `ENOTSUP`. Volume capabilities advertise support for the two
operations.
([XNU `rename(2)`](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/man/man2/rename.2),
[XNU declarations](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/stdio.h))

These interfaces condition only on whether a destination name exists. None of
their documented flags means “replace this destination only if it still names
the previously inspected object.” This is an absence in the documented
interfaces, not proof that no platform-specific mechanism could ever provide
such a comparison.

An atomic exchange does not solve that comparison problem. If another process
replaces the public entry after validation but before `RENAME_EXCHANGE` or
`RENAME_SWAP`, the exchange can move that new, foreign entry away from its
public name. Detecting the substitution afterward is too late to satisfy a
strict “never displace external state” invariant. This is an inference from
the documented exchange semantics.

Namespace atomicity is also distinct from crash durability. Linux explicitly
requires `fsync` on the containing directory to persist its directory entry;
syncing the file alone is insufficient.
([Linux `fsync(2)`](https://man7.org/linux/man-pages/man2/fsync.2.html))
Darwin documents that ordinary `fsync` can still leave writes buffered or
reordered by a drive, and provides `F_FULLFSYNC` for applications requiring
tighter persistence and ordering. Darwin's `F_BARRIERFSYNC` orders phases but
does not promise that they are durable when the call returns.
([XNU `fsync(2)`](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/man/man2/fsync.2),
[XNU `fcntl(2)`](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/man/man2/fcntl.2))

Advisory locks coordinate only cooperating processes. Linux documents that a
process with permission can ignore `flock` and perform I/O; remote filesystem
semantics also vary by protocol, mount options, and server.
([Linux `flock(2)`](https://man7.org/linux/man-pages/man2/flock.2.html))
Therefore the dots home/state locks can serialize dots operations, but cannot
turn a pathname replacement into an atomic expected-object comparison against
an editor or another process.

## Existing behavior and verification

The rejection is intentional: the approved compatibility exception in
[#584](https://github.com/yersonargotev/dots/issues/584), delivered by
[#585](https://github.com/yersonargotev/dots/pull/585), requires an operator to
preserve or move even a validated older public link.
[ADR 0021](adr/0021-recover-user-local-provider-receipts.md) records this policy.

The redundancy search covered `actionFor`, `herdrCustomInstallGuard`,
`priorHerdrCustomArtifact`, `InstallUserLocal`, `UserLocalInstaller` publication
and recovery, receipt loading, and provider/CLI tests. Planning and direct
installation both enforce the guard; publication independently refuses occupied
destinations. Changing guard order alone cannot deliver the requested update.
`priorHerdrCustomArtifact` validates layout, markers and file hashes; it does
**not** establish finalized receipt authority. See
[provider validation](../internal/deps/herdr_custom.go),
[planning](../internal/deps/plan.go),
[publication](../internal/deps/user_local_transaction.go), and
[receipt loading](../internal/deps/dependency_metadata.go).

The only current `.out-of-scope/` entry concerns VS Code and is unrelated. The
relevant prior decision is the compatibility exception above. CodeGraph was
unavailable in this session; source exploration used targeted reads.

Existing tests passed on the macOS host using temporary homes and fixture
artifacts. Their simulated Linux platform does not constitute Linux kernel
validation or installation of a real macOS release:

```text
go test ./internal/deps -run '^TestHerdrCustom(UpgradePreservesOccupiedPriorLink|UpgradeRejectsUnconfinedOrUnverifiedPrior|InstallPreservesOfficialAndUsesDotsConfig|RejectsUnmanagedCommandBeforeDownload)$' -count=1 -v
PASS
```

This confirms rejection at planning and direct publication and preservation of
the prior link. It does not reproduce the reporter's exact Homebrew version or
full CLI invocation. A separate deterministic temporary-directory experiment
confirmed the check-to-replace race:

```text
check=managed-old; concurrent replacement=foreign-late; after replace=managed-new
no-replace publication refused collision; public=foreign-late
```

## Alternatives and recommendation

| Design | Public-path preservation | Main cost | Decision |
| --- | --- | --- | --- |
| Validate then replace/exchange public link | Cannot preserve a late foreign edit | Recurring exclusive access to public leaf | Reject as default |
| Stable dispatcher and private selector | Updates never write the public leaf | One migration; permanent private runtime reservation | Recommend |
| Stable dispatcher and immutable activation history | Updates never write the public leaf | History validation, ordering and reader consistency on launch | Defer |

An explicit exclusive-update option could retain today's layout only if the
operator guarantees no other writer changes the public leaf during each update
and retry. `--yes` cannot silently grant that authority. Rechecking or locking
does not enforce this precondition against unrelated processes.

An append-only activation history avoids replacing a private selector but does
not authenticate coherent same-UID tampering or solve legacy migration. Making
one home-local activation record the sole receipt-and-selection commit would
also relocate receipt authority. Retain the existing recoverable lifecycle.

**Recommend a stable public dispatcher, immutable verified generations and an
atomically replaced private selector.** Recognizing a prior bundle is useful
validation, not authorization to replace a public link. #592's acceptance criteria
must become: explicit legacy migration once, then automatic managed activation.

## Runtime layout and authority

Illustrative layout:

```text
~/.local/bin/herdr-custom
  -> ~/.local/opt/herdr-custom/.dots-runtime/v1/launcher

.dots-runtime/v1/                 # 0700, permanently reserved to dots
  owner.json                     # immutable runtime ID + receipt-state binding
  launcher                       # immutable dispatcher, ABI 1
  current                        # 0600: 32 lowercase hex bytes + newline
  generations/<operation-id>/    # immutable verified generation
    entry                        # reviewed route to the bundled launcher
    payload/...

<registered-state>/dependencies.d/<operation-id>/
  authority-<digest>.json
  captured.json
  ready.json
  final.json
  complete.json
```

The new assumption needs explicit approval: the runtime namespace is reserved
to dots **between operations as well as during them**. Other same-UID tools must
not mutate it. Mode 0700 does not enforce that rule against the same UID.
Observed corruption fails closed; coherent replacement of the entire private
authority is outside the guarantee. Public commands remain concurrently editable.
This extends ADR 0021's operation-private assumption.

Create registration through explicit enrollment. Bind its home-local runtime ID
to an immutable counterpart in the selected receipt state and its canonical
location. Resolve aliases under the home lock. Reject another state root with
guidance to use the registered state; it cannot bypass pending recovery. State
relocation/rebinding is outside the first implementation.

Completed registration is durable logical authority, not equality to a prior
boot's object incarnation. Capture fresh physical identities per operation and
validate registration counterparts. The current Linux incarnation includes boot
and mount identity: pending operations retain ADR 0021's epoch-change failure
behavior, while completed runtimes must work after reboot. Historical receipts
must never be interpreted as this new registration authority.
([Linux identity](../internal/deps/user_local_identity_linux.go),
[receipt model](../internal/deps/dependency_journal.go))

For ABI 1, choose a small **native dispatcher**, built and verified with the
release. It opens `current` once and accepts exactly 32 lowercase hexadecimal
bytes plus one newline, rejecting NULs, extra bytes, invalid IDs and missing
generations. It executes the fixed `entry` path in that generation, preserving
arguments, environment and working directory. Runtime discovery uses its
enrolled executable location, independently of caller `HOME`, `PATH`, the
current dots executable and arbitrary `--state-root`. Verified dispatcher
packaging is part of implementation scope; a later ABI/dispatcher replacement
needs its own protocol.

A shell sketch using only builtins worked in a temporary sandbox: 80 launches
during 500 atomic selector replacements observed only complete old/new
generations and kept the public symlink inode unchanged. Five malformed text
selectors were rejected with unusable `PATH` and unrelated `HOME`. This sketch
is not production code: shell `read` can discard NULs and cannot prove the exact
byte format. It tested neither receipt recovery nor crash durability. Choosing
a native dispatcher avoids that limitation.

Retain all generations. Each invocation selects one generation; its lazy runtime
file reads continue to use that immutable path. No automatic process restart,
generation deletion, or application-state rollback accompanies activation.

## Module interface

Keep activation behind the existing `UserLocalInstaller` interface. Callers
submit a reviewed `InstallAction`; they do not coordinate selectors or receipt
phases. Internal seams cover download, filesystem publication and receipt fault
injection, with Linux/Darwin adapters for platform primitives.

Read-only planning classifies a selected Dependency as satisfied, fresh install,
managed activation, legacy migration required, or conflict. Execution/recovery
consumes captured pending authority before ordinary PATH satisfaction. The
normal `deps install`, `install`, `update` and `upgrade` callers share this
module; avoid a separate CLI-specific activation implementation.

For enrolled runtimes, a new pinned version is eligible only when registration,
completed predecessor authority, selector and generation agree. Same version
with a different checksum is a conflict. Reject foreign public routes, PATH
shadows, unmanaged bundles and changed owned bytes. Rolling providers retain
[ADR 0016](adr/0016-verified-rolling-agent-cli-providers.md)'s present-command
satisfaction policy.

## Transaction and recovery

Hold the home-wide lock and registered state lock, in that order. Validate their
named identities and captured public-parent/root ancestry as today.

1. Reconcile applicable pending work. Validate registration, public routing,
   current generation and completed predecessor authority.
2. Persist captured authority for the exact desired artifact, predecessor,
   roots, generation, selector preimage/postimage and complete command surface.
3. Verify the archive and stage the complete generation. Prepare, synchronize
   and close the canonical selector file in the same private directory as
   `current`. Durably persist ready evidence for the staged generation tree,
   source identities/parents and selector postimage **before publishing either**.
4. Publish the new generation with no-replace operations. Verify and synchronize
   it and its ancestry, then persist publication progress. Ready evidence lets
   recovery recognize promotion even if its progress write was interrupted.
5. Persist the final receipt payload, pending without completion.
6. Revalidate authority. Atomically replace **private** `current` and synchronize
   its directory. Never write it in place. Replacement relies on private
   reservation, not an expected-object kernel comparison.
7. Verify selection, generation, public routing, parents and locks. Persist
   completion acknowledgement and verify again before reporting success.

Public entries remain untouched. An external public edit can redirect future
commands and makes observed validation fail; dots preserves the edit. An edit
detected after activation can leave a new private selection with pending
recovery. Do not overwrite public state or destructively roll back to conceal
that failure.

| Interruption / observed state | Required result |
| --- | --- |
| No durable capture | No publication allowed |
| Captured/staged; predecessor selected | Keep predecessor serving; resume captured artifact |
| Final payload durable; selector unchanged | Revalidate and activate captured generation |
| Intended selector visible; directory durability uncertain | Validate and synchronize before relying on it |
| New generation selected; completion absent | Finalize same operation; no new resolution or switch-back |
| Completion durable | Applicable retry is a no-op |
| Foreign selector, changed generation, registration mismatch or ambiguous evidence | Preserve evidence and external paths; actionable execution error |

Activation and acknowledged receipt completion remain distinct. A new command
can run the selected generation before receipt completion, as publication can
precede finalization today. The installing command returns an error until
completion succeeds. Extend receipt loading explicitly for the new publication
format; project only completed receipts and preserve legacy bytes without
inferred ownership. Recovery after interrupted selector replacement must
classify its exact preimage/postimage rather than resolve a new release.

## Legacy enrollment and Herdr state

Legacy planning must explain that one migration is required. `--yes` alone does
not authorize moving the old public link. Provide a reviewed operator procedure
to preserve/move it, then enroll the new layout. Publish the stable public entry
only when its name is absent; a late foreign occupant remains untouched. Keep
the old bundle and receipt. Verify the desired artifact afresh instead of
inventing modern authority from old markers.

Fresh enrollment is itself journaled: prepare the runtime, initial generation
and selector before publishing the absent public command. Completion requires
the entire surface. Preserve a conflicting runtime directory. An interrupted
enrollment resumes captured authority instead of constructing another runtime.

Executable activation does not upgrade Herdr application state. Existing
[compatibility guidance](herdr.md#runtime-adoption-and-rollback) requires
preserving configuration, plugins, session state and history. A new client does
not replace a running server or downgrade its state. Provisioners must wait
until the desired Dependency is satisfied; restarting/migrating live sessions
remains separately authorized.

## Acceptance and implementation gates

Before making #592 agent-ready, approve the permanent runtime reservation,
explicit legacy migration, native dispatcher distribution and supported
filesystem/durability policy. Record an ADR explicitly distinguishing legacy
publication from future private activation. This research is a proposal, not
that approval. Synchronization must match the chosen failure model, with
platform evidence; do not expand ADR 0021's durability claims implicitly.

Implementation acceptance must prove:

- Fresh enrollment and prior-to-new activation through dry-run and `--yes`,
  plus shared install/update/upgrade paths, with sandbox CLI invocations and
  fixture artifacts. Normal activation preserves the public symlink identity.
- Legacy migration required; absent-name enrollment rejects late foreign links.
  Reject foreign routing, PATH shadows, changed payloads, same-version checksum
  conflicts and unauthorized state roots without overwriting external state.
- Faults at capture, staging, final payload, selector preparation/replacement,
  synchronization and completion. Fresh-process retries consume the exact
  captured artifact and preserve unrelated receipts.
- Complete old/new reader observations; exact selector bytes including NULs;
  arguments/cwd/environment; paths with spaces/apostrophes; no dependency on a
  surviving dots installation; corrupt runtime errors.
- Concurrent dots calls across state roots, aliases, lock/ancestor substitution,
  pending epoch changes, completed-runtime use after reboot, and read-only
  diagnostics/dry runs that never repair or write state.
- Actual supported-platform filesystem tests, synchronization fault injection,
  and an explicit supported durability policy. Simulating another OS value on
  one host is insufficient evidence for that kernel.

Scope the first implementation to fixed `herdr-custom`. Generic upgrades,
rolling-policy changes, garbage collection, state relocation, automatic legacy
replacement, live-session migration and application-state rollback are excluded.
