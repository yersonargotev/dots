# Publish a v0.x Release

This workflow publishes `dots` Release Artifacts for macOS and Linux, then updates the Homebrew tap from the same checksum manifest. The Bootstrapper and Homebrew formula use the same raw binaries, so package distribution never becomes a second source of install logic.

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
4. Verify the tap repository has an updated `Formula/dots.rb` commit for the same tag.

## Manual dispatch

Use manual dispatch when the tag already exists but the release needs to be rebuilt or re-uploaded.

1. Go to **Actions → Release → Run workflow**.
2. Enter the existing tag, for example `v0.1.0`.
3. Run the workflow. It checks out that tag, rebuilds the four artifacts, recreates `checksums.txt`, verifies `HOMEBREW_TAP_TOKEN` is present, checks out the Homebrew tap, regenerates and locally commits `Formula/dots.rb` when changed, dry-run pushes that prepared tap state, uploads assets with `--clobber`, and only then pushes the prepared tap commit.

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

## Homebrew install

Install the latest tap formula:

```bash
brew install yersonargotev/tap/dots
dots --help
dots install
```

Equivalent tapped form: `brew tap yersonargotev/tap`, then `brew install dots`.

The formula selects the correct Release Artifact for macOS/Linux and amd64/arm64, marks each raw executable URL with `using: :nounzip`, verifies the published SHA-256 checksum from the generated formula, installs the downloaded binary as `dots`, and runs `dots --help` in `brew test`.

The release workflow proves tap write access before it mutates the public GitHub Release, but it does not publish the tap update until the release assets exist:

1. It verifies `HOMEBREW_TAP_TOKEN` is present and checks out `yersonargotev/homebrew-tap`.
2. It generates `Formula/dots.rb` with `scripts/generate-homebrew-formula.sh`. The script reads `dist/checksums.txt` and requires exactly the four supported artifact entries.
3. It stages and commits the formula update locally when the tap formula changed.
4. It dry-run pushes the prepared local tap state before creating or uploading GitHub Release assets.
5. After the release assets upload succeeds, it pushes the already-prepared tap commit, or no-ops if the formula already matched the tag.

Maintainer setup:

- Create or maintain the `yersonargotev/homebrew-tap` repository.
- Store a token with write access to that tap as this repository secret: `HOMEBREW_TAP_TOKEN`.
- Do not use `GITHUB_TOKEN` for the tap push; it is scoped to this repository.

## Bootstrapper install

Install a published v0.x release with the checksum-verified Bootstrapper:

```bash
curl -fsSL https://raw.githubusercontent.com/yersonargotev/dots/main/scripts/install.sh | DOTS_VERSION=v0.1.0 bash
```

The Bootstrapper downloads `checksums.txt` and the matching platform artifact from GitHub Releases, verifies the SHA-256 checksum, installs the executable as `~/.local/bin/dots`, and then delegates setup to:

```bash
~/.local/bin/dots install
```

For development checkouts, pass the Installed Repository override through the Bootstrapper instead of duplicating install behavior in shell:

```bash
DOTS_VERSION=v0.1.0 DOTS_SOURCE_ROOT="$PWD" bash scripts/install.sh
```

## Release details

| Topic | Decision |
|-------|----------|
| Trigger | Push tags matching `v0.*`, or run the workflow manually with an existing v0.x tag. |
| Artifacts | Raw `dots` binaries named `dots_<version>_<goos>_<goarch>`. |
| Platforms | `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and `linux/arm64`. |
| Checksums | One `checksums.txt` file covers every Release Artifact. |
| Homebrew Distribution | `scripts/generate-homebrew-formula.sh` generates `Formula/dots.rb` from the release tag and `checksums.txt`; the workflow locally prepares the tap commit, dry-run proves that prepared state can be pushed, then pushes it to `yersonargotev/homebrew-tap` with `HOMEBREW_TAP_TOKEN` after release assets upload. |

## First v0.x checklist

- [ ] The tag is a v0.x tag, such as `v0.1.0`.
- [ ] The workflow completed from the tag commit.
- [ ] All four platform artifacts are attached to the GitHub Release.
- [ ] `checksums.txt` contains one SHA-256 entry for each artifact.
- [ ] The Bootstrapper maps its detected `goos/goarch` to the matching artifact name.
- [ ] `Formula/dots.rb` in `yersonargotev/homebrew-tap` points at the same tag and checksums.
- [ ] `HOMEBREW_TAP_TOKEN` is configured before relying on automatic tap updates.
