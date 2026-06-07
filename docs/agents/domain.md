# Domain Docs

This repository uses a single-context domain documentation layout.

## Layout

- `CONTEXT.md` — canonical domain glossary for the dotfiles distribution context.
- `docs/adr/` — architectural decisions for the project.

## Consumer Rules

- Read `CONTEXT.md` before writing PRDs, issues, implementation plans, or architecture reviews.
- Use the glossary terms consistently, especially `Source of Truth`, `Install Manifest`, `Managed Entry`, `Install Plan`, `Conflict`, `Backup Set`, and `Drift`.
- Read relevant ADRs under `docs/adr/` before proposing changes that affect architecture or workflow.
- Do not treat `CONTEXT.md` as a spec; it is a glossary.
