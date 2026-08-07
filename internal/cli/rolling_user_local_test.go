package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
)

func TestRollingCodexCLIPlanAndInstallUseResolvedFixtureArtifact(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	localBin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatalf("mkdir local bin: %v", err)
	}
	t.Setenv("PATH", localBin+string(os.PathListSeparator)+"/usr/bin:/bin")

	assetName, binaryName := codexFixtureNames(t, runtime.GOOS, runtime.GOARCH)
	archive := codexFixtureArchive(t, binaryName)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	var metadataRequests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			metadataRequests.Add(1)
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"tag_name": "rust-v9.0.0-alpha.1", "prerelease": true},
				{"tag_name": "rust-v8.7.6", "assets": []map[string]string{{"name": assetName, "browser_download_url": server.URL + "/" + assetName, "digest": "sha256:" + digest}}},
			})
		case "/" + assetName:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	previousClient, previousURL := depsHTTPClient, depsRollingReleaseURL
	depsHTTPClient, depsRollingReleaseURL = server.Client(), server.URL+"/releases"
	t.Cleanup(func() { depsHTTPClient, depsRollingReleaseURL = previousClient, previousURL })

	if err := os.MkdirAll(filepath.Join(sourceRoot, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "configs", "agent.txt"), []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	manifestPath := filepath.Join(t.TempDir(), "dots.yaml")
	manifest := `version: 1
profiles:
  default:
    tags: [agents]
entries:
  - source: configs/agent.txt
    target: ~/.config/dots-agent.txt
    strategy: copy
    tags: [agents]
    dependencies:
      - name: codex
        command: codex
        rolling_user_local:
          recipe: codex
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	plan := NewRootCommand()
	var planOut bytes.Buffer
	plan.SetOut(&planOut)
	plan.SetErr(&planOut)
	plan.SetArgs([]string{"--output", "json", "deps", "install", "--dry-run", "--profile", "default", "--file", manifestPath, "--home", home, "--state-root", stateRoot})
	if err := plan.Execute(); err != nil {
		t.Fatalf("deps install --dry-run error = %v\n%s", err, planOut.String())
	}
	for _, want := range []string{"rust-v8.7.6", assetName, "sha256:" + digest, runtime.GOOS + "_" + runtime.GOARCH, filepath.Join(home, ".local", "opt", "codex", "rust-v8.7.6")} {
		if !strings.Contains(planOut.String(), want) {
			t.Fatalf("dry-run JSON missing %q\n%s", want, planOut.String())
		}
	}

	install := NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := install.Execute(); err != nil {
		t.Fatalf("install error = %v\n%s", err, installOut.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "dots-agent.txt")); err != nil {
		t.Fatalf("managed configuration missing after dependency success: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(localBin, "codex")); err != nil || !strings.Contains(target, filepath.Join("codex", "rust-v8.7.6", "bin", "codex")) {
		t.Fatalf("codex link = (%q, %v)", target, err)
	}
	metadata, err := deps.LoadDependencyMetadata(deps.DependencyMetadataPath(stateRoot))
	if err != nil || len(metadata.Dependencies) != 1 {
		t.Fatalf("dependency metadata = (%#v, %v)", metadata, err)
	}
	receipt := metadata.Dependencies[0]
	if receipt.Provider != string(deps.TierUserLocal) || receipt.Version != "rust-v8.7.6" || receipt.URL != server.URL+"/"+assetName || receipt.Artifact != assetName || receipt.Digest != "sha256:"+digest || receipt.Checksum != digest || receipt.Platform != runtime.GOOS+"_"+runtime.GOARCH || receipt.Path != filepath.Join(localBin, "codex") || receipt.InstalledAt == "" {
		t.Fatalf("receipt = %#v", receipt)
	}
	if metadataRequests.Load() != 2 {
		t.Fatalf("metadata requests = %d, want one dry-run resolution and one pinned install resolution", metadataRequests.Load())
	}
}

func TestRollingClaudeCLIPlanAndInstallUseResolvedFixtureArtifact(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	localBin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatalf("mkdir local bin: %v", err)
	}
	t.Setenv("PATH", localBin+string(os.PathListSeparator)+"/usr/bin:/bin")

	platform := claudeFixturePlatform(t, runtime.GOOS, runtime.GOARCH)
	binary := []byte("#!/bin/sh\nprintf 'claude fixture\\n'\n")
	sum := sha256.Sum256(binary)
	digest := hex.EncodeToString(sum[:])
	var metadataRequests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stable":
			metadataRequests.Add(1)
			_, _ = w.Write([]byte("2.1.220"))
		case "/2.1.220/manifest.json":
			metadataRequests.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version":   "2.1.220",
				"platforms": map[string]any{platform: map[string]any{"binary": "claude", "checksum": digest}},
			})
		case "/2.1.220/" + platform + "/claude":
			_, _ = w.Write(binary)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	previousClient, previousURL := depsHTTPClient, depsRollingReleaseURL
	depsHTTPClient, depsRollingReleaseURL = server.Client(), server.URL
	t.Cleanup(func() { depsHTTPClient, depsRollingReleaseURL = previousClient, previousURL })

	if err := os.WriteFile(filepath.Join(sourceRoot, "source"), []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	manifestPath := filepath.Join(t.TempDir(), "dots.yaml")
	manifest := "version: 1\nprofiles:\n  default:\n    tags: [agents]\nentries:\n  - source: source\n    target: ~/.managed\n    strategy: copy\n    tags: [agents]\n    dependencies:\n      - name: claude\n        command: claude\n        rolling_user_local:\n          recipe: claude\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	dryRun := NewRootCommand()
	var dryRunOut bytes.Buffer
	dryRun.SetOut(&dryRunOut)
	dryRun.SetErr(&dryRunOut)
	dryRun.SetArgs([]string{"--output", "json", "deps", "install", "--dry-run", "--profile", "default", "--file", manifestPath, "--home", home, "--state-root", stateRoot})
	if err := dryRun.Execute(); err != nil {
		t.Fatalf("deps install --dry-run error = %v\n%s", err, dryRunOut.String())
	}
	for _, want := range []string{"2.1.220", platform, "sha256:" + digest, server.URL + "/2.1.220/" + platform + "/claude", filepath.Join(home, ".local", "opt", "claude", "2.1.220")} {
		if !strings.Contains(dryRunOut.String(), want) {
			t.Fatalf("dry-run JSON missing %q\n%s", want, dryRunOut.String())
		}
	}

	install := NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := install.Execute(); err != nil {
		t.Fatalf("install error = %v\n%s", err, installOut.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".managed")); err != nil {
		t.Fatalf("managed configuration missing after dependency success: %v", err)
	}
	target, err := os.Readlink(filepath.Join(localBin, "claude"))
	if err != nil || target != filepath.Join(home, ".local", "opt", "claude", "2.1.220", "claude") {
		t.Fatalf("claude link = (%q, %v)", target, err)
	}
	metadata, err := deps.LoadDependencyMetadata(deps.DependencyMetadataPath(stateRoot))
	if err != nil || len(metadata.Dependencies) != 1 {
		t.Fatalf("dependency metadata = (%#v, %v)", metadata, err)
	}
	receipt := metadata.Dependencies[0]
	if receipt.Provider != string(deps.TierUserLocal) || receipt.Version != "2.1.220" || receipt.Artifact != "claude" || receipt.Digest != "sha256:"+digest || receipt.Checksum != digest || receipt.Platform != platform || receipt.Path != filepath.Join(localBin, "claude") || receipt.InstalledAt == "" {
		t.Fatalf("receipt = %#v", receipt)
	}
	if metadataRequests.Load() != 4 {
		t.Fatalf("metadata requests = %d, want stable and manifest for dry-run and pinned install", metadataRequests.Load())
	}
}

func TestRollingCodexCLIExistingCommandSkipsReleaseLookup(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	command := filepath.Join(bin, "codex")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write codex stub: %v", err)
	}
	t.Setenv("PATH", bin)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()
	previousClient, previousURL := depsHTTPClient, depsRollingReleaseURL
	depsHTTPClient, depsRollingReleaseURL = server.Client(), server.URL
	t.Cleanup(func() { depsHTTPClient, depsRollingReleaseURL = previousClient, previousURL })

	manifestPath := filepath.Join(t.TempDir(), "dots.yaml")
	manifest := "version: 1\nprofiles:\n  default:\n    tags: [agents]\nentries:\n  - source: source\n    target: ~/.source\n    strategy: copy\n    tags: [agents]\n    dependencies:\n      - name: codex\n        command: codex\n        rolling_user_local:\n          recipe: codex\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "plan", "--profile", "default", "--file", manifestPath, "--home", home})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("deps plan error = %v\n%s", err, out.String())
	}
	if requests.Load() != 0 || !strings.Contains(out.String(), "already installed") {
		t.Fatalf("requests = %d, output = %s", requests.Load(), out.String())
	}
}

func TestRollingCodexCLIResolutionFailurePrecedesManagedConfiguration(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "metadata unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	previousClient, previousURL := depsHTTPClient, depsRollingReleaseURL
	depsHTTPClient, depsRollingReleaseURL = server.Client(), server.URL
	t.Cleanup(func() { depsHTTPClient, depsRollingReleaseURL = previousClient, previousURL })

	if err := os.WriteFile(filepath.Join(sourceRoot, "source"), []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	manifestPath := filepath.Join(t.TempDir(), "dots.yaml")
	manifest := "version: 1\nprofiles:\n  default:\n    tags: [agents]\nentries:\n  - source: source\n    target: ~/.managed\n    strategy: copy\n    tags: [agents]\n    dependencies:\n      - name: codex\n        command: codex\n        rolling_user_local:\n          recipe: codex\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", t.TempDir()})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "resolve rolling user-local recipe") {
		t.Fatalf("install error = %v\n%s", err, out.String())
	}
	if _, statErr := os.Stat(filepath.Join(home, ".managed")); !os.IsNotExist(statErr) {
		t.Fatalf("Managed Configuration changed after resolution failure: %v", statErr)
	}
}

func codexFixtureNames(t *testing.T, goos, goarch string) (asset, binary string) {
	t.Helper()
	targets := map[string]string{
		"darwin_amd64": "x86_64-apple-darwin",
		"darwin_arm64": "aarch64-apple-darwin",
		"linux_amd64":  "x86_64-unknown-linux-musl",
		"linux_arm64":  "aarch64-unknown-linux-musl",
	}
	target, ok := targets[goos+"_"+goarch]
	if !ok {
		t.Skipf("unsupported test platform %s/%s", goos, goarch)
	}
	return "codex-package-" + target + ".tar.gz", "bin/codex"
}

func claudeFixturePlatform(t *testing.T, goos, goarch string) string {
	t.Helper()
	platforms := map[string]string{
		"darwin_amd64": "darwin-x64",
		"darwin_arm64": "darwin-arm64",
		"linux_amd64":  "linux-x64",
		"linux_arm64":  "linux-arm64",
	}
	platform, ok := platforms[goos+"_"+goarch]
	if !ok {
		t.Skipf("unsupported test platform %s/%s", goos, goarch)
	}
	return platform
}

func codexFixtureArchive(t *testing.T, binary string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\nprintf 'codex fixture\\n'\n")
	if err := tw.WriteHeader(&tar.Header{Name: binary, Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}
