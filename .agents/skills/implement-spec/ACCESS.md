---
name: implement-spec
description: "Implement the result of /to-spec and /to-tickets in code."
disable-model-invocation: true
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

You have been provided a spec. This spec should have tickets associated with it, describing how to implement the spec.

The issue tracker should have been provided to you. If not, tell the user to run `matty:setup-matt-pocock-skills` (obtain it only for an explicitly selected setup session using the same host and snapshot).

The goal is the entire spec implemented on a single **integration branch**, with every ticket resolved the way the issue tracker closes work.

The tickets are not a list of steps. They are a **task graph** with blocking relationships between them. This means there is always a **frontier** of tickets which are ready to be grabbed.

Communication to and from subagents should be sparse. Communicate primarily through **context pointers**: to the spec, tickets, research notes, and previous commits. Don't duplicate information already available via pointers.

**Implementer subagents** should be run in the background where possible for maximum concurrency.

## Steps

1. Read the spec and tickets to understand the task graph.

2. (optional) Use an **exploration subagent** to conduct any exploration required by the tickets - relevant codebase files or external documentation. Ensure the exploration subagent can save files - it should save its markdown notes in a directory outside the repo, accessible by all future subagents. This lets **implementer subagents** focus on implementation rather than exploration.

3. Create the integration branch. If the issue tracker closes work through PRs, or the user asks for one, open a draft PR after the first merge in step 5 (a branch with no commits ahead of main can't open one), marked as closing the spec and tickets.

4. Give every exploration, implementer, merger, and repair subagent the initiating
   explicit host and immutable snapshot identity, the user's target repository,
   its owned worktree and writable scratch/output, the specification/ticket
   pointers, and the returned Matty entry/resource locations. Implementers read
   and follow [matty:tdd]({{packy.access:matty:tdd}}) and final reviewers read and follow
   [matty:code-review]({{packy.access:matty:code-review}}). Every delegated nested retrieval uses
   `packy get skill matty:<skill> --surface <initiating-surface> --snapshot <initiating-snapshot-id> --json`.
   Missing retained resources require restoring that revision; native registration
   or another Pack's `tdd` is not a substitute.

   Use **implementer subagents** to implement each ticket, each in its own worktree on its own branch. Each implementer subagent:
   - confirms its worktree is based on the integration branch before starting, and resets onto it if not;
   - reads and follows [matty:tdd]({{packy.access:matty:tdd}}) to build the ticket, retaining its user-confirmed test seams;
   - merges the integration branch tip into its own branch before reporting done

5. Once an **implementer subagent** completes, merge its work to the integration branch with a **merger subagent**.

6. If this changes the **frontier** of available tickets, kick off more **implementer subagents** to work on the new tickets. This allows for maximum concurrency.

7. Once all tickets are complete, read and follow [matty:code-review]({{packy.access:matty:code-review}}) on the integration branch. Fix all issues raised by the code review in a single **implementer subagent**.

8. If a draft PR exists, mark it ready for review. Otherwise, resolve each ticket the way the issue tracker closes work, and report the integration branch.

9. Clean up all **implementer subagent** worktrees.
