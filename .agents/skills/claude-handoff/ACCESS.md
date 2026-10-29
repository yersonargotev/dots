---
name: claude-handoff
description: Hand the current conversation off to a fresh background agent that picks up the work immediately.
argument-hint: "What will the next session be used for?"
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

On both Codex and Claude Code, this procedure requires the external Claude CLI
and its `claude --bg --name` capability. Check those prerequisites before the
explicitly requested handoff. Run the selected command in the user's intended
target working directory. Obtaining this entry does not launch Claude or create
a Codex-native continuation.

Write a handoff summary of the current conversation so a fresh agent can continue the work. Instead of saving it, launch a background agent seeded with the summary as its prompt: `claude --bg --name "<descriptive name>" "<handoff summary>"`. It starts in the current working directory and returns immediately; the user manages it with `claude agents`.

Always pass `-n`/`--name` with a descriptive name (e.g. `--name "Fix login bug"`); it sets the display name shown in the job list, session picker, and terminal title.

Include a "suggested skills" section in the summary, naming qualified `matty:<skill>` recommendations selected for the next session, with the initiating explicit host, immutable snapshot, target directory, and returned resource pointers. The receiving agent obtains each selected resource with `packy get skill matty:<skill> --surface <initiating-surface> --snapshot <initiating-snapshot-id> --json`, preserving user-only invocation rules.

Do not duplicate content already captured in other artifacts (specs, plans, ADRs, issues, commits, diffs). Reference them by path or URL instead.

Redact any sensitive information, such as API keys, passwords, or personally identifiable information, since the summary becomes the agent's prompt.

If the user passed arguments, treat them as a description of what the next session will focus on and tailor the summary accordingly.
