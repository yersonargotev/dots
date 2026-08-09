package install_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestAtuinTOMLSubsetLifecycleMigratesReconcilesAndUninstallsSafely(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "atuin", "config.toml")
	target := filepath.Join(home, ".config", "atuin", "config.toml")
	dataDir := filepath.Join(home, ".local", "share", "atuin")
	oldBaseline := []byte(`# portable baseline
enter_accept = true
retired = "old"

[sync]
records = true
`)
	incomingBaseline := []byte(`# portable baseline
enter_accept = true

[sync]
records = true

[theme]
name = "catppuccin-mocha"
`)
	external := []byte(`
# written by atuin config set
search_mode = "fuzzy"

[daemon]
enabled = true
`)

	writeMigrationFile(t, source, oldBaseline)
	runMigrationGit(t, sourceRoot, "init")
	runMigrationGit(t, sourceRoot, "config", "user.email", "dots@example.test")
	runMigrationGit(t, sourceRoot, "config", "user.name", "dots test")
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "legacy Atuin baseline")
	oldRevision := strings.TrimSpace(runMigrationGit(t, sourceRoot, "rev-parse", "HEAD"))

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("symlink legacy Atuin config: %v", err)
	}
	if err := os.WriteFile(source, append(append([]byte(nil), oldBaseline...), external...), 0o600); err != nil {
		t.Fatalf("simulate Atuin write through symlink: %v", err)
	}
	for name, content := range map[string]string{
		"history.db": "history",
		"key":        "private-key",
		"session":    "auth-token",
		"host_id":    "host",
		"records.db": "records",
	} {
		writeMigrationFile(t, filepath.Join(dataDir, name), []byte(content))
	}

	oldManifest := atuinManifest("symlink", "")
	newManifest := atuinManifest("copy", "toml-subset")
	meta := state.Metadata{
		Version:    state.CurrentVersion,
		Provenance: state.Provenance{SourceRoot: sourceRoot, SourceRevision: oldRevision},
		Entries: []state.Record{{
			Target: target, Source: "configs/atuin/config.toml", Strategy: "symlink", Ownership: "whole",
		}},
	}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatalf("save legacy metadata: %v", err)
	}
	captures, err := repositoryrefresh.CaptureLegacyTargets(oldManifest, newManifest, meta, sourceRoot, home, "", oldRevision)
	if err != nil {
		t.Fatalf("CaptureLegacyTargets() error = %v", err)
	}
	if got := captures[target].CapturedContent; !bytes.Equal(got, append(append([]byte(nil), oldBaseline...), external...)) {
		t.Fatalf("captured Atuin config = %q", got)
	}

	writeMigrationFile(t, source, incomingBaseline)
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "materialize Atuin config")
	p, err := plan.Build(newManifest, plan.Options{
		Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home,
		Metadata: meta, LegacyMigrations: captures,
	})
	if err != nil {
		t.Fatalf("Build(migration) error = %v", err)
	}
	if len(p.Actions) != 1 || p.Actions[0].Status != plan.StatusMigrate {
		t.Fatalf("migration plan = %#v, want one migrate action", p.Actions)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(migration) error = %v", err)
	}
	if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("materialized Atuin target = (%v, %v), want regular file", info, err)
	}
	assertContainsAll(t, target, []string{
		`enter_accept = true`, `search_mode = "fuzzy"`, `[daemon]`, `enabled = true`, `[theme]`, `name = "catppuccin-mocha"`,
	})
	if got := runMigrationGit(t, sourceRoot, "status", "--porcelain"); got != "" {
		t.Fatalf("repository status after Atuin-like addition = %q, want clean", got)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil || len(backupMeta.Sets) != 1 || len(backupMeta.Sets[0].Targets) != 1 || backupMeta.Sets[0].Targets[0] != target {
		t.Fatalf("migration Backup Sets = (%#v, %v), want target backup", backupMeta, err)
	}

	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load migrated metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Ownership != "toml-subset" || !bytes.Equal(rec.OwnedBytes, incomingBaseline) {
		t.Fatalf("Atuin ownership record = %#v, want current TOML contribution", rec)
	}
	report, err := status.Build(newManifest, meta, status.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home})
	if err != nil || len(report.Entries) != 1 || report.Entries[0].State != status.StateOK {
		t.Fatalf("status after migration = (%#v, %v), want ok", report, err)
	}

	live, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read materialized target: %v", err)
	}
	changedOwned := bytes.Replace(live, []byte("enter_accept = true"), []byte("enter_accept = false"), 1)
	if err := os.WriteFile(target, changedOwned, 0o600); err != nil {
		t.Fatalf("change owned Atuin value: %v", err)
	}
	report, err = status.Build(newManifest, meta, status.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home})
	if err != nil || report.Entries[0].State != status.StateDrifted {
		t.Fatalf("status after owned change = (%#v, %v), want drifted", report, err)
	}
	conflictPlan, err := plan.Build(newManifest, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: meta})
	if err != nil || conflictPlan.Actions[0].Status != plan.StatusConflict {
		t.Fatalf("plan after owned change = (%#v, %v), want conflict", conflictPlan, err)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, changedOwned) {
		t.Fatalf("owned Conflict was overwritten = (%q, %v)", got, err)
	}
	if err := os.WriteFile(target, live, 0o600); err != nil {
		t.Fatalf("restore compatible Atuin target: %v", err)
	}

	updatedBaseline := []byte(`# portable baseline
enter_accept = false
filter_mode_shell_up_key_binding = "session"

[theme]
name = "catppuccin-mocha"
`)
	writeMigrationFile(t, source, updatedBaseline)
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "update Atuin baseline")
	updatePlan, err := plan.Build(newManifest, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: meta})
	if err != nil || updatePlan.Actions[0].Status != plan.StatusUpdate {
		t.Fatalf("baseline update plan = (%#v, %v), want update", updatePlan, err)
	}
	if err := install.Apply(updatePlan, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(update) error = %v", err)
	}
	assertContainsAll(t, target, []string{
		`enter_accept = false`, `filter_mode_shell_up_key_binding = "session"`, `search_mode = "fuzzy"`, `[daemon]`, `enabled = true`,
	})
	if got, _ := os.ReadFile(target); bytes.Contains(got, []byte("records = true")) {
		t.Fatalf("updated Atuin target retained retired owned value:\n%s", got)
	}

	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("reload updated metadata: %v", err)
	}
	rec, ok = meta.FindByTarget(target)
	if !ok || !bytes.Equal(rec.OwnedBytes, updatedBaseline) {
		t.Fatalf("updated Atuin ownership record = %#v", rec)
	}
	uninstallPlan, err := plan.BuildUninstall(state.Metadata{Entries: []state.Record{rec}}, plan.UninstallOptions{SourceRoot: sourceRoot, Home: home})
	if err != nil || len(uninstallPlan.Actions) != 1 || uninstallPlan.Actions[0].Status != plan.UninstallRemove {
		t.Fatalf("BuildUninstall() = (%#v, %v), want partial removal", uninstallPlan, err)
	}
	result, err := uninstall.Apply(uninstallPlan, uninstall.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot, Force: true})
	if err != nil {
		t.Fatalf("uninstall Apply() error = %v", err)
	}
	if len(result.Updated) != 1 || result.Updated[0] != target || len(result.Removed) != 0 {
		t.Fatalf("uninstall result = %#v, want preserved external Atuin config", result)
	}
	assertContainsAll(t, target, []string{`search_mode = "fuzzy"`, `[daemon]`, `enabled = true`})
	if got, _ := os.ReadFile(target); bytes.Contains(got, []byte("enter_accept")) || bytes.Contains(got, []byte("filter_mode_shell_up_key_binding")) || bytes.Contains(got, []byte(`name = "catppuccin-mocha"`)) {
		t.Fatalf("uninstalled target retained dots-owned values:\n%s", got)
	}
	for name, want := range map[string]string{
		"history.db": "history", "key": "private-key", "session": "auth-token", "host_id": "host", "records.db": "records",
	} {
		got, err := os.ReadFile(filepath.Join(dataDir, name))
		if err != nil || string(got) != want {
			t.Fatalf("Atuin runtime state %s = (%q, %v), want unchanged", name, got, err)
		}
	}
	if _, ok := loadInstallMetadata(t, stateRoot).FindByTarget(target); ok {
		t.Fatal("Atuin metadata record not pruned after partial uninstall")
	}
}

func atuinManifest(strategy, ownership string) manifest.Manifest {
	return manifest.Manifest{Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}}, Entries: []manifest.Entry{{
		Source: "configs/atuin/config.toml", Target: "~/.config/atuin/config.toml", Strategy: strategy, Ownership: ownership, Tags: []string{"core"}, OS: []string{"linux"},
	}}}
}

func assertContainsAll(t *testing.T, path string, values []string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, value := range values {
		if !bytes.Contains(content, []byte(value)) {
			t.Fatalf("%s missing %q:\n%s", path, value, content)
		}
	}
}

func loadInstallMetadata(t *testing.T, stateRoot string) state.Metadata {
	t.Helper()
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	return meta
}
