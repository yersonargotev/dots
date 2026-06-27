# Issue Tracker

This repository tracks work in GitHub Issues.

## Repository

- Owner: `yersonargotev`
- Repository: `dotfiles`

## Workflow

- Product requirements and implementation-ready work should be published as GitHub Issues.
- PRD issues should use the `ready-for-agent` label once they are sufficiently specified for an agent to implement without additional human context.
- Use the GitHub CLI (`gh`) when creating or updating issues from automation.
- Authenticate before agent work that touches GitHub: run `gh auth login` for an interactive workstation, or export `GH_TOKEN`/`GITHUB_TOKEN` in non-interactive environments.

## Notes

The current local git remote may still point to `yersonargotev/dots` until the repository rename is completed. The canonical project name is `dotfiles`.
