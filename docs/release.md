# Publish a v0.x Release

This workflow publishes Bootstrapper-ready `dots` Release Artifacts for macOS and Linux. Each release includes one raw binary per Supported Platform plus `checksums.txt` for Checksum Verification.

## Quick path

1. Confirm the release candidate passes validation:
   ```bash
   go test ./...
   ```
2. Create and push a v0.x tag:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```
3. Open the GitHub release created by the `Release` workflow and verify these assets exist:
   - `dots_v0.1.0_darwin_amd64`
   - `dots_v0.1.0_darwin_arm64`
   - `dots_v0.1.0_linux_amd64`
   - `dots_v0.1.0_linux_arm64`
   - `checksums.txt`

## Manual dispatch

Use manual dispatch when the tag already exists but the release needs to be rebuilt or re-uploaded.

1. Go to **Actions → Release → Run workflow**.
2. Enter the existing tag, for example `v0.1.0`.
3. Run the workflow. It checks out that tag, rebuilds the four artifacts, recreates `checksums.txt`, and uploads assets with `--clobber`.

## Checksum Verification contract

`checksums.txt` uses the standard SHA-256 manifest format:

```text
<sha256>  dots_<version>_<goos>_<goarch>
```

A Bootstrapper can verify the downloaded artifact by selecting the line for its platform and comparing the SHA-256 of the downloaded binary before installing or executing it.

Local maintainer check:

```bash
scripts/build-release-artifacts.sh --version v0.1.0 --out-dir dist
cd dist
shasum -a 256 -c checksums.txt
```

On Linux, `sha256sum -c checksums.txt` is equivalent.

## Release details

| Topic | Decision |
|-------|----------|
| Trigger | Push tags matching `v0.*`, or run the workflow manually with an existing v0.x tag. |
| Artifacts | Raw `dots` binaries named `dots_<version>_<goos>_<goarch>`. |
| Platforms | `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and `linux/arm64`. |
| Checksums | One `checksums.txt` file covers every Release Artifact. |
| Homebrew Distribution | Deferred. Do not add tap, formula, or bottle steps in this slice. |

## First v0.x checklist

- [ ] The tag is a v0.x tag, such as `v0.1.0`.
- [ ] The workflow completed from the tag commit.
- [ ] All four platform artifacts are attached to the GitHub Release.
- [ ] `checksums.txt` contains one SHA-256 entry for each artifact.
- [ ] The Bootstrapper can later map its detected `goos/goarch` to the matching artifact name.
- [ ] No Homebrew Distribution step was added.
