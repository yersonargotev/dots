package release_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var supportedReleasePlatforms = []string{
	"darwin/amd64",
	"darwin/arm64",
	"linux/amd64",
	"linux/arm64",
}

func TestReleaseWorkflowPublishesBootstrapperConsumableArtifacts(t *testing.T) {
	root := repoRoot(t)
	workflow := readWorkflow(t, filepath.Join(root, ".github", "workflows", "release.yml"))

	triggers := mapAt(t, workflow, "on")
	push := mapAt(t, triggers, "push")
	tags := stringSliceAt(t, push, "tags")
	if !contains(tags, "v0.*") {
		t.Fatalf("release workflow should run for v0.x tags; got tags %v", tags)
	}
	if _, ok := triggers["workflow_dispatch"]; !ok {
		t.Fatalf("release workflow should support manual dispatch for the first v0.x release")
	}

	workflowText, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflowText)
	for _, want := range []string{
		"actions/checkout@v5",
		"actions/setup-go@v6",
		"scripts/build-release-artifacts.sh",
		"gh release upload",
		"checksums.txt",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow should contain %q so GitHub Releases get versioned assets plus checksums", want)
		}
	}
	if strings.Contains(strings.ToLower(text), "homebrew") {
		t.Fatalf("release workflow should not configure Homebrew distribution in this slice")
	}
}

func TestBuildReleaseArtifactsCreatesChecksummedSupportedPlatforms(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles release artifacts")
	}

	root := repoRoot(t)
	outDir := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "build-release-artifacts.sh"), "--version", "v0.99.0", "--out-dir", outDir)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "GOCACHE="+t.TempDir(), "GOMODCACHE="+goEnv(t, "GOMODCACHE"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build release artifacts: %v\n%s", err, output)
	}

	wantAssets := make([]string, 0, len(supportedReleasePlatforms))
	for _, platform := range supportedReleasePlatforms {
		parts := strings.Split(platform, "/")
		wantAssets = append(wantAssets, fmt.Sprintf("dots_v0.99.0_%s_%s", parts[0], parts[1]))
	}
	sort.Strings(wantAssets)

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	var gotAssets []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "checksums.txt" {
			continue
		}
		gotAssets = append(gotAssets, entry.Name())
	}
	sort.Strings(gotAssets)
	if strings.Join(gotAssets, "\n") != strings.Join(wantAssets, "\n") {
		t.Fatalf("release assets mismatch\nwant:\n%s\ngot:\n%s", strings.Join(wantAssets, "\n"), strings.Join(gotAssets, "\n"))
	}

	checksums := readChecksums(t, filepath.Join(outDir, "checksums.txt"))
	for _, asset := range wantAssets {
		gotChecksum, ok := checksums[asset]
		if !ok {
			t.Fatalf("checksums.txt missing checksum for %s", asset)
		}
		if gotChecksum != sha256File(t, filepath.Join(outDir, asset)) {
			t.Fatalf("checksum for %s does not match artifact bytes", asset)
		}
	}
	if len(checksums) != len(wantAssets) {
		t.Fatalf("checksums.txt should contain exactly release artifacts; got %d entries", len(checksums))
	}
}

func TestBuildReleaseArtifactsValidatesReleaseVersionLikeWorkflow(t *testing.T) {
	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "build-release-artifacts.sh")

	t.Run("accepts v0 release tags", func(t *testing.T) {
		for _, version := range []string{"v0.1.0", "v0.2", "v0.1.0-rc.1"} {
			t.Run(version, func(t *testing.T) {
				fakeBin, logPath := fakeGoBuild(t)
				outDir := t.TempDir()
				cmd := exec.Command("bash", script, "--version", version, "--out-dir", outDir)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("build release artifacts should accept %s: %v\n%s", version, err, output)
				}
				if _, err := os.Stat(logPath); err != nil {
					t.Fatalf("expected accepted version %s to reach go build: %v", version, err)
				}
			})
		}
	})

	t.Run("rejects non-v0 and malformed versions before building", func(t *testing.T) {
		for _, version := range []string{"v1.0.0", "0.1.0", "main", ""} {
			t.Run(fmt.Sprintf("%q", version), func(t *testing.T) {
				fakeBin, logPath := fakeGoBuild(t)
				outDir := t.TempDir()
				cmd := exec.Command("bash", script, "--version", version, "--out-dir", outDir)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

				output, err := cmd.CombinedOutput()
				if err == nil {
					t.Fatalf("build release artifacts should reject version %q\n%s", version, output)
				}
				if !strings.Contains(string(output), "Release version must be a v0.x tag") {
					t.Fatalf("rejection should explain v0.x requirement, got:\n%s", output)
				}
				if _, err := os.Stat(logPath); !os.IsNotExist(err) {
					t.Fatalf("invalid version %q should fail before go build; stat error: %v", version, err)
				}
			})
		}
	})
}

func fakeGoBuild(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "go-build.log")
	goPath := filepath.Join(dir, "go")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> %q
out=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-o" ]]; then
    out="${2:-}"
    break
  fi
  shift
done
if [[ -n "$out" ]]; then
  mkdir -p "$(dirname "$out")"
  printf 'fake binary\n' > "$out"
fi
`, logPath)
	if err := os.WriteFile(goPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, logPath
}

func goEnv(t *testing.T, key string) string {
	t.Helper()
	cmd := exec.Command("go", "env", key)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go env %s: %v", key, err)
	}
	return strings.TrimSpace(string(output))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readWorkflow(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var workflow map[string]any
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse workflow YAML: %v", err)
	}
	return workflow
}

func mapAt(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := m[key]
	if !ok {
		t.Fatalf("missing %q", key)
	}
	nested, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%q should be a map, got %T", key, value)
	}
	return nested
}

func stringSliceAt(t *testing.T, m map[string]any, key string) []string {
	t.Helper()
	value, ok := m[key]
	if !ok {
		t.Fatalf("missing %q", key)
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("%q should be a list, got %T", key, value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("%q item should be a string, got %T", key, item)
		}
		out = append(out, text)
	}
	return out
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func readChecksums(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	checksums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			t.Fatalf("checksum line should be '<sha256>  <asset>', got %q", scanner.Text())
		}
		if len(fields[0]) != sha256.Size*2 {
			t.Fatalf("checksum for %s should be SHA-256 hex, got %q", fields[1], fields[0])
		}
		if strings.Contains(fields[1], string(os.PathSeparator)) {
			t.Fatalf("checksum entry should use asset filename only, got %q", fields[1])
		}
		checksums[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return checksums
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
