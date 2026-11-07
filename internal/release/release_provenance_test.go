package release_test

import (
	"debug/buildinfo"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildReleaseArtifactsPreservesTaggedProvenance(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles release artifacts in a disposable Git checkout")
	}

	// Exercise the real builder and ignore rules with a dependency-free module.
	// Keep the home, caches and outputs outside the tracked source unless the
	// scenario deliberately uses the workflow's in-checkout dist directory.
	root := t.TempDir()
	env := append(os.Environ(), "HOME="+t.TempDir(), "GOCACHE="+t.TempDir(),
		"GOMODCACHE="+goEnv(t, "GOMODCACHE"), "GOPROXY=off", "GOSUMDB=off",
		"GOENV=off", "GOWORK=off", "GOFLAGS=", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1", "RELEASE_VERSION=", "RELEASE_OUT_DIR=")
	run := func(t *testing.T, extraEnv []string, name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = root
		cmd.Env = append(append([]string{}, env...), extraEnv...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	write := func(path string, data []byte) {
		t.Helper()
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{".gitignore", "scripts/build-release-artifacts.sh"} {
		data, err := os.ReadFile(filepath.Join(repoRoot(t), path))
		if err != nil {
			t.Fatal(err)
		}
		write(path, data)
	}
	write("go.mod", []byte("module github.com/yersonargotev/dots\n\ngo 1.24.2\n"))
	source := []byte(`package main

import (
	"fmt"
	"github.com/yersonargotev/dots/internal/version"
)

func main() { fmt.Println("dots " + version.Value) }
`)
	write("cmd/dots/main.go", source)
	write("internal/version/version.go", []byte("package version\n\nvar Value = \"dev\"\n"))
	run(t, nil, "git", "init", "--quiet")
	run(t, nil, "git", "config", "user.name", "Release test")
	run(t, nil, "git", "config", "user.email", "release-test@example.invalid")
	run(t, nil, "git", "config", "commit.gpgsign", "false")
	run(t, nil, "git", "config", "core.hooksPath", os.DevNull)
	run(t, nil, "git", "add", ".")
	run(t, nil, "git", "commit", "--quiet", "-m", "release fixture")
	run(t, nil, "git", "tag", "v0.99.0")
	revision := run(t, nil, "git", "rev-parse", "HEAD")

	checkBuild := func(t *testing.T, outDir string, dirty bool, extraEnv []string, args ...string) {
		t.Helper()
		before := run(t, nil, "git", "status", "--porcelain", "--untracked-files=all")
		run(t, extraEnv, "bash", append([]string{"scripts/build-release-artifacts.sh"}, args...)...)
		after := run(t, nil, "git", "status", "--porcelain", "--untracked-files=all")
		t.Logf("Git status before: %q; after: %q", before, after)
		wantStatus := ""
		wantVersion, wantModified := "v0.99.0", "false"
		if dirty {
			wantStatus = "M cmd/dots/main.go"
			wantVersion, wantModified = "v0.99.0+dirty", "true"
		}
		if before != wantStatus || after != wantStatus {
			t.Errorf("Git status should remain %q; before %q, after %q", wantStatus, before, after)
		}
		assertReleaseArtifacts(t, outDir)
		for _, platform := range supportedReleasePlatforms {
			artifact := filepath.Join(outDir, "dots_v0.99.0_"+strings.ReplaceAll(platform, "/", "_"))
			info, err := buildinfo.ReadFile(artifact)
			if err != nil {
				t.Fatal(err)
			}
			settings := map[string]string{}
			for _, setting := range info.Settings {
				settings[setting.Key] = setting.Value
			}
			t.Logf("%s: module=%s revision=%s modified=%s", platform, info.Main.Version, settings["vcs.revision"], settings["vcs.modified"])
			if info.Main.Version != wantVersion || settings["vcs.revision"] != revision || settings["vcs.modified"] != wantModified {
				t.Errorf("%s: want module=%s revision=%s modified=%s", platform, wantVersion, revision, wantModified)
			}
		}
	}

	external := t.TempDir()
	t.Run("explicit external output", func(t *testing.T) {
		checkBuild(t, external, false, []string{"RELEASE_VERSION=v0.1.0", "RELEASE_OUT_DIR=" + t.TempDir()}, "--version", "v0.99.0", "--out-dir", external)
	})
	dist := filepath.Join(root, "dist")
	t.Run("workflow output", func(t *testing.T) {
		checkBuild(t, dist, false, nil, "--version", "v0.99.0", "--out-dir", "dist")
	})
	t.Run("repeat with default output", func(t *testing.T) {
		checkBuild(t, dist, false, []string{"RELEASE_VERSION=v0.99.0", "RELEASE_OUT_DIR="})
	})
	t.Run("repeat with environment output", func(t *testing.T) {
		checkBuild(t, external, false, []string{"RELEASE_VERSION=v0.99.0", "RELEASE_OUT_DIR=" + external})
	})
	t.Run("tracked source modification", func(t *testing.T) {
		write("cmd/dots/main.go", append(append([]byte{}, source...), []byte("\n// Deliberate source modification.\n")...))
		checkBuild(t, dist, true, nil, "--version", "v0.99.0", "--out-dir", "dist")
	})
	t.Run("restored source", func(t *testing.T) {
		write("cmd/dots/main.go", source)
		checkBuild(t, dist, false, nil, "--version", "v0.99.0", "--out-dir", "dist")
	})
}
