---
status: accepted
---

# Keep Herdr task identity in a dedicated plugin and distribute it through dots

The user confirmed this ownership boundary during the design interview for
[issue #543](https://github.com/yersonargotev/dots/issues/543): a dedicated
Herdr plugin owns stable task identity and its runtime state, while dots
provisions a pinned plugin revision and manages portable presentation and
preferences. Codex and Claude propose a human-readable task name through one
shared producer contract. This separates semantic task intent from Tabby's
foreground-command/directory labels and from the Git provider's workspace
metadata. The primary-source findings and alternatives are recorded in the
[research note](../herdr-attention-ownership-research.md).

A task can span multiple prompts in one conversation. Its displayed name stays
stable through questions, tests, and continuations; it changes automatically
when a distinct task starts. A human correction wins during that task and is
released at the next task. The name and its correction survive Herdr restart
and resume of the same conversation, while a different conversation using the
same pane must not inherit them. The display preserves the engine and adds the
human task name on a separate row. Codex and Claude are first-delivery
requirements; other engines may reuse the contract when that adds negligible
work.

## Why a separate owner

Configuration can render a token but cannot produce semantic task intent or
own the requested restoration and correction policy. In the inspected Herdr
0.9.3 baseline, custom tokens are not cold-restored, and saved ordinary agent
aliases are skipped by the normal cold native-resume path. A pane name alone
also lacks conversation/task isolation. A dedicated identity component can own
that lifecycle without changing Herdr's semantic agent state. Packaging it as
a Herdr plugin supplies declared startup/actions, an application-owned state
location, and an existing reviewed distribution path through dots.

A standalone helper remains a possible internal implementation of the plugin;
this decision chooses the owner and package, not a language, resident daemon,
or storage schema. A bare instruction that writes a token is insufficient for
the chosen persistence contract. Expanding Tabby would couple its focused-tab
naming and manual tab locks to a different cross-agent task lifecycle. Adding
that behavior to the Git provider would combine unrelated state and freshness
requirements. Keeping the implementation in dots would turn the distributor
into a runtime task service. These alternatives were not selected.

## Existing Auto Title alternative

The subsequent [Auto Title comparison](../herdr-auto-title-comparison-research.md)
evaluates `kryptamine/herdr-auto-title` at v0.13.1 and a release-qualified HEAD.
It already names panes and tabs and reports workspace activity, so a general
contextual-title product would not justify rebuilding those capabilities.
It does not implement this decision's task-boundary and correction contract:
activity follows available titles/transcripts, Claude has the only native
transcript extractor, and manual locks are resource-ID/label records rather
than native conversation/task records. Both it and Tabby also write tab names.

This evidence does not make a new general auto-renamer necessary. Keep the
selected task owner narrow, and assess an upstream task mode if that project
wants the same semantics. Reusing Auto Title unchanged would require a
separate product decision to relax stable task boundaries, correction lifetime,
and conversation isolation. The ownership choice remains accepted; no new
plugin implementation is authorized by this comparison.

## Consequences and boundaries

The plugin persists and reconciles Task Names, task transitions, and protected
manual corrections against validated native Conversation Identity. The agents
supply the semantic new-task signal; the plugin does not guess it from cwd,
process names, screen scraping, or idle transitions. Stored task data is
Application-Owned Runtime State, outside Managed Entries, Installation Metadata,
and the Installed Repository. dots owns the portable configuration and the
reviewed installation declaration. Herdr owns rendering, navigation, native
conversation reporting, and lifecycle state. Follow [ADR 0017](0017-keep-application-writable-targets-outside-the-installed-repository.md)
and [ADR 0018](0018-atomic-capability-tags-and-profile-presets.md) for filesystem
ownership and installation selection.

The first delivery covers readability and task identity. Native attention
filtering already exists in Herdr 0.9.3; explicit human review, new core UI
controls, and a review queue are later design work. Publishing display metadata
must not claim that a task succeeded or replace Herdr's lifecycle authority.

This ADR records an accepted architecture, not a completed implementation or
authorization to change live configuration. Producer support, lifecycle timing,
concurrency, persistence, missing-identity behavior, and sandboxed acceptance
still require evidence in subsequent implementation specifications. No plugin
repository, concrete API spelling, storage format, or version is selected here.
