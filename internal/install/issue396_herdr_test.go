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

func TestHerdrTOMLSubsetLifecycleMigratesAdaptiveConfigAndPreservesExternalContent(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	defaultSource := filepath.Join(sourceRoot, "configs", "herdr", "config.toml")
	adaptiveSource := filepath.Join(sourceRoot, "configs", "herdr", "config-adaptive.toml")
	target := filepath.Join(home, ".config", "herdr", "config.toml")
	defaultBaseline := []byte("onboarding = true\n\n[theme]\nname = \"catppuccin\"\n\n[keys]\nprefix = \"ctrl+a\"\n")
	oldAdaptiveBaseline := []byte("onboarding = true\n\n[theme]\nname = \"catppuccin\"\nauto_switch = true\n\n[keys]\nprefix = \"ctrl+a\"\nreset = \"prefix+r\"\n")
	incomingAdaptiveBaseline := []byte("onboarding = true\n\n[theme]\nname = \"catppuccin\"\nauto_switch = true\nlight_name = \"catppuccin-latte\"\n\n[keys]\nprefix = \"ctrl+a\"\nreset = \"prefix+r\"\n")
	external := []byte("\n# retained exactly for a Herdr-owned extension\n[local]\ncustom = \"value\" # keep inline comment\n")

	writeMigrationFile(t, defaultSource, defaultBaseline)
	writeMigrationFile(t, adaptiveSource, oldAdaptiveBaseline)
	runMigrationGit(t, sourceRoot, "init")
	runMigrationGit(t, sourceRoot, "config", "user.email", "dots@example.test")
	runMigrationGit(t, sourceRoot, "config", "user.name", "dots test")
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "legacy Herdr baseline")
	oldRevision := strings.TrimSpace(runMigrationGit(t, sourceRoot, "rev-parse", "HEAD"))

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(adaptiveSource, target); err != nil {
		t.Fatalf("symlink legacy Herdr config: %v", err)
	}
	if err := os.WriteFile(adaptiveSource, append(append([]byte(nil), oldAdaptiveBaseline...), external...), 0o600); err != nil {
		t.Fatalf("simulate legacy Herdr target write: %v", err)
	}

	oldManifest := herdrManifest("symlink", "")
	newManifest := herdrManifest("copy", "toml-subset")
	meta := state.Metadata{
		Version:    state.CurrentVersion,
		Provenance: state.Provenance{SourceRoot: sourceRoot, SourceRevision: oldRevision},
		Entries: []state.Record{{
			Target: target, Source: "configs/herdr/config-adaptive.toml", Strategy: "symlink", Ownership: "whole",
		}},
	}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatalf("save legacy metadata: %v", err)
	}
	captures, err := repositoryrefresh.CaptureLegacyTargets(oldManifest, newManifest, meta, sourceRoot, home, "", oldRevision)
	if err != nil {
		t.Fatalf("CaptureLegacyTargets() error = %v", err)
	}
	if got := captures[target].CapturedContent; !bytes.Equal(got, append(append([]byte(nil), oldAdaptiveBaseline...), external...)) {
		t.Fatalf("captured Herdr config = %q", got)
	}

	writeMigrationFile(t, adaptiveSource, incomingAdaptiveBaseline)
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "materialize Herdr config")
	p, err := plan.Build(newManifest, plan.Options{
		Profile: "default", ExtraTags: []string{"adaptive-theme"}, OS: "darwin", SourceRoot: sourceRoot, Home: home,
		Metadata: meta, LegacyMigrations: captures,
	})
	if err != nil {
		t.Fatalf("Build(migration) error = %v", err)
	}
	if len(p.Actions) != 1 || p.Actions[0].Status != plan.StatusMigrate || p.Actions[0].Source != "configs/herdr/config-adaptive.toml" {
		t.Fatalf("migration plan = %#v, want one adaptive migrate action", p.Actions)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(migration) error = %v", err)
	}
	if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("materialized Herdr target = (%v, %v), want regular file", info, err)
	}
	assertContainsAll(t, target, []string{`light_name = "catppuccin-latte"`, `[keys]`, `prefix = "ctrl+a"`, string(external)})
	if got := runMigrationGit(t, sourceRoot, "status", "--porcelain"); got != "" {
		t.Fatalf("repository status after Herdr migration = %q, want clean", got)
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
	if !ok || rec.Source != "configs/herdr/config-adaptive.toml" || rec.Ownership != "toml-subset" || !bytes.Equal(rec.OwnedBytes, incomingAdaptiveBaseline) {
		t.Fatalf("Herdr ownership record = %#v, want adaptive TOML contribution", rec)
	}
	if len(rec.Contributions) != 1 || rec.Contributions[0].Source != "configs/herdr/config-adaptive.toml" ||
		!bytes.Equal(rec.Contributions[0].OwnedBytes, incomingAdaptiveBaseline) ||
		len(rec.Contributions[0].SelectorTags) != 2 || rec.Contributions[0].SelectorTags[0] != "core" || rec.Contributions[0].SelectorTags[1] != "adaptive-theme" {
		t.Fatalf("Herdr contribution attribution = %#v, want core + adaptive-theme exact TOML evidence", rec.Contributions)
	}
	report, err := status.Build(newManifest, meta, status.Options{Profile: "default", ExtraTags: []string{"adaptive-theme"}, OS: "darwin", SourceRoot: sourceRoot, Home: home})
	if err != nil || len(report.Entries) != 1 || report.Entries[0].State != status.StateOK {
		t.Fatalf("status after migration = (%#v, %v), want ok", report, err)
	}

	live, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read materialized target: %v", err)
	}
	afterOnboarding := bytes.Replace(live, []byte("onboarding = true"), []byte("onboarding = false"), 1)
	afterResetKeys := bytes.Replace(afterOnboarding, []byte("prefix = \"ctrl+a\"\nreset = \"prefix+r\"\n"), nil, 1)
	if err := os.WriteFile(target, afterResetKeys, 0o600); err != nil {
		t.Fatalf("simulate Herdr onboarding and config reset-keys: %v", err)
	}
	if got := runMigrationGit(t, sourceRoot, "status", "--porcelain"); got != "" {
		t.Fatalf("Herdr-supported target writes changed the Source of Truth: %q", got)
	}
	report, err = status.Build(newManifest, meta, status.Options{Profile: "default", ExtraTags: []string{"adaptive-theme"}, OS: "darwin", SourceRoot: sourceRoot, Home: home})
	if err != nil || report.Entries[0].State != status.StateDrifted {
		t.Fatalf("status after Herdr-owned changes = (%#v, %v), want drifted", report, err)
	}
	conflictPlan, err := plan.Build(newManifest, plan.Options{Profile: "default", ExtraTags: []string{"adaptive-theme"}, OS: "darwin", SourceRoot: sourceRoot, Home: home, Metadata: meta})
	if err != nil || conflictPlan.Actions[0].Status != plan.StatusConflict {
		t.Fatalf("plan after Herdr-owned changes = (%#v, %v), want conflict", conflictPlan, err)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, afterResetKeys) {
		t.Fatalf("owned Herdr changes were overwritten = (%q, %v)", got, err)
	}
	if err := os.WriteFile(target, live, 0o600); err != nil {
		t.Fatalf("restore compatible Herdr target: %v", err)
	}

	updatedAdaptiveBaseline := []byte("onboarding = false\n\n[theme]\nname = \"catppuccin\"\nauto_switch = true\ndark_name = \"catppuccin\"\nlight_name = \"catppuccin-latte\"\n\n[keys]\nprefix = \"ctrl+a\"\n")
	writeMigrationFile(t, adaptiveSource, updatedAdaptiveBaseline)
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "update Herdr adaptive baseline")
	updatePlan, err := plan.Build(newManifest, plan.Options{Profile: "default", ExtraTags: []string{"adaptive-theme"}, OS: "darwin", SourceRoot: sourceRoot, Home: home, Metadata: meta})
	if err != nil || updatePlan.Actions[0].Status != plan.StatusUpdate {
		t.Fatalf("adaptive baseline update plan = (%#v, %v), want update", updatePlan, err)
	}
	if err := install.Apply(updatePlan, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(update) error = %v", err)
	}
	assertContainsAll(t, target, []string{`onboarding = false`, `dark_name = "catppuccin"`, `prefix = "ctrl+a"`, string(external)})
	if got, _ := os.ReadFile(target); bytes.Contains(got, []byte(`reset = "prefix+r"`)) {
		t.Fatalf("updated Herdr target retained retired owned key:\n%s", got)
	}

	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("reload updated metadata: %v", err)
	}
	rec, ok = meta.FindByTarget(target)
	if !ok || rec.Source != "configs/herdr/config-adaptive.toml" || !bytes.Equal(rec.OwnedBytes, updatedAdaptiveBaseline) {
		t.Fatalf("updated Herdr ownership record = %#v", rec)
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
		t.Fatalf("uninstall result = %#v, want preserved external Herdr config", result)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Contains(got, external) {
		t.Fatalf("external Herdr content after uninstall = (%q, %v), want retained exactly", got, err)
	}
	for _, removed := range []string{"onboarding", "prefix", "light_name", "dark_name"} {
		if bytes.Contains(got, []byte(removed)) {
			t.Fatalf("uninstalled target retained dots-owned %q:\n%s", removed, got)
		}
	}
	if _, ok := loadInstallMetadata(t, stateRoot).FindByTarget(target); ok {
		t.Fatal("Herdr metadata record not pruned after partial uninstall")
	}
}

func TestHerdrLegacySymlinkWithoutProvenanceFailsClosed(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "configs", "herdr", "config.toml")
	target := filepath.Join(home, ".config", "herdr", "config.toml")
	writeMigrationFile(t, source, []byte("onboarding = false\n"))
	runMigrationGit(t, sourceRoot, "init")
	runMigrationGit(t, sourceRoot, "config", "user.email", "dots@example.test")
	runMigrationGit(t, sourceRoot, "config", "user.name", "dots test")
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "legacy Herdr baseline")
	revision := strings.TrimSpace(runMigrationGit(t, sourceRoot, "rev-parse", "HEAD"))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("symlink legacy Herdr config: %v", err)
	}
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{{
		Target: target, Source: "configs/herdr/config.toml", Strategy: "symlink", Ownership: "whole",
	}}}
	captures, err := repositoryrefresh.CaptureLegacyTargets(herdrManifest("symlink", ""), herdrManifest("copy", "toml-subset"), meta, sourceRoot, home, "", revision)
	if err != nil {
		t.Fatalf("CaptureLegacyTargets() error = %v", err)
	}
	if len(captures) != 0 {
		t.Fatalf("captures without provenance = %#v, want none", captures)
	}
	p, err := plan.Build(herdrManifest("copy", "toml-subset"), plan.Options{Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home, Metadata: meta})
	if err != nil || len(p.Actions) != 1 || p.Actions[0].Status != plan.StatusConflict {
		t.Fatalf("plan without migration provenance = (%#v, %v), want conflict", p.Actions, err)
	}
	if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("ambiguous legacy target changed = (%v, %v), want untouched symlink", info, err)
	}
}

func herdrManifest(strategy, ownership string) manifest.Manifest {
	return manifest.Manifest{Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}}, Entries: []manifest.Entry{{
		Source: "configs/herdr/config.toml", SourceOverrides: map[string]string{"adaptive-theme": "configs/herdr/config-adaptive.toml"},
		Target: "~/.config/herdr/config.toml", Strategy: strategy, Ownership: ownership, Tags: []string{"core"}, OS: []string{"darwin"},
	}}}
}
