package bootstrap_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBootstrapperInstallsVerifiedArtifactAndDelegatesToDotsInstall(t *testing.T) {
	root := repoRoot(t)
	version := "v0.99.0"
	artifact := "dots_v0.99.0_darwin_arm64"
	releaseRoot := t.TempDir()
	versionDir := filepath.Join(releaseRoot, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "dots-args.log")
	artifactBody := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\nprintf '%%s\\n' \"$*\" > %q\n", logPath)
	writeFile(t, filepath.Join(versionDir, artifact), artifactBody, 0o644)
	writeFile(t, filepath.Join(versionDir, "checksums.txt"), fmt.Sprintf("%s  %s\n", sha256Hex([]byte(artifactBody)), artifact), 0o644)

	server := httptest.NewServer(http.FileServer(http.Dir(releaseRoot)))
	t.Cleanup(server.Close)

	installDir := filepath.Join(t.TempDir(), "bin")
	sourceRoot := filepath.Join(t.TempDir(), "checkout")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"DOTS_VERSION="+version,
		"DOTS_RELEASE_BASE_URL="+server.URL,
		"DOTS_INSTALL_DIR="+installDir,
		"DOTS_SOURCE_ROOT="+sourceRoot,
		"DOTS_OS=darwin",
		"DOTS_ARCH=arm64",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap install failed: %v\n%s", err, output)
	}

	installed := filepath.Join(installDir, "dots")
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("expected installed dots binary: %v\noutput:\n%s", err, output)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed dots should be executable, mode=%v", info.Mode())
	}

	gotArgs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected bootstrapper to delegate to installed dots: %v\noutput:\n%s", err, output)
	}
	wantArgs := "install --source-root " + sourceRoot
	if strings.TrimSpace(string(gotArgs)) != wantArgs {
		t.Fatalf("delegated args = %q, want %q", strings.TrimSpace(string(gotArgs)), wantArgs)
	}
}

func TestBootstrapperDelegatesWithoutSourceRootForDefaultInstalledRepository(t *testing.T) {
	root := repoRoot(t)
	version := "v0.99.0"
	artifact := "dots_v0.99.0_linux_arm64"
	releaseRoot := t.TempDir()
	versionDir := filepath.Join(releaseRoot, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "dots-args.log")
	artifactBody := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\nprintf '%%s\\n' \"$*\" > %q\n", logPath)
	writeFile(t, filepath.Join(versionDir, artifact), artifactBody, 0o644)
	writeFile(t, filepath.Join(versionDir, "checksums.txt"), fmt.Sprintf("%s  %s\n", sha256Hex([]byte(artifactBody)), artifact), 0o644)

	server := httptest.NewServer(http.FileServer(http.Dir(releaseRoot)))
	t.Cleanup(server.Close)

	sourceRepo := newBootstrapSourceRepo(t)
	home := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"DOTS_VERSION="+version,
		"DOTS_RELEASE_BASE_URL="+server.URL,
		"DOTS_REPOSITORY_URL="+sourceRepo,
		"DOTS_INSTALL_DIR="+filepath.Join(t.TempDir(), "bin"),
		"DOTS_OS=linux",
		"DOTS_ARCH=arm64",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap install failed: %v\n%s", err, output)
	}

	gotArgs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected bootstrapper to delegate to installed dots: %v\noutput:\n%s", err, output)
	}
	if strings.TrimSpace(string(gotArgs)) != "install" {
		t.Fatalf("delegated args = %q, want %q", strings.TrimSpace(string(gotArgs)), "install")
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "share", "dots", "dots.yaml")); err != nil {
		t.Fatalf("expected bootstrapper to clone default Installed Repository: %v\noutput:\n%s", err, output)
	}
}

func TestBootstrapperDefaultsToLatestReleaseArtifactWhenVersionIsUnset(t *testing.T) {
	root := repoRoot(t)
	version := "v0.99.1"
	artifact := "dots_v0.99.1_darwin_arm64"
	releaseRoot := t.TempDir()
	latestDir := filepath.Join(releaseRoot, "latest")
	if err := os.MkdirAll(latestDir, 0o755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "dots-args.log")
	artifactBody := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\nprintf '%%s\\n' \"$*\" > %q\n", logPath)
	writeFile(t, filepath.Join(latestDir, artifact), artifactBody, 0o644)
	writeFile(t, filepath.Join(latestDir, "checksums.txt"), fmt.Sprintf("%s  %s\n", sha256Hex([]byte(artifactBody)), artifact), 0o644)

	server := httptest.NewServer(http.FileServer(http.Dir(releaseRoot)))
	t.Cleanup(server.Close)

	installDir := filepath.Join(t.TempDir(), "bin")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"DOTS_RELEASE_BASE_URL="+server.URL,
		"DOTS_INSTALL_DIR="+installDir,
		"DOTS_SOURCE_ROOT="+filepath.Join(t.TempDir(), "checkout"),
		"DOTS_OS=darwin",
		"DOTS_ARCH=arm64",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bootstrap install failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Downloading "+artifact) {
		t.Fatalf("bootstrap should resolve latest artifact %s, got:\n%s", artifact, output)
	}
	if !strings.Contains(string(output), "Downloading checksums for latest") {
		t.Fatalf("bootstrap should use latest release when DOTS_VERSION is unset, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(installDir, "dots")); err != nil {
		t.Fatalf("expected installed dots binary for %s: %v", version, err)
	}
	gotArgs, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected bootstrapper to delegate to installed dots: %v\noutput:\n%s", err, output)
	}
	if got := strings.TrimSpace(string(gotArgs)); !strings.HasPrefix(got, "install --source-root ") {
		t.Fatalf("delegated args = %q, want install with explicit source root", got)
	}
}

func TestBootstrapperRejectsChecksumMismatchBeforeInstallOrDelegation(t *testing.T) {
	root := repoRoot(t)
	version := "v0.99.0"
	artifact := "dots_v0.99.0_linux_amd64"
	releaseRoot := t.TempDir()
	versionDir := filepath.Join(releaseRoot, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "dots-args.log")
	artifactBody := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\nprintf '%%s\\n' \"$*\" > %q\n", logPath)
	writeFile(t, filepath.Join(versionDir, artifact), artifactBody, 0o644)
	writeFile(t, filepath.Join(versionDir, "checksums.txt"), fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), artifact), 0o644)

	server := httptest.NewServer(http.FileServer(http.Dir(releaseRoot)))
	t.Cleanup(server.Close)

	installDir := filepath.Join(t.TempDir(), "bin")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"DOTS_VERSION="+version,
		"DOTS_RELEASE_BASE_URL="+server.URL,
		"DOTS_INSTALL_DIR="+installDir,
		"DOTS_OS=linux",
		"DOTS_ARCH=amd64",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("bootstrap should reject checksum mismatch\n%s", output)
	}
	if !strings.Contains(string(output), "checksum mismatch") {
		t.Fatalf("checksum mismatch should be explained, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(installDir, "dots")); !os.IsNotExist(err) {
		t.Fatalf("checksum mismatch should not install dots; stat error: %v", err)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("checksum mismatch should not delegate to dots install; stat error: %v", err)
	}
}

func TestBootstrapperRejectsUnsupportedPlatformBeforeDownloading(t *testing.T) {
	root := repoRoot(t)
	releaseHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		releaseHits++
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"DOTS_VERSION=v0.99.0",
		"DOTS_RELEASE_BASE_URL="+server.URL,
		"DOTS_INSTALL_DIR="+filepath.Join(t.TempDir(), "bin"),
		"DOTS_OS=windows",
		"DOTS_ARCH=amd64",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("bootstrap should reject unsupported platforms\n%s", output)
	}
	if !strings.Contains(string(output), "unsupported operating system") {
		t.Fatalf("unsupported platform should be explained, got:\n%s", output)
	}
	if releaseHits != 0 {
		t.Fatalf("unsupported platform should fail before downloading; got %d release requests", releaseHits)
	}
}

func TestBootstrapperSelectsSupportedReleaseArtifacts(t *testing.T) {
	root := repoRoot(t)
	version := "v0.99.0"
	releaseRoot := t.TempDir()
	versionDir := filepath.Join(releaseRoot, version)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		os       string
		arch     string
		artifact string
	}{
		{name: "macos amd64", os: "Darwin", arch: "x86_64", artifact: "dots_v0.99.0_darwin_amd64"},
		{name: "macos arm64", os: "Darwin", arch: "arm64", artifact: "dots_v0.99.0_darwin_arm64"},
		{name: "linux amd64", os: "Linux", arch: "x86_64", artifact: "dots_v0.99.0_linux_amd64"},
		{name: "linux arm64", os: "Linux", arch: "aarch64", artifact: "dots_v0.99.0_linux_arm64"},
	}

	var checksumLines []string
	for _, tt := range cases {
		artifactBody := "#!/usr/bin/env bash\nset -euo pipefail\nexit 0\n"
		writeFile(t, filepath.Join(versionDir, tt.artifact), artifactBody, 0o644)
		checksumLines = append(checksumLines, fmt.Sprintf("%s  %s", sha256Hex([]byte(artifactBody)), tt.artifact))
	}
	writeFile(t, filepath.Join(versionDir, "checksums.txt"), strings.Join(checksumLines, "\n")+"\n", 0o644)

	server := httptest.NewServer(http.FileServer(http.Dir(releaseRoot)))
	t.Cleanup(server.Close)

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			installDir := filepath.Join(t.TempDir(), "bin")
			cmd := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"))
			cmd.Dir = root
			cmd.Env = append(os.Environ(),
				"HOME="+t.TempDir(),
				"DOTS_VERSION="+version,
				"DOTS_RELEASE_BASE_URL="+server.URL,
				"DOTS_INSTALL_DIR="+installDir,
				"DOTS_SOURCE_ROOT="+filepath.Join(t.TempDir(), "checkout"),
				"DOTS_OS="+tt.os,
				"DOTS_ARCH="+tt.arch,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("bootstrap install failed: %v\n%s", err, output)
			}
			if !strings.Contains(string(output), "Downloading "+tt.artifact) {
				t.Fatalf("bootstrap should download %s, got:\n%s", tt.artifact, output)
			}
		})
	}
}

func newBootstrapSourceRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for bootstrap source clone test")
	}
	repo := t.TempDir()
	runBootstrapGit(t, repo, "init", "-b", "main")
	runBootstrapGit(t, repo, "config", "user.email", "dots@test.local")
	runBootstrapGit(t, repo, "config", "user.name", "dots test")
	writeFile(t, filepath.Join(repo, "dots.yaml"), "version: 1\nprofiles: {default: {tags: [core]}}\nentries: []\n", 0o600)
	runBootstrapGit(t, repo, "add", "dots.yaml")
	runBootstrapGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runBootstrapGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
