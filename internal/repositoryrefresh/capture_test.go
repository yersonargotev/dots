package repositoryrefresh_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/repositoryrefresh"
	"github.com/yersonargotev/dots/internal/state"
)

func TestCaptureLegacyTargetsRequiresMatchingProvenanceAndDeclaredSymlink(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	runGit(t, sourceRoot, "init", "-b", "main")
	runGit(t, sourceRoot, "config", "user.email", "dots@test.local")
	runGit(t, sourceRoot, "config", "user.name", "dots test")
	source := filepath.Join(sourceRoot, "configs", "app.json")
	writeFile(t, source, "{\"owned\":1}\n")
	runGit(t, sourceRoot, "add", ".")
	runGit(t, sourceRoot, "commit", "-m", "legacy")
	revision := gitOutput(t, sourceRoot, "rev-parse", "HEAD")
	target := filepath.Join(home, ".config", "app.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}
	writeFile(t, source, "{\"owned\":1,\"runtime\":true}\n")

	m := manifest.Manifest{Entries: []manifest.Entry{{Source: "configs/app.json", Target: "~/.config/app.json", Strategy: "symlink", Tags: []string{"core"}}}}
	meta := state.Metadata{
		Provenance: state.Provenance{SourceRoot: sourceRoot, SourceRevision: revision[:12]},
		Entries:    []state.Record{{Target: target, Source: "configs/app.json", Strategy: "symlink"}},
	}
	captures, err := repositoryrefresh.CaptureLegacyTargets(m, m, meta, sourceRoot, home, filepath.Join(home, ".local", "state"), revision)
	if err != nil {
		t.Fatal(err)
	}
	capture, ok := captures[target]
	if !ok {
		t.Fatal("expected provenance-backed legacy target capture")
	}
	if string(capture.CapturedContent) != "{\"owned\":1,\"runtime\":true}\n" || string(capture.PreviousSourceContent) != "{\"owned\":1}\n" {
		t.Fatalf("capture = %#v", capture)
	}

	meta.Provenance.SourceRevision = "deadbeef"
	captures, err = repositoryrefresh.CaptureLegacyTargets(m, m, meta, sourceRoot, home, filepath.Join(home, ".local", "state"), revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(captures) != 0 {
		t.Fatalf("stale provenance captured targets: %#v", captures)
	}

	meta.Provenance.SourceRevision = revision[:1]
	captures, err = repositoryrefresh.CaptureLegacyTargets(m, m, meta, sourceRoot, home, filepath.Join(home, ".local", "state"), revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(captures) != 0 {
		t.Fatalf("ambiguous short provenance captured targets: %#v", captures)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out[:len(out)-1])
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
