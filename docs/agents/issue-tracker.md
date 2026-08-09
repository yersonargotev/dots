# Issue Tracker

This repository tracks work in GitHub Issues.

## Repository

- Owner: `yersonargotev`
- Repository: `dotfiles`
- `delivery-issue` accepts `N`, `#N`, or a GitHub issue URL. Short identifiers resolve through the active repository reported by `gh repo view`; URLs must resolve to that same repository identity, including redirects caused by a repository rename. Reject cross-repository URLs before making changes.

## Workflow

- Product requirements and implementation-ready work should be published as GitHub Issues.
- Sliced work uses GitHub's native parent/sub-issue and blocking relationships
  whenever the supported CLI exposes them. Textual relationship sections are
  only readable mirrors or an explicitly reported unsupported-platform
  fallback, never the source of truth.
- A Delivery Contract may come from a complete historical Agent Brief, a
  complete standalone issue body, or a delivery ticket composed with its native
  parent specification and relationships. See
  [`docs/agents/delivery-contract.md`](delivery-contract.md).
- `ready-for-agent` records specification completeness. Native blockers decide
  whether a Delivery Unit belongs to the Execution Frontier without changing
  that readiness label. Issues do not need a `type:*` label; PRs do.
- [`workflows/delivery-issue.md`](../../workflows/delivery-issue.md) is the single source of truth for taking one approved issue through squash merge and green post-merge CI on `main`. There is no separate delivery fast path.
- A Tracking Issue returns a read-only `tracking` result that identifies
  executable child Delivery Units and creates no branch or PR.
- Issue creation prepares delivery input. Release publication remains a
  separate, potentially batched workflow after delivery.
- Public issue templates intentionally do not auto-apply labels; maintainers/admins apply triage labels after creation.
- The `Maintainer governance` workflow removes issue/PR labels added by non-maintainers and reopens PRs closed without merge by non-maintainers.
- Use the GitHub CLI (`gh`) when creating or updating issues from automation.
- The direct planning path is `to-spec` → `to-tickets` → `delivery-issue` when
  those external skills publish complete contracts and native relationships.
  No mandatory retriage or duplicate Agent Brief synthesis sits between those
  steps.
- `dots-issue-creation` remains the repository-specific direct publication
  workflow for templates, duplicate detection, labels, native relationships,
  and verification. It is not a mandatory post-processor for external planning
  skills.
- External `grill-with-docs`, `to-spec`, `to-tickets`, and `triage` skills remain
  unmodified and unvendored by this repository.
- Authenticate before agent work that touches GitHub: run `gh auth login` for an interactive workstation, or export `GH_TOKEN`/`GITHUB_TOKEN` in non-interactive environments.

## Notes

The current local git remote may still point to `yersonargotev/dots` until the repository rename is completed. The canonical project name is `dotfiles`.
