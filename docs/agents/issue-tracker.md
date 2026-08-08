# Issue Tracker

This repository tracks work in GitHub Issues.

## Repository

- Owner: `yersonargotev`
- Repository: `dotfiles`
- `delivery-issue` accepts `N`, `#N`, or a GitHub issue URL. Short identifiers resolve through the active repository reported by `gh repo view`; URLs must resolve to that same repository identity, including redirects caused by a repository rename. Reject cross-repository URLs before making changes.

## Workflow

- Product requirements and implementation-ready work should be published as GitHub Issues.
- `dots-issue-creation` is the sole producer of `ready-for-agent` issues. It applies the label only after creating or updating exactly one complete Agent Brief.
- `ready-for-agent` requires both the label and exactly one complete, internally consistent Agent Brief. See [`docs/agents/agent-brief.md`](agent-brief.md) for the repository-owned contract.
- [`workflows/delivery-issue.md`](../../workflows/delivery-issue.md) is the single source of truth for taking one approved issue through squash merge and green post-merge CI on `main`. There is no separate delivery fast path.
- Issue creation prepares delivery input. Release publication remains a separate, potentially batched workflow after delivery.
- Public issue templates intentionally do not auto-apply labels; maintainers/admins apply triage labels after creation.
- The `Maintainer governance` workflow removes issue/PR labels added by non-maintainers and reopens PRs closed without merge by non-maintainers.
- Use the GitHub CLI (`gh`) when creating or updating issues from automation.
- Authenticate before agent work that touches GitHub: run `gh auth login` for an interactive workstation, or export `GH_TOKEN`/`GITHUB_TOKEN` in non-interactive environments.

## Notes

The current local git remote may still point to `yersonargotev/dots` until the repository rename is completed. The canonical project name is `dotfiles`.
