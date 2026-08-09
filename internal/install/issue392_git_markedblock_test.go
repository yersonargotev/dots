package install_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/repositoryrefresh"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
	"github.com/yersonargotev/dots/internal/uninstall"
)

func TestGitMarkedBlockLifecycle(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	portableSource := filepath.Join(sourceRoot, "configs/git/gitconfig")
	loaderSource := filepath.Join(sourceRoot, "configs/git/loader.gitconfig")
	legacy := []byte("[dots]\n\tlayer = portable\n[include]\n\tpath = ~/.gitconfig.local\n")
	portable := []byte("[dots]\n\tlayer = portable\n")
	loader := []byte("# >>> dots managed block >>>\n[include]\n\tpath = ~/.config/dots/git/gitconfig\n\tpath = ~/.gitconfig.local\n# <<< dots managed block <<<\n")
	writeMigrationFile(t, portableSource, legacy)
	writeMigrationFile(t, filepath.Join(home, ".gitconfig.local"), []byte("[dots]\n\tlayer = local\n[user]\n\tname = Local User\n"))
	runMigrationGit(t, sourceRoot, "init")
	runMigrationGit(t, sourceRoot, "config", "user.email", "dots@example.test")
	runMigrationGit(t, sourceRoot, "config", "user.name", "dots test")
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "legacy git config")
	revision := runMigrationGit(t, sourceRoot, "rev-parse", "HEAD")

	target := filepath.Join(home, ".gitconfig")
	if err := os.Symlink(portableSource, target); err != nil {
		t.Fatalf("symlink legacy gitconfig: %v", err)
	}
	runGlobalGitConfig(t, home, "user.email", "native@example.test")
	legacyAfterGlobalWrite := mustReadFile(t, portableSource)
	nativeAdditions := []byte("# native additions\n[dots]\n\tlayer = native\n[includeIf \"gitdir:~/work/\"]\n\tpath = ~/.gitconfig.work\n")
	if err := os.WriteFile(portableSource, append(legacyAfterGlobalWrite, nativeAdditions...), 0o600); err != nil {
		t.Fatalf("append native values: %v", err)
	}
	external := append([]byte(nil), mustTrimPrefix(t, mustReadFile(t, portableSource), legacy)...)
	if len(external) == 0 {
		t.Fatal("git config --global did not append native content to the legacy source")
	}
	meta := state.Metadata{Version: state.CurrentVersion, Provenance: state.Provenance{SourceRoot: sourceRoot, SourceRevision: revision}, Entries: []state.Record{{
		Target: target, Source: "configs/git/gitconfig", Strategy: "symlink", Ownership: "whole",
	}}}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatalf("save legacy metadata: %v", err)
	}
	oldManifest := gitManifest("configs/git/gitconfig", "~/.gitconfig", "symlink", "")
	newManifest := manifest.Manifest{Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}}, Entries: []manifest.Entry{
		{Source: "configs/git/loader.gitconfig", Target: "~/.gitconfig", Strategy: "copy", Ownership: "marked-block", Tags: []string{"core"}, OS: []string{"linux"}},
		{Source: "configs/git/gitconfig", Target: "~/.config/dots/git/gitconfig", Strategy: "symlink", Tags: []string{"core"}, OS: []string{"linux"}},
	}}
	captures, err := repositoryrefresh.CaptureLegacyTargets(oldManifest, newManifest, meta, sourceRoot, home, "", revision)
	if err != nil {
		t.Fatalf("CaptureLegacyTargets() error = %v", err)
	}
	writeMigrationFile(t, portableSource, portable)
	writeMigrationFile(t, loaderSource, loader)
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "co-own native git config")

	p, err := plan.Build(newManifest, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: meta, LegacyMigrations: captures})
	if err != nil {
		t.Fatalf("Build(migrate) error = %v", err)
	}
	if action := actionForTarget(t, p, target); action.Status != plan.StatusMigrate {
		t.Fatalf("native migration action = %#v, want migrate", action)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(migrate) error = %v", err)
	}
	want := append(append([]byte(nil), loader...), external...)
	if got := mustReadFile(t, target); !bytes.Equal(got, want) {
		t.Fatalf("migrated target = %q, want %q", got, want)
	}
	if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("native target = (%v, %v), want regular file", info, err)
	}
	if got := runGitConfigGetAll(t, home, "dots.layer"); got != "portable\nlocal\nnative" {
		t.Fatalf("effective precedence = %q, want portable, local, native", got)
	}
	runGlobalGitConfig(t, home, "user.name", "Native User")
	if out := runMigrationGit(t, sourceRoot, "status", "--porcelain"); out != "" {
		t.Fatalf("repository status after global write = %q, want clean", out)
	}

	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load marked-block metadata: %v", err)
	}
	report, err := status.Build(newManifest, meta, status.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home})
	if err != nil || statusForTarget(t, report, target) != status.StateOK {
		t.Fatalf("status after native write = (%#v, %v), want ok", report, err)
	}
	compatible := mustReadFile(t, target)
	for name, content := range map[string][]byte{
		"duplicate":     append(append([]byte(nil), loader...), loader...),
		"missing-close": []byte("# >>> dots managed block >>>\n[include]\n\tpath = bad\n"),
		"moved":         append([]byte("[user]\n\tname = Before\n"), compatible...),
		"modified":      []byte("# >>> dots managed block >>>\n[include]\n\tpath = changed\n# <<< dots managed block <<<\n"),
	} {
		t.Run(name+" conflict", func(t *testing.T) {
			if err := os.WriteFile(target, content, 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			conflictPlan, err := plan.Build(newManifest, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: meta})
			if err != nil || actionForTarget(t, conflictPlan, target).Status != plan.StatusConflict {
				t.Fatalf("plan = (%#v, %v), want conflict", conflictPlan, err)
			}
			if got := mustReadFile(t, target); !bytes.Equal(got, content) {
				t.Fatalf("conflict fixture changed = %q", got)
			}
		})
	}
	if err := os.WriteFile(target, compatible, 0o600); err != nil {
		t.Fatalf("restore compatible target: %v", err)
	}

	uninstallPlan, err := plan.BuildUninstall(meta, plan.UninstallOptions{SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("BuildUninstall() error = %v", err)
	}
	result, err := uninstall.Apply(uninstallPlan, uninstall.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("uninstall Apply() error = %v", err)
	}
	if len(result.Updated) != 1 || result.Updated[0] != target {
		t.Fatalf("uninstall result = %#v, want updated native target", result)
	}
	remaining := mustReadFile(t, target)
	if bytes.Contains(remaining, []byte("dots managed block")) || !bytes.Contains(remaining, []byte("native@example.test")) || !bytes.Contains(remaining, []byte("Native User")) {
		t.Fatalf("uninstalled native target = %q, want only preserved external content", remaining)
	}
	if got := mustReadFile(t, filepath.Join(home, ".gitconfig.local")); !bytes.Contains(got, []byte("Local User")) {
		t.Fatalf("local extension changed during uninstall: %q", got)
	}
}

func gitManifest(source, target, strategy, ownership string) manifest.Manifest {
	return manifest.Manifest{Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}}, Entries: []manifest.Entry{{
		Source: source, Target: target, Strategy: strategy, Ownership: ownership, Tags: []string{"core"}, OS: []string{"linux"},
	}}}
}

func runGlobalGitConfig(t *testing.T, home, key, value string) {
	t.Helper()
	cmd := exec.Command("git", "config", "--global", key, value)
	cmd.Env = append(os.Environ(), "HOME="+home, "GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig"), "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config --global %s: %v\n%s", key, err, out)
	}
}

func runGitConfigGetAll(t *testing.T, home, key string) string {
	t.Helper()
	cmd := exec.Command("git", "config", "--global", "--includes", "--get-all", key)
	cmd.Env = append(os.Environ(), "HOME="+home, "GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig"), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git config --get-all %s: %v\n%s", key, err, out)
	}
	return strings.TrimSpace(string(out))
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

func mustTrimPrefix(t *testing.T, content, prefix []byte) []byte {
	t.Helper()
	if !bytes.HasPrefix(content, prefix) {
		t.Fatalf("content %q does not preserve prefix %q", content, prefix)
	}
	return content[len(prefix):]
}

func actionForTarget(t *testing.T, p plan.Plan, target string) plan.Action {
	t.Helper()
	for _, action := range p.Actions {
		if action.Target == target {
			return action
		}
	}
	t.Fatalf("plan has no action for %s: %#v", target, p.Actions)
	return plan.Action{}
}

func statusForTarget(t *testing.T, report status.Report, target string) status.State {
	t.Helper()
	for _, entry := range report.Entries {
		if entry.Target == target {
			return entry.State
		}
	}
	t.Fatalf("status has no entry for %s: %#v", target, report.Entries)
	return ""
}
