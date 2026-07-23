# Publish a v0.x Release

This workflow publishes `dots` Release Artifacts for macOS and Linux, then updates the Homebrew tap from the same checksum manifest. The Bootstrapper and Homebrew formula use the same raw binaries, so package distribution never becomes a second source of install logic.

## Quick path

1. Confirm the release candidate passes validation:
   ```bash
   go test ./...
   ```
2. Create and push a v0.x tag:
   ```bash
   git tag v0.5.1
   git push origin v0.5.1
   ```
3. Open the GitHub release created by the `Release` workflow and verify these assets exist:
   - `dots_v0.5.1_darwin_amd64`
   - `dots_v0.5.1_darwin_arm64`
   - `dots_v0.5.1_linux_amd64`
   - `dots_v0.5.1_linux_arm64`
   - `checksums.txt`
4. Verify the tap repository has an updated `Formula/dots.rb` commit for the same tag.

## Manual dispatch

Use manual dispatch when the tag already exists but the release needs to be rebuilt or re-uploaded.

1. Go to **Actions → Release → Run workflow**.
2. Enter the existing tag, for example `v0.5.1`.
3. Run the workflow. It checks out that tag, rebuilds the four artifacts, recreates `checksums.txt`, and uploads the GitHub Release assets with `--clobber`.
4. Approve the pending `homebrew` deployment. The protected job downloads the published checksum manifest, verifies `HOMEBREW_TAP_TOKEN` is present, regenerates and locally commits `Formula/dots.rb` when changed, dry-run proves access, and pushes the prepared tap commit.

## Checksum Verification contract

`checksums.txt` uses the standard SHA-256 manifest format:

```text
<sha256>  dots_<version>_<goos>_<goarch>
```

A Bootstrapper can verify the downloaded artifact by selecting the line for its platform and comparing the SHA-256 of the downloaded binary before installing or executing it.

Local maintainer check:

```bash
scripts/build-release-artifacts.sh --version v0.5.1 --out-dir dist
cd dist
shasum -a 256 -c checksums.txt
```

On Linux, `sha256sum -c checksums.txt` is equivalent. To validate Homebrew's future Tap Trust behavior locally, run install checks with `HOMEBREW_REQUIRE_TAP_TRUST=1` and use either the fully-qualified formula install or explicit `brew trust --formula yersonargotev/tap/dots`.

## Homebrew install

Install the latest tap formula with a fully-qualified formula name:

```bash
brew install yersonargotev/tap/dots
dots --version
dots install --profile workstation
```

First-run installs require an explicit Profile. Repeat `--profile` to compose
selections: `workstation` covers `core + desktop + agents`, while `web` and
`mobile` remain opt-ins.

This is the preferred Tap Trust path for `dots`: Homebrew trusts only the formula being installed instead of trusting every current and future formula, cask, or external command in `yersonargotev/tap`.

If you keep the tap installed and want short-name installs, trust the formula before installing from the tap:

```bash
brew tap yersonargotev/tap
brew trust --formula yersonargotev/tap/dots
brew install dots
```

Trust the whole tap with `brew trust yersonargotev/tap` only when you intentionally accept the broader blast radius. Homebrew currently allows non-official taps by default, but Tap Trust will require explicit trust in a future Homebrew release; maintainers can test the stricter behavior now with `HOMEBREW_REQUIRE_TAP_TRUST=1`.

For Brewfile-managed machines, prefer formula-level trust:

```ruby
brew "yersonargotev/tap/dots", trusted: true
```

The formula selects the correct Release Artifact for macOS/Linux and amd64/arm64, marks each raw executable URL with `using: :nounzip`, verifies the published SHA-256 checksum from the generated formula, installs the downloaded binary as `dots`, and runs `dots --version` in `brew test`.

The release workflow publishes the GitHub Release first, then gates the separate tap update behind the protected `homebrew` environment:

1. The release job builds and uploads the four binaries plus `checksums.txt`, preserving the checksum manifest as a workflow artifact.
2. The dependent Homebrew job waits for approval of the `homebrew` environment, then reads its `HOMEBREW_TAP_TOKEN` environment secret and checks out `yersonargotev/homebrew-tap`.
3. It generates `Formula/dots.rb` with `scripts/generate-homebrew-formula.sh`. The script reads the transferred `dist/checksums.txt` and requires exactly the four supported artifact entries.
4. It stages and commits the formula update locally when the tap formula changed, dry-run proves that prepared state can be pushed, and then publishes the tap commit.

Maintainer setup:

- Create or maintain the `yersonargotev/homebrew-tap` repository.
- Configure a protected `homebrew` environment that requires maintainer approval and permits `main` plus release tags matching `v0.*`.
- Store a fine-grained token with write access only to that tap as the environment secret `HOMEBREW_TAP_TOKEN`.
- Do not use `GITHUB_TOKEN` for the tap push; it is scoped to this repository.

## Bootstrapper install

Install the latest published v0.x release with the checksum-verified Bootstrapper:

```bash
curl -fsSL https://raw.githubusercontent.com/yersonargotev/dots/main/scripts/install.sh | bash
```

The Bootstrapper downloads `checksums.txt` and the matching platform artifact from the latest GitHub Release by default, verifies the SHA-256 checksum, installs the executable as `~/.local/bin/dots`, clones the Source of Truth to `~/.local/share/dots` when needed, and then uses that tested binary for setup. When `DOTS_VERSION=vX.Y.Z` is pinned, the default Source of Truth clone uses the same Git ref; `latest` keeps using the repository default branch. Before any real-home install, verify the installed binary version:

```bash
~/.local/bin/dots --version
~/.local/bin/dots install --profile workstation
```

For development checkouts, pass the Installed Repository override through the Bootstrapper instead of duplicating install behavior in shell:

```bash
DOTS_VERSION=v0.5.1 DOTS_SOURCE_ROOT="$PWD" bash scripts/install.sh
```

`DOTS_VERSION` is optional. Use it only when you intentionally need to pin or test a specific release tag. Set `DOTS_REPOSITORY_REF` only when the release artifact and Source of Truth ref intentionally differ.

## Release details

| Topic | Decision |
|-------|----------|
| Trigger | Push tags matching `v0.*`, or run the workflow manually with an existing v0.x tag. |
| Artifacts | Raw `dots` binaries named `dots_<version>_<goos>_<goarch>`. |
| Platforms | `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and `linux/arm64`. |
| Checksums | One `checksums.txt` file covers every Release Artifact. |
| Homebrew Distribution | `scripts/generate-homebrew-formula.sh` generates `Formula/dots.rb` from the release tag and transferred `checksums.txt`; after the GitHub Release assets exist, the environment-protected Homebrew job locally prepares the tap commit, dry-run proves that prepared state can be pushed, then publishes it to `yersonargotev/homebrew-tap` with the environment `HOMEBREW_TAP_TOKEN`. Users should prefer formula-level Tap Trust through `brew install yersonargotev/tap/dots`, `brew trust --formula yersonargotev/tap/dots`, or Brewfile `trusted: true`. Homebrew installs only the binary, so first-run package-manager installs must run `dots init` to clone the default Installed Repository before `dots status`, `dots doctor`, or `dots install --profile workstation --dry-run`. |

## First v0.x checklist

- [ ] The tag is a v0.x tag, such as `v0.5.1`.
- [ ] The workflow completed from the tag commit.
- [ ] The pending `homebrew` deployment was approved by the required reviewer.
- [ ] All four platform artifacts are attached to the GitHub Release.
- [ ] `checksums.txt` contains one SHA-256 entry for each artifact.
- [ ] The Bootstrapper maps its detected `goos/goarch` to the matching artifact name.
- [ ] `dots --version` and `dots version` both report the release tag.
- [ ] A Homebrew-installed binary can run `dots init` and then read the default Installed Repository manifest.
- [ ] `Formula/dots.rb` in `yersonargotev/homebrew-tap` points at the same tag and checksums.
- [ ] A Tap Trust install path has been checked with `HOMEBREW_REQUIRE_TAP_TRUST=1` using the fully-qualified formula or formula-level trust.
- [ ] `HOMEBREW_TAP_TOKEN` is configured as an environment secret before relying on automatic tap updates.
