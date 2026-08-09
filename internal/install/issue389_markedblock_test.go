package install_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/repositoryrefresh"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
	"github.com/yersonargotev/dots/internal/uninstall"
)

func TestZshMarkedBlockLifecycle(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	legacySource := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	loaderSource := filepath.Join(sourceRoot, "configs/zsh/loader.zsh")
	legacy := []byte("# legacy portable shell\nexport OLD=1\n")
	loader := []byte("# >>> dots managed block >>>\nsource \"${HOME}/.config/dots/zsh/zshrc\"\n# <<< dots managed block <<<\n")
	writeMigrationFile(t, legacySource, legacy)
	runMigrationGit(t, sourceRoot, "init")
	runMigrationGit(t, sourceRoot, "config", "user.email", "dots@example.test")
	runMigrationGit(t, sourceRoot, "config", "user.name", "dots test")
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "legacy zsh")
	revision := runMigrationGit(t, sourceRoot, "rev-parse", "HEAD")

	target := filepath.Join(home, ".zshrc")
	if err := os.Symlink(legacySource, target); err != nil {
		t.Fatalf("symlink legacy zsh: %v", err)
	}
	external := []byte("\n# third-party installer\nexport TOOL_HOME=/tmp/tool\n")
	if err := os.WriteFile(legacySource, append(append([]byte(nil), legacy...), external...), 0o600); err != nil {
		t.Fatalf("append through legacy target: %v", err)
	}
	meta := state.Metadata{Version: state.CurrentVersion, Provenance: state.Provenance{SourceRoot: sourceRoot, SourceRevision: revision}, Entries: []state.Record{{
		Target: target, Source: "configs/zsh/zshrc", Strategy: "symlink", Ownership: "whole",
	}}}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatalf("save legacy metadata: %v", err)
	}
	oldManifest := zshManifest("configs/zsh/zshrc", "symlink", "")
	newManifest := zshManifest("configs/zsh/loader.zsh", "copy", "marked-block")
	captures, err := repositoryrefresh.CaptureLegacyTargets(oldManifest, newManifest, meta, sourceRoot, home, "", revision)
	if err != nil {
		t.Fatalf("CaptureLegacyTargets() error = %v", err)
	}
	writeMigrationFile(t, legacySource, legacy)
	writeMigrationFile(t, loaderSource, loader)
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "marked zsh loader")

	opts := plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: meta, LegacyMigrations: captures}
	p, err := plan.Build(newManifest, opts)
	if err != nil {
		t.Fatalf("Build(migrate) error = %v", err)
	}
	if len(p.Actions) != 1 || p.Actions[0].Status != plan.StatusMigrate {
		t.Fatalf("migration plan = %#v", p.Actions)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(migrate) error = %v", err)
	}
	want := append(append([]byte(nil), loader...), external...)
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("migrated target = (%q, %v), want %q", got, err, want)
	}
	if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("native target = (%v, %v), want regular file", info, err)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil || len(backupMeta.Sets) != 1 {
		t.Fatalf("migration backups = (%#v, %v), want one", backupMeta, err)
	}
	if out := runMigrationGit(t, sourceRoot, "status", "--porcelain"); out != "" {
		t.Fatalf("repository status after target append = %q, want clean", out)
	}

	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load marked-block metadata: %v", err)
	}
	report, err := status.Build(newManifest, meta, status.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home})
	if err != nil || len(report.Entries) != 1 || report.Entries[0].State != status.StateOK {
		t.Fatalf("status after append = (%#v, %v), want ok", report, err)
	}

	for name, content := range map[string][]byte{
		"duplicate":     append(append([]byte(nil), loader...), loader...),
		"missing-close": []byte("# >>> dots managed block >>>\nsource bad\n"),
		"moved":         append([]byte("export BEFORE=1\n"), loader...),
		"modified":      []byte("# >>> dots managed block >>>\nsource changed\n# <<< dots managed block <<<\n"),
	} {
		t.Run(name+" conflict", func(t *testing.T) {
			if err := os.WriteFile(target, content, 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			conflictPlan, err := plan.Build(newManifest, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: meta})
			if err != nil || len(conflictPlan.Actions) != 1 || conflictPlan.Actions[0].Status != plan.StatusConflict {
				t.Fatalf("plan = (%#v, %v), want conflict", conflictPlan, err)
			}
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, content) {
				t.Fatalf("conflict fixture changed = (%q, %v)", got, err)
			}
		})
	}
	if err := os.WriteFile(target, want, 0o600); err != nil {
		t.Fatalf("restore compatible target: %v", err)
	}
	updatedLoader := bytes.Replace(loader, []byte("source \""), []byte("# updated baseline\nsource \""), 1)
	writeMigrationFile(t, loaderSource, updatedLoader)
	updatePlan, err := plan.Build(newManifest, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: meta})
	if err != nil || updatePlan.Actions[0].Status != plan.StatusUpdate {
		t.Fatalf("update plan = (%#v, %v), want update", updatePlan, err)
	}
	if err := install.Apply(updatePlan, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(update) error = %v", err)
	}
	want = append(append([]byte(nil), updatedLoader...), external...)
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("updated target = (%q, %v), want %q", got, err, want)
	}
	backupMeta, err = backups.Load(backups.Path(stateRoot))
	if err != nil || len(backupMeta.Sets) != 2 {
		t.Fatalf("update backups = (%#v, %v), want two total", backupMeta, err)
	}

	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("reload metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatal("marked-block record missing")
	}
	uninstallPlan, err := plan.BuildUninstall(state.Metadata{Entries: []state.Record{rec}}, plan.UninstallOptions{SourceRoot: sourceRoot, Home: home})
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
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, external) {
		t.Fatalf("uninstalled target = (%q, %v), want external bytes", got, err)
	}
}

func zshManifest(source, strategy, ownership string) manifest.Manifest {
	return manifest.Manifest{Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}}, Entries: []manifest.Entry{{
		Source: source, Target: "~/.zshrc", Strategy: strategy, Ownership: ownership, Tags: []string{"core"}, OS: []string{"linux"},
	}}}
}
