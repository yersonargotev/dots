# dots

`dots` is a safe Dotfiles CLI for installing repository-owned shell, git, starship, and tmux configuration. It plans before changing files, detects conflicts, and keeps install behavior inside the tested Go CLI instead of duplicating it in shell scripts.

## Quick install

Choose one install path. Both paths install the same checksum-verified GitHub Release Artifact for your platform.

### Homebrew

```bash
brew install yersonargotev/tap/dots
```

If you prefer tapping first, run `brew tap yersonargotev/tap` and then `brew install dots`.

Verify the binary is available:

```bash
dots --help
```

Then run the installer from the CLI:

```bash
dots install
```

### Bootstrapper

```bash
curl -fsSL https://raw.githubusercontent.com/yersonargotev/dots/main/scripts/install.sh | DOTS_VERSION=v0.1.0 bash
```

The Bootstrapper downloads `checksums.txt`, verifies the matching Release Artifact, installs `dots` to `~/.local/bin/dots`, and delegates setup to `dots install`.

## Release artifacts

Each release publishes raw binaries for:

| Platform | Artifact suffix |
|----------|-----------------|
| macOS amd64 | `darwin_amd64` |
| macOS arm64 | `darwin_arm64` |
| Linux amd64 | `linux_amd64` |
| Linux arm64 | `linux_arm64` |

Homebrew and the Bootstrapper use those same artifacts plus the published SHA-256 manifest.

## Maintainer docs

- [`docs/release.md`](docs/release.md) — release workflow, checksums, Homebrew tap automation, and Bootstrapper install.
- [`docs/scope.md`](docs/scope.md) — current scope boundaries and deferred work.
- [`CONTEXT.md`](CONTEXT.md) — domain vocabulary and architecture context.
