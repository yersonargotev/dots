---
name: to-questionnaire
description: Turn a decision you can't fully answer into a questionnaire for someone else to fill in.
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

Read and follow the [source procedure](SKILL.md), including its branch references.
