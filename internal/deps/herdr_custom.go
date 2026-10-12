package deps

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// This launcher leaves the published launcher and its provenance intact.
const herdrCustomLauncher = `#!/bin/sh
set -eu
script=$0
while [ -L "$script" ]; do
  directory=$(CDPATH='' cd -- "$(dirname -- "$script")" && pwd)
  target=$(readlink "$script")
  case "$target" in /*) script=$target ;; *) script=$directory/$target ;; esac
done
root=$(CDPATH='' cd -- "$(dirname -- "$script")/.." && pwd)
if [ -z "${HERDR_CONFIG_PATH:-}" ]; then
  HERDR_CONFIG_PATH=$HOME/.config/herdr/config.toml
  export HERDR_CONFIG_PATH
fi
exec "$root/bin/herdr-custom" "$@"
`

var herdrCustomVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+-custom\.[0-9]+$`)

func isHerdrCustom(dep manifest.Dependency) bool {
	return dep.UserLocal != nil && (dep.UserLocal.Recipe == "herdr-custom" || dep.UserLocal.Recipe == "" && dep.Name == "herdr-custom")
}

func herdrCustomArtifact(dep manifest.Dependency, opts Options) (UserLocalArtifact, bool, error) {
	arch := opts.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	target := ""
	switch {
	case opts.OS == "darwin" && arch == "arm64":
		target = "macos-aarch64"
	case opts.OS == "linux" && arch == "amd64":
		target = "linux-x86_64"
	default:
		return UserLocalArtifact{}, false, fmt.Errorf("herdr-custom supports only darwin/arm64 and linux/amd64, got %s/%s", opts.OS, arch)
	}
	version := strings.TrimSpace(dep.UserLocal.Version)
	if !herdrCustomVersion.MatchString(version) {
		return UserLocalArtifact{}, false, fmt.Errorf("invalid herdr-custom version %q", version)
	}
	checksum := dep.UserLocal.Checksums[platformKey(opts.OS, arch)]
	if len(checksum) != 64 {
		return UserLocalArtifact{}, false, fmt.Errorf("herdr-custom requires a per-platform SHA256 for %s", platformKey(opts.OS, arch))
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("invalid herdr-custom SHA256: %w", err)
	}
	archive := "herdr-custom-" + strings.TrimPrefix(version, "v") + "-" + target + ".tar.gz"
	home := opts.Home
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return UserLocalArtifact{}, false, fmt.Errorf("resolve herdr-custom home: %w", err)
		}
	}
	destination, installed := userLocalPaths(home, "herdr-custom", version, userLocalLayoutBundle, "herdr-custom")
	return UserLocalArtifact{Recipe: "herdr-custom", Version: version, Artifact: archive, URL: "https://github.com/yersonargotev/herdr-custom/releases/download/" + version + "/" + archive, Checksum: checksum, Platform: platformKey(opts.OS, arch), Layout: userLocalLayoutBundle, Command: "herdr-custom", Destination: destination, InstalledPath: installed}, true, nil
}

func herdrCustomRoot(bundle string, a UserLocalArtifact) string {
	return filepath.Join(bundle, strings.TrimSuffix(a.archiveName(), ".tar.gz"))
}

func regularExecutable(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect custom executable %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("custom executable %s must be a regular executable file", path)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read custom file %s for checksum: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type herdrCustomIntegration struct {
	Checksum string `json:"archive_sha256"`
	Launcher string `json:"original_launcher_sha256"`
	Runtime  string `json:"runtime_sha256"`
}

func stageHerdrCustom(staging string, a UserLocalArtifact) error {
	root := herdrCustomRoot(staging, a)
	for _, p := range []string{"bin/herdr-custom", "libexec/herdr"} {
		if err := regularExecutable(filepath.Join(root, p)); err != nil {
			return fmt.Errorf("validate staged herdr-custom bundle: %w", err)
		}
	}
	// Provenance is published by the recipe and remains unchanged in the bundle.
	provenance, err := os.ReadFile(filepath.Join(root, "provenance.json"))
	if err != nil {
		return fmt.Errorf("read herdr-custom provenance: %w", err)
	}
	var published struct {
		BinarySHA256 string `json:"binary_sha256"`
	}
	if err := json.Unmarshal(provenance, &published); err != nil {
		return fmt.Errorf("decode herdr-custom provenance: %w", err)
	}
	binaryHash, err := fileSHA256(filepath.Join(root, "libexec/herdr"))
	if err != nil {
		return fmt.Errorf("hash custom runtime: %w", err)
	}
	if published.BinarySHA256 != binaryHash {
		return fmt.Errorf("herdr-custom runtime does not match published provenance")
	}
	launcherHash, err := fileSHA256(filepath.Join(root, "bin/herdr-custom"))
	if err != nil {
		return fmt.Errorf("hash original custom launcher: %w", err)
	}
	integration := filepath.Join(root, "dots")
	if _, err := os.Lstat(integration); err == nil {
		return fmt.Errorf("herdr-custom archive reserves unexpected dots integration path")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect staged integration path %s: %w", integration, err)
	}
	if err := os.Mkdir(integration, 0755); err != nil {
		return fmt.Errorf("create custom integration directory %s: %w", integration, err)
	}
	if err := os.WriteFile(filepath.Join(integration, "herdr-custom"), []byte(herdrCustomLauncher), 0755); err != nil {
		return fmt.Errorf("write custom integration launcher in %s: %w", integration, err)
	}
	marker, err := json.Marshal(herdrCustomIntegration{a.Checksum, launcherHash, binaryHash})
	if err != nil {
		return fmt.Errorf("encode custom integration marker: %w", err)
	}
	if err := os.WriteFile(filepath.Join(integration, "integration.json"), marker, 0644); err != nil {
		return fmt.Errorf("write custom integration marker in %s: %w", integration, err)
	}
	return nil
}

func validHerdrCustomBundle(a UserLocalArtifact) bool {
	root := herdrCustomRoot(a.Destination, a)
	for _, directory := range []string{a.Destination, root, filepath.Join(root, "dots"), filepath.Join(root, "bin"), filepath.Join(root, "libexec")} {
		info, err := os.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	launcher := filepath.Join(root, "dots/herdr-custom")
	for _, p := range []string{launcher, filepath.Join(root, "bin/herdr-custom"), filepath.Join(root, "libexec/herdr")} {
		if regularExecutable(p) != nil {
			return false
		}
	}
	data, err := os.ReadFile(launcher)
	if err != nil || string(data) != herdrCustomLauncher {
		return false
	}
	marker, err := readHerdrCustomIntegration(root)
	if err != nil || marker.Checksum != a.Checksum {
		return false
	}

	lh, err := fileSHA256(filepath.Join(root, "bin/herdr-custom"))
	if err != nil || lh != marker.Launcher {
		return false
	}
	bh, err := fileSHA256(filepath.Join(root, "libexec/herdr"))
	return err == nil && bh == marker.Runtime
}

func herdrCustomPresent(dep manifest.Dependency, opts Options) bool {
	a, _, err := herdrCustomArtifact(dep, opts)
	if err != nil || !validHerdrCustomBundle(a) {
		return false
	}
	expected := filepath.Join(herdrCustomRoot(a.Destination, a), "dots/herdr-custom")
	target, err := os.Readlink(a.InstalledPath)
	if err != nil || target != expected {
		return false
	}
	resolved, err := exec.LookPath("herdr-custom")
	if err != nil {
		return false
	}
	resolved, err = filepath.Abs(resolved)
	return err == nil && resolved == a.InstalledPath
}

func herdrCustomInstallGuard(home string, a UserLocalArtifact) error {
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve custom install home: %w", err)
		}
	}
	a.Destination, a.InstalledPath = userLocalPaths(home, "herdr-custom", a.Version, userLocalLayoutBundle, "herdr-custom")
	conflict := func() error {
		return fmt.Errorf("herdr-custom integration conflict at %s; preserve or move the existing custom installation explicitly, then retry", a.InstalledPath)
	}
	if _, err := os.Lstat(a.InstalledPath); err == nil {
		if _, ok := priorHerdrCustomArtifact(a); !ok {
			return conflict()
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect custom public path %s: %w", a.InstalledPath, err)
	}
	if _, err := os.Lstat(a.Destination); err == nil && !validHerdrCustomBundle(a) {
		return conflict()
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect custom destination %s: %w", a.Destination, err)
	}
	if resolved, err := exec.LookPath("herdr-custom"); err == nil {
		absolute, err := filepath.Abs(resolved)
		if err != nil || absolute != a.InstalledPath {
			return fmt.Errorf("herdr-custom PATH shadow at %s; remove the shadow explicitly before installing the dots integration", resolved)
		}
	}
	return nil
}

// Read only regular markers; archive integrity and stored file hashes establish
// whether the prior bundle is an installation managed by this integration.
func readHerdrCustomIntegration(root string) (herdrCustomIntegration, error) {
	var marker herdrCustomIntegration
	path := filepath.Join(root, "dots/integration.json")
	info, err := os.Lstat(path)
	if err != nil {
		return marker, fmt.Errorf("read custom integration marker %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return marker, fmt.Errorf("custom integration marker must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return marker, fmt.Errorf("read custom integration marker %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &marker); err != nil {
		return marker, fmt.Errorf("decode custom integration marker %s: %w", path, err)
	}
	for _, hash := range []string{marker.Checksum, marker.Launcher, marker.Runtime} {
		if len(hash) != 64 {
			return marker, fmt.Errorf("invalid custom integration hash")
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return marker, fmt.Errorf("decode custom integration hash in %s: %w", path, err)
		}
	}
	return marker, nil
}

// A prior version is accepted only at the exact home-owned layout emitted by
// this provider. Never follow an arbitrary public link to discover provenance.
func priorHerdrCustomArtifact(desired UserLocalArtifact) (UserLocalArtifact, bool) {
	target, err := os.Readlink(desired.InstalledPath)
	if err != nil || !filepath.IsAbs(target) {
		return UserLocalArtifact{}, false
	}
	base := filepath.Dir(desired.Destination)
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return UserLocalArtifact{}, false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 4 || !herdrCustomVersion.MatchString(parts[0]) || parts[2] != "dots" || parts[3] != "herdr-custom" {
		return UserLocalArtifact{}, false
	}
	platform := ""
	switch desired.Platform {
	case "darwin_arm64":
		platform = "macos-aarch64"
	case "linux_amd64":
		platform = "linux-x86_64"
	default:
		return UserLocalArtifact{}, false
	}
	archive := "herdr-custom-" + strings.TrimPrefix(parts[0], "v") + "-" + platform + ".tar.gz"
	root := strings.TrimSuffix(archive, ".tar.gz")
	if parts[1] != root || target != filepath.Join(base, parts[0], root, "dots/herdr-custom") {
		return UserLocalArtifact{}, false
	}
	destination := filepath.Join(base, parts[0])
	for _, directory := range []string{destination, filepath.Join(destination, root), filepath.Join(destination, root, "dots"), filepath.Join(destination, root, "bin"), filepath.Join(destination, root, "libexec")} {
		info, err := os.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return UserLocalArtifact{}, false
		}
	}
	marker, err := readHerdrCustomIntegration(filepath.Join(destination, root))
	if err != nil {
		return UserLocalArtifact{}, false
	}
	prior := desired
	prior.Version = parts[0]
	prior.Artifact = archive
	prior.URL = "https://github.com/yersonargotev/herdr-custom/releases/download/" + prior.Version + "/" + archive
	prior.Destination = destination
	prior.Checksum = marker.Checksum
	if prior.Version == desired.Version && prior.Checksum != desired.Checksum {
		return UserLocalArtifact{}, false
	}
	return prior, validHerdrCustomBundle(prior)
}
