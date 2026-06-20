---
name: dots-release
description: Release workflow for the dots repository. Use when asked to publish a new dots release, create or verify a v0.x tag, clean up a merged release branch, inspect release workflow status, verify GitHub Release assets, or confirm the Homebrew tap formula after a release.
---

# Dots Release

Run the dots release path safely and verify every external side effect. This repository publishes releases by pushing `v0.*` tags; the GitHub `Release` workflow builds artifacts, uploads checksums, and updates `yersonargotev/homebrew-tap`.

## Preconditions

- Work from `/Users/argote/Documents/dev/yersonargotev/dots` unless the user explicitly names another checkout.
- Read `docs/release.md` and `.github/workflows/release.yml` before changing tags or releases.
- Respect `AGENTS.md`: no AI attribution, conventional commits only, and never validate dotfiles against the real home directory.
- Do not overwrite or discard unrelated local changes. If the working tree is dirty, identify unrelated files and leave them untouched.

## Standard workflow

1. Resolve the target change.
   - If the user references a PR, verify it is merged before cleanup or release.
   - Inspect PR status with `gh pr view <number> --json state,mergedAt,mergeCommit,headRefName,baseRefName,url`.

2. Clean merged branches.
   - Switch to `main`, then fast-forward from origin:
     ```bash
     git switch main
     git pull --ff-only origin main
     git fetch --prune origin
     ```
   - Delete the remote branch only if it still exists:
     ```bash
     git ls-remote --exit-code --heads origin <branch>
     git push origin --delete <branch>
     ```
   - Delete the local branch. If the PR was squash-merged, `git branch -d` may fail because the branch tip is not an ancestor of `main`; after verifying the PR is merged, use `git branch -D <branch>`.

3. Validate before tagging.
   - Run at least:
     ```bash
     go test ./...
     ```
   - For higher-risk releases, also run the CI-equivalent set from `AGENTS.md`: `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`.

4. Choose the next tag.
   - List existing tags/releases:
     ```bash
     git tag --sort=-v:refname | head -20
     gh release list --repo yersonargotev/dots --limit 10
     ```
   - Use a `v0.x.y` tag accepted by the workflow regex. Prefer patch bump for bug fixes and minor bump for new user-visible features.
   - Verify the chosen tag does not exist locally or remotely before creating it.

5. Create and push the tag.
   ```bash
   git tag v0.x.y
   git push origin v0.x.y
   ```

6. Watch the release workflow to completion.
   ```bash
   gh run list --repo yersonargotev/dots --workflow Release --limit 5 \
     --json databaseId,status,conclusion,headBranch,url,event
   gh run watch <run-id> --repo yersonargotev/dots --exit-status
   ```

7. Verify the GitHub Release.
   - Confirm the release is not draft/prerelease unless explicitly intended.
   - Confirm exactly these expected assets exist for the tag:
     - `dots_<tag>_darwin_amd64`
     - `dots_<tag>_darwin_arm64`
     - `dots_<tag>_linux_amd64`
     - `dots_<tag>_linux_arm64`
     - `checksums.txt`
   - Use:
     ```bash
     gh release view <tag> --repo yersonargotev/dots \
       --json tagName,name,isDraft,isPrerelease,url,assets,publishedAt,targetCommitish
     ```

8. Verify the Homebrew tap.
   ```bash
   gh api repos/yersonargotev/homebrew-tap/contents/Formula/dots.rb \
     --jq '.content' | base64 --decode | grep -n '<tag>\|sha256'
   ```

9. Report concise results.
   - Include the release URL, workflow result, branch cleanup status, and any untouched local changes.
   - If any release step fails, stop and report the failing workflow URL/log step before attempting repairs.

## Guardrails

- Never create a release from a stale local `main`; fast-forward first.
- Never force-push or retag a published release unless the user explicitly asks and understands the blast radius.
- Never delete a branch just because it “looks merged”; verify PR merge state or ancestry first.
- Never treat a successful tag push as a complete release; the workflow, assets, and Homebrew tap must be verified.
