package install_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/ownershipevidence"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func TestApplyComposedJSONSubsetCreatesAndUpdatesOneSharedTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	for rel, content := range map[string]string{
		"configs/base.json":   `{"editor":{"theme":"dark"},"servers":["one"]}`,
		"configs/mobile.json": `{"mobile":true,"servers":["two"]}`,
	} {
		path := filepath.Join(sourceRoot, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir source: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}
	}
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"base", "mobile"}}},
		Entries: []manifest.Entry{
			{Source: "configs/base.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"base"}},
			{Source: "configs/mobile.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"mobile"}},
		},
	}
	opts := plan.Options{Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home}
	p, err := plan.Build(m, opts)
	if err != nil {
		t.Fatalf("Build(create) error = %v", err)
	}
	if len(p.Actions) != 1 {
		t.Fatalf("create actions = %d, want one", len(p.Actions))
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(create) error = %v", err)
	}

	target := filepath.Join(home, ".config", "shared.json")
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatalf("metadata missing target %s", target)
	}
	wantSources := []string{"configs/base.json", "configs/mobile.json"}
	if !reflect.DeepEqual(rec.SourceList(), wantSources) {
		t.Fatalf("metadata sources = %#v, want %#v", rec.SourceList(), wantSources)
	}
	targetHash, err := state.HashFile(target)
	if err != nil {
		t.Fatalf("hash target: %v", err)
	}
	if rec.Hash != targetHash {
		t.Fatalf("metadata hash = %q, want composed target hash %q", rec.Hash, targetHash)
	}
	if len(rec.Contributions) != 2 {
		t.Fatalf("metadata contributions = %+v, want two attributed sources", rec.Contributions)
	}
	for i, want := range []struct {
		source string
		tag    string
		owned  string
	}{
		{source: "configs/base.json", tag: "base", owned: `{"editor":{"theme":"dark"},"servers":["one"]}`},
		{source: "configs/mobile.json", tag: "mobile", owned: `{"mobile":true,"servers":["two"]}`},
	} {
		contribution := rec.Contributions[i]
		if contribution.Source != want.source ||
			!reflect.DeepEqual(contribution.SelectorTags, []string{want.tag}) ||
			contribution.Ownership != "json-subset" ||
			contribution.Hash != state.HashBytes([]byte(want.owned)) {
			t.Fatalf("metadata contribution[%d] = %+v, want source %q selected by %q with exact JSON evidence", i, contribution, want.source, want.tag)
		}
		var gotOwned, wantOwned any
		if err := json.Unmarshal(contribution.OwnedContent, &gotOwned); err != nil {
			t.Fatalf("decode contribution[%d] owned content: %v", i, err)
		}
		if err := json.Unmarshal([]byte(want.owned), &wantOwned); err != nil {
			t.Fatalf("decode wanted contribution[%d] owned content: %v", i, err)
		}
		if !reflect.DeepEqual(gotOwned, wantOwned) {
			t.Fatalf("metadata contribution[%d] owned content = %v, want %v", i, gotOwned, wantOwned)
		}
	}

	if err := os.WriteFile(target, []byte(`{"editor":{"theme":"dark"},"servers":["one"],"userOnly":"keep"}`), 0o640); err != nil {
		t.Fatalf("write trusted target: %v", err)
	}
	opts.Metadata = meta
	p, err = plan.Build(m, opts)
	if err != nil {
		t.Fatalf("Build(update) error = %v", err)
	}
	if len(p.Actions) != 1 || p.Actions[0].Status != plan.StatusUpdate {
		t.Fatalf("update actions = %+v, want one update", p.Actions)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(update) error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read updated target: %v", err)
	}
	for _, want := range []string{`"userOnly":"keep"`, `"mobile":true`, `"two"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("updated target missing %s:\n%s", want, got)
		}
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(backupMeta.Sets) != 1 {
		t.Fatalf("backup sets = %d, want one shared-target backup", len(backupMeta.Sets))
	}
	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("reload metadata: %v", err)
	}
	uninstallPlan, err := plan.BuildUninstall(meta, plan.UninstallOptions{SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("BuildUninstall() error = %v", err)
	}
	var shared *plan.UninstallAction
	for i := range uninstallPlan.Actions {
		if uninstallPlan.Actions[i].Target == target {
			shared = &uninstallPlan.Actions[i]
			break
		}
	}
	if shared == nil || shared.Status != plan.UninstallRemove || shared.Ownership != "json-subset" {
		t.Fatalf("shared uninstall action = %+v, want partial removal that preserves target-only state", shared)
	}
}

func TestApplyRecordsModeSpecificContributionEvidence(t *testing.T) {
	tests := []struct {
		name      string
		strategy  string
		ownership string
		content   string
		assert    func(*testing.T, state.Contribution, []byte)
	}{
		{
			name: "whole copy", strategy: "copy", content: "whole\n",
			assert: func(t *testing.T, got state.Contribution, content []byte) {
				t.Helper()
				if got.Hash != state.HashBytes(content) || len(got.OwnedContent) != 0 || len(got.OwnedBytes) != 0 || len(got.SeededBaseline) != 0 {
					t.Fatalf("whole-copy evidence = %+v, want source hash only", got)
				}
			},
		},
		{
			name: "whole symlink", strategy: "symlink", content: "linked\n",
			assert: func(t *testing.T, got state.Contribution, _ []byte) {
				t.Helper()
				if got.Hash != "" || len(got.OwnedContent) != 0 || len(got.OwnedBytes) != 0 || len(got.SeededBaseline) != 0 {
					t.Fatalf("whole-symlink evidence = %+v, want Source of Truth identity only", got)
				}
			},
		},
		{
			name: "json subset", strategy: "copy", ownership: "json-subset", content: `{"owned":true}`,
			assert: func(t *testing.T, got state.Contribution, _ []byte) {
				t.Helper()
				if !json.Valid(got.OwnedContent) {
					t.Fatalf("json-subset evidence = %+v, want owned JSON", got)
				}
			},
		},
		{
			name: "jsonc subset", strategy: "copy", ownership: "jsonc-subset", content: "{\n  // portable\n  \"owned\": true,\n}\n",
			assert: func(t *testing.T, got state.Contribution, _ []byte) {
				t.Helper()
				if !json.Valid(got.OwnedContent) || strings.Contains(string(got.OwnedContent), "portable") {
					t.Fatalf("jsonc-subset evidence = %+v, want canonical semantic JSON", got)
				}
			},
		},
		{
			name: "toml subset", strategy: "copy", ownership: "toml-subset", content: "owned = true\n",
			assert: func(t *testing.T, got state.Contribution, content []byte) {
				t.Helper()
				if !reflect.DeepEqual(got.OwnedBytes, content) {
					t.Fatalf("toml-subset evidence = %q, want %q", got.OwnedBytes, content)
				}
			},
		},
		{
			name: "empty toml subset", strategy: "copy", ownership: "toml-subset", content: "",
			assert: func(t *testing.T, got state.Contribution, _ []byte) {
				t.Helper()
				if !got.EvidenceRecorded || len(got.OwnedBytes) != 0 {
					t.Fatalf("empty toml-subset evidence = %+v, want recorded empty bytes", got)
				}
			},
		},
		{
			name: "marked block", strategy: "copy", ownership: "marked-block", content: "# >>> dots managed block >>>\nsource portable\n# <<< dots managed block <<<\n",
			assert: func(t *testing.T, got state.Contribution, content []byte) {
				t.Helper()
				if !reflect.DeepEqual(got.OwnedBytes, content) {
					t.Fatalf("marked-block evidence = %q, want %q", got.OwnedBytes, content)
				}
			},
		},
		{
			name: "seeded runtime state", strategy: "copy", ownership: "seeded", content: "seeded baseline\n",
			assert: func(t *testing.T, got state.Contribution, content []byte) {
				t.Helper()
				if !reflect.DeepEqual(got.SeededBaseline, content) {
					t.Fatalf("seeded evidence = %q, want %q", got.SeededBaseline, content)
				}
			},
		},
		{
			name: "empty seeded runtime state", strategy: "copy", ownership: "seeded", content: "",
			assert: func(t *testing.T, got state.Contribution, _ []byte) {
				t.Helper()
				if !got.EvidenceRecorded || len(got.SeededBaseline) != 0 {
					t.Fatalf("empty seeded evidence = %+v, want recorded empty baseline", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRoot := t.TempDir()
			home := t.TempDir()
			stateRoot := filepath.Join(home, ".local", "state", "dots")
			rel := filepath.Join("configs", strings.ReplaceAll(tt.name, " ", "-"))
			source := filepath.Join(sourceRoot, rel)
			if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
				t.Fatalf("mkdir source: %v", err)
			}
			content := []byte(tt.content)
			if err := os.WriteFile(source, content, 0o600); err != nil {
				t.Fatalf("write source: %v", err)
			}
			m := manifest.Manifest{
				Version:  1,
				Profiles: map[string]manifest.Profile{"default": {Tags: []string{"capability"}}},
				Entries: []manifest.Entry{{
					Source: rel, Target: "~/.config/" + strings.ReplaceAll(tt.name, " ", "-"),
					Strategy: tt.strategy, Ownership: tt.ownership, Tags: []string{"capability"},
				}},
			}
			p, err := plan.Build(m, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(meta.Entries) != 1 || len(meta.Entries[0].Contributions) != 1 {
				t.Fatalf("metadata = %+v, want one target with one contribution", meta)
			}
			contribution := meta.Entries[0].Contributions[0]
			wantOwnership := tt.ownership
			if wantOwnership == "" {
				wantOwnership = "whole"
			}
			if contribution.Source != rel || !reflect.DeepEqual(contribution.SelectorTags, []string{"capability"}) || contribution.Ownership != wantOwnership || !contribution.EvidenceRecorded {
				t.Fatalf("contribution identity = %+v, want %q selected by capability with %q ownership", contribution, rel, wantOwnership)
			}
			tt.assert(t, contribution, content)
		})
	}
}

func TestApplyContributionEvidencePreservesUnrelatedInventoryAndInstalledSelection(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "new")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	selection := &state.InstalledSelection{
		Profiles: []string{"existing"}, ExtraTags: []string{"extra"}, ResolvedTags: []string{"existing", "extra"},
		Provenance: state.Provenance{SourceRoot: "/previous", SourceRevision: "abc123", RecordedAt: "2026-08-21T00:00:00Z"},
	}
	unrelated := state.Record{Target: filepath.Join(home, ".unrelated"), Source: "configs/unrelated", Strategy: "copy", Ownership: "whole", Hash: "unchanged"}
	provisioners := []state.ProvisionerRecord{{Profile: "existing", Tool: "codex", Executable: "codex", Args: []string{"mcp", "add"}, Status: "provisioned"}}
	previous := state.Metadata{Version: 6, Entries: []state.Record{unrelated}, Provisioners: provisioners, InstalledSelection: selection}
	if err := state.Save(state.Path(stateRoot), previous); err != nil {
		t.Fatalf("save previous metadata: %v", err)
	}
	m := manifest.Manifest{
		Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"new"}}},
		Entries: []manifest.Entry{{Source: "configs/new", Target: "~/.new", Strategy: "copy", Tags: []string{"new"}}},
	}
	p, err := plan.Build(m, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: previous})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Version != state.CurrentVersion || !reflect.DeepEqual(got.InstalledSelection, selection) || !reflect.DeepEqual(got.Provisioners, provisioners) {
		t.Fatalf("metadata terminal state = %+v, want v%d with prior selection and Provisioners", got, state.CurrentVersion)
	}
	if kept, ok := got.FindByTarget(unrelated.Target); !ok || !reflect.DeepEqual(kept, unrelated) {
		t.Fatalf("unrelated inventory = %+v, want preserved %+v", kept, unrelated)
	}
	newRecord, ok := got.FindByTarget(filepath.Join(home, ".new"))
	if !ok || len(newRecord.Contributions) != 1 || !reflect.DeepEqual(newRecord.Contributions[0].SelectorTags, []string{"new"}) {
		t.Fatalf("new record = %+v, want terminal contribution evidence", newRecord)
	}
}

func TestApplyManagedEntriesCommitsContributionEvidenceAndSelectionAtomically(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "new")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	previousSelection := state.InstalledSelection{Profiles: []string{"previous"}, ResolvedTags: []string{"previous"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previousSelection}); err != nil {
		t.Fatalf("save previous metadata: %v", err)
	}
	m := manifest.Manifest{
		Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"new"}}},
		Entries: []manifest.Entry{{Source: "configs/new", Target: "~/.new", Strategy: "copy", Tags: []string{"new"}}},
	}
	p, err := plan.Build(m, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	commit, err := install.ApplyManagedEntries(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("ApplyManagedEntries() error = %v", err)
	}

	staged, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load staged metadata: %v", err)
	}
	if len(staged.Entries) != 1 || len(staged.Entries[0].Contributions) != 0 {
		t.Fatalf("staged Entries = %+v, want compatibility inventory without exact evidence", staged.Entries)
	}
	if staged.InstalledSelection == nil || !reflect.DeepEqual(*staged.InstalledSelection, previousSelection) {
		t.Fatalf("staged InstalledSelection = %+v, want previous %+v", staged.InstalledSelection, previousSelection)
	}
	provisioners := []state.ProvisionerRecord{{Tool: "codex", Executable: "codex", Status: "provisioned"}}
	staged.Provisioners = provisioners
	if err := state.Save(state.Path(stateRoot), staged); err != nil {
		t.Fatalf("save terminal Provisioner inventory: %v", err)
	}

	installed := state.InstalledSelection{Profiles: []string{"default"}, ResolvedTags: []string{"new"}}
	if err := commit.Commit(&installed); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	final, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load final metadata: %v", err)
	}
	if len(final.Entries) != 1 || len(final.Entries[0].Contributions) != 1 || !reflect.DeepEqual(final.Entries[0].Contributions[0].SelectorTags, []string{"new"}) {
		t.Fatalf("final Entries = %+v, want exact selected contribution evidence", final.Entries)
	}
	if final.InstalledSelection == nil || !reflect.DeepEqual(*final.InstalledSelection, installed) {
		t.Fatalf("final InstalledSelection = %+v, want %+v", final.InstalledSelection, installed)
	}
	if !reflect.DeepEqual(final.Provisioners, provisioners) {
		t.Fatalf("final Provisioners = %+v, want preserved %+v", final.Provisioners, provisioners)
	}
}

func TestMetadataCommitRejectsConcurrentDriftWithoutStrengtheningEvidence(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "new")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	previousSelection := state.InstalledSelection{Profiles: []string{"previous"}, ResolvedTags: []string{"previous"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previousSelection}); err != nil {
		t.Fatalf("save previous metadata: %v", err)
	}
	m := manifest.Manifest{
		Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"new"}}},
		Entries: []manifest.Entry{{Source: "configs/new", Target: "~/.new", Strategy: "copy", Tags: []string{"new"}}},
	}
	p, err := plan.Build(m, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	commit, err := install.ApplyManagedEntries(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("ApplyManagedEntries() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".new"), []byte("concurrent drift\n"), 0o600); err != nil {
		t.Fatalf("write concurrent Drift: %v", err)
	}
	installed := state.InstalledSelection{Profiles: []string{"default"}, ResolvedTags: []string{"new"}}
	if err := commit.Commit(&installed); !errors.Is(err, ownershipevidence.ErrDrift) {
		t.Fatalf("Commit() error = %v, want ErrDrift", err)
	}

	final, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata after Drift: %v", err)
	}
	if len(final.Entries) != 1 || len(final.Entries[0].Contributions) != 0 {
		t.Fatalf("Entries after Drift = %+v, want compatibility inventory without exact evidence", final.Entries)
	}
	if final.InstalledSelection == nil || !reflect.DeepEqual(*final.InstalledSelection, previousSelection) {
		t.Fatalf("InstalledSelection after Drift = %+v, want previous %+v", final.InstalledSelection, previousSelection)
	}
}

func TestMetadataCommitRejectsChangedSourceEvidenceEvenWhenTargetContainsIt(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "shared.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte(`{"owned":true,"retired":true}`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	previousSelection := state.InstalledSelection{Profiles: []string{"previous"}, ResolvedTags: []string{"previous"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previousSelection}); err != nil {
		t.Fatalf("save previous metadata: %v", err)
	}
	m := manifest.Manifest{
		Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"shared"}}},
		Entries: []manifest.Entry{{
			Source: "configs/shared.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"shared"},
		}},
	}
	p, err := plan.Build(m, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	commit, err := install.ApplyManagedEntries(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("ApplyManagedEntries() error = %v", err)
	}
	// The live target still contains this reduced subset, but it is not the
	// exact Source of Truth contribution that produced convergence.
	if err := os.WriteFile(source, []byte(`{"owned":true}`), 0o600); err != nil {
		t.Fatalf("change Source of Truth after staging: %v", err)
	}
	installed := state.InstalledSelection{Profiles: []string{"default"}, ResolvedTags: []string{"shared"}}
	if err := commit.Commit(&installed); !errors.Is(err, ownershipevidence.ErrDrift) {
		t.Fatalf("Commit() error = %v, want changed source ErrDrift", err)
	}

	final, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata after source change: %v", err)
	}
	if len(final.Entries) != 1 || len(final.Entries[0].Contributions) != 0 {
		t.Fatalf("Entries after source change = %+v, want compatibility inventory without exact evidence", final.Entries)
	}
	if final.InstalledSelection == nil || !reflect.DeepEqual(*final.InstalledSelection, previousSelection) {
		t.Fatalf("InstalledSelection after source change = %+v, want previous %+v", final.InstalledSelection, previousSelection)
	}
}

func TestMetadataCommitRejectsChangedRelativeSourceRootIdentity(t *testing.T) {
	workspace := t.TempDir()
	firstWorkingDir := filepath.Join(workspace, "first")
	secondWorkingDir := filepath.Join(workspace, "second")
	for _, workingDir := range []string{firstWorkingDir, secondWorkingDir} {
		if err := os.MkdirAll(filepath.Join(workingDir, "repo", "configs"), 0o755); err != nil {
			t.Fatalf("mkdir source root: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workingDir, "repo", "configs", "tool.conf"), []byte("same content\n"), 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}
	}
	t.Chdir(firstWorkingDir)

	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	firstSource := filepath.Join(firstWorkingDir, "repo", "configs", "tool.conf")
	target := filepath.Join(home, ".config", "tool.conf")
	p := plan.Plan{Actions: []plan.Action{{
		Source:         "configs/tool.conf",
		ResolvedSource: firstSource,
		Target:         target,
		Strategy:       "copy",
		Status:         plan.StatusCreate,
	}}}
	commit, err := install.ApplyManagedEntries(p, install.Options{SourceRoot: "repo", Home: home, StateRoot: stateRoot})
	if err != nil {
		t.Fatalf("ApplyManagedEntries() error = %v", err)
	}
	t.Chdir(secondWorkingDir)

	installed := state.InstalledSelection{ExtraTags: []string{"tool"}, ResolvedTags: []string{"tool"}}
	if err := commit.Commit(&installed); err == nil || !strings.Contains(err.Error(), "terminal source") || !strings.Contains(err.Error(), "after applying from") {
		t.Fatalf("Commit() error = %v, want changed terminal source identity error", err)
	}

	final, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata after source identity change: %v", err)
	}
	if len(final.Entries) != 1 || len(final.Entries[0].Contributions) != 0 {
		t.Fatalf("Entries after source identity change = %+v, want compatibility inventory without exact evidence", final.Entries)
	}
	if final.InstalledSelection != nil {
		t.Fatalf("InstalledSelection after source identity change = %+v, want nil", final.InstalledSelection)
	}
}

func TestApplyMetadataWriteFailurePreservesPreviousContributionEvidence(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "new")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	previous := state.Metadata{Version: 6, Entries: []state.Record{{Target: filepath.Join(home, ".old"), Source: "configs/old", Strategy: "copy", Hash: "old"}}}
	metadataPath := state.Path(stateRoot)
	if err := state.Save(metadataPath, previous); err != nil {
		t.Fatalf("save previous metadata: %v", err)
	}
	previousBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read previous metadata: %v", err)
	}
	m := manifest.Manifest{
		Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"new"}}},
		Entries: []manifest.Entry{{Source: "configs/new", Target: "~/.new", Strategy: "copy", Tags: []string{"new"}}},
	}
	p, err := plan.Build(m, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: previous})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := os.Chmod(stateRoot, 0o500); err != nil {
		t.Fatalf("make state root read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateRoot, 0o700) })

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil {
		t.Skip("filesystem permits metadata writes to a read-only state root")
	}
	unchanged, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read metadata after failed write: %v", err)
	}
	if !reflect.DeepEqual(unchanged, previousBytes) {
		t.Fatalf("metadata after failed write = %q, want previous bytes %q", unchanged, previousBytes)
	}
	got, err := state.Load(metadataPath)
	if err != nil {
		t.Fatalf("load metadata after failed write: %v", err)
	}
	if got.Version != 6 || len(got.Entries) != 1 || len(got.Entries[0].Contributions) != 0 {
		t.Fatalf("metadata after failed write = %+v, want untouched previous evidence", got)
	}
}

func TestApplyRejectsStaleUnchangedPlanWithoutRecordingContributionEvidence(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "shared.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte(`{"owned":"portable"}`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".config", "shared.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"owned":"portable","local":true}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	previous := state.Metadata{Version: 6, Entries: []state.Record{{
		Target: target, Source: "configs/shared.json", Strategy: "copy", Ownership: "json-subset",
		OwnedContent: []byte(`{"owned":"portable"}`),
	}}}
	metadataPath := state.Path(stateRoot)
	if err := state.Save(metadataPath, previous); err != nil {
		t.Fatalf("save previous metadata: %v", err)
	}
	previousBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read previous metadata: %v", err)
	}
	m := manifest.Manifest{
		Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"shared"}}},
		Entries: []manifest.Entry{{
			Source: "configs/shared.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"shared"},
		}},
	}
	p, err := plan.Build(m, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: previous})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Actions) != 1 || p.Actions[0].Status != plan.StatusUnchanged {
		t.Fatalf("plan = %+v, want one unchanged action", p)
	}
	if err := os.WriteFile(target, []byte(`{"owned":"drifted","local":true}`), 0o600); err != nil {
		t.Fatalf("drift target after planning: %v", err)
	}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil || !strings.Contains(err.Error(), "no longer converged") {
		t.Fatalf("Apply() error = %v, want stale convergence rejection", err)
	}
	unchanged, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("read metadata after rejected apply: %v", err)
	}
	if !reflect.DeepEqual(unchanged, previousBytes) {
		t.Fatalf("metadata after rejected apply = %q, want previous bytes %q", unchanged, previousBytes)
	}
}

func TestApplyMigrationCreatesContentBackupAndRegularTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "app.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	current := []byte("{\"owned\":2}\n")
	if err := os.WriteFile(source, current, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".config", "app.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}
	captured := []byte("{\"owned\":1,\"runtime\":true}\n")
	final := []byte("{\n  \"owned\": 2,\n  \"runtime\": true\n}\n")
	p := plan.Plan{Actions: []plan.Action{{
		Source: "configs/app.json", ResolvedSource: source, Target: target, Strategy: "copy", Ownership: "json-subset", Status: plan.StatusMigrate,
		Migration: &plan.LegacyMigration{LinkDestination: source, CapturedContent: captured, ExpectedLinkContent: current, FinalContent: final},
	}}}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("migrated target mode = %v, err=%v", info, err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != string(final) {
		t.Fatalf("target = %q", got)
	}
	metadata, err := backups.Load(backups.Path(stateRoot))
	if err != nil || len(metadata.Sets) != 1 {
		t.Fatalf("backup metadata = %#v, err=%v", metadata, err)
	}
	backup, err := os.ReadFile(backups.FilePath(stateRoot, metadata.Sets[0].ID, 1, target))
	if err != nil || string(backup) != string(captured) {
		t.Fatalf("backup = %q, err=%v", backup, err)
	}
}

func TestApplyMigrationRejectsConcurrentTargetChangeBeforeBackup(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".state")
	source := filepath.Join(sourceRoot, "app")
	if err := os.WriteFile(source, []byte("changed after plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".app")
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{Source: "app", ResolvedSource: source, Target: target, Strategy: "copy", Status: plan.StatusMigrate, Migration: &plan.LegacyMigration{LinkDestination: source, CapturedContent: []byte("old\n"), ExpectedLinkContent: []byte("expected\n"), FinalContent: []byte("final\n")}}}}
	err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot})
	if err == nil || !strings.Contains(err.Error(), "content changed") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(backups.Path(stateRoot)); !os.IsNotExist(statErr) {
		t.Fatalf("backup metadata created after stale plan: %v", statErr)
	}
}

func TestApplyCreatesSymlinkForCreateAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "zsh", ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/zsh/zshrc",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink", info.Mode())
	}
	gotDest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink target: %v", err)
	}
	if gotDest != sourcePath {
		t.Fatalf("symlink target = %q, want %q", gotDest, sourcePath)
	}
}

func TestApplyDefaultsConflictActionsToSkipAndContinuesSafeCreates(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	createSource := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(createSource), 0o755); err != nil {
		t.Fatalf("mkdir create source: %v", err)
	}
	if err := os.WriteFile(createSource, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write create source: %v", err)
	}
	conflictSource := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(conflictSource), 0o755); err != nil {
		t.Fatalf("mkdir conflict source: %v", err)
	}
	if err := os.WriteFile(conflictSource, []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write conflict source: %v", err)
	}

	createdTarget := filepath.Join(home, ".zshrc")
	conflictingTarget := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(conflictingTarget, []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write conflicting target: %v", err)
	}
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: createdTarget, Strategy: "symlink", Status: plan.StatusCreate},
		{Source: "configs/git/gitconfig", Target: conflictingTarget, Strategy: "copy", Status: plan.StatusConflict},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Lstat(createdTarget); err != nil {
		t.Fatalf("created target missing after conflict skip default: %v", err)
	}
	got, err := os.ReadFile(conflictingTarget)
	if err != nil {
		t.Fatalf("read conflicting target: %v", err)
	}
	if string(got) != "local\n" {
		t.Fatalf("conflicting target = %q, want original local content", got)
	}
}

func TestApplyReplaceConflictCreatesBackupSetBeforeChangingTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o640); err != nil {
		t.Fatalf("write local target: %v", err)
	}
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/git/gitconfig",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusConflict,
	}}}

	if err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		StateRoot:  stateRoot,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionReplace,
		},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced target: %v", err)
	}
	if string(got) != "managed\n" {
		t.Fatalf("target contents = %q, want managed source", got)
	}
	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("Backup Sets = %d, want 1", len(meta.Sets))
	}
	if len(meta.Sets[0].Targets) != 1 || meta.Sets[0].Targets[0] != target {
		t.Fatalf("Backup Set targets = %v, want [%s]", meta.Sets[0].Targets, target)
	}
}

func TestApplyReplaceConflictReplacesDirectoryTargetWithSymlink(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	sourceDir := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(filepath.Join(sourceDir, "lua"), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "init.lua"), []byte("-- managed\n"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	target := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(filepath.Join(target, "plugin"), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	localFile := filepath.Join(target, "plugin", "local.lua")
	if err := os.WriteFile(localFile, []byte("-- local\n"), 0o600); err != nil {
		t.Fatalf("write local target file: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusConflict,
	}}}

	if err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		StateRoot:  stateRoot,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionReplace,
		},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink", info.Mode())
	}
	dest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink target: %v", err)
	}
	if dest != sourceDir {
		t.Fatalf("symlink dest = %q, want %q", dest, sourceDir)
	}

	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("Backup Sets = %d, want 1", len(meta.Sets))
	}
	preserved := filepath.Join(backups.FilePath(stateRoot, meta.Sets[0].ID, 1, target), "plugin", "local.lua")
	got, err := os.ReadFile(preserved)
	if err != nil {
		t.Fatalf("read preserved directory file: %v", err)
	}
	if string(got) != "-- local\n" {
		t.Fatalf("preserved directory file = %q, want local content", got)
	}
}

func TestApplyReplaceConflictRemovesNonWritableDirectoryTargetWithSymlink(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	sourceDir := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "init.lua"), []byte("-- managed\n"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	target := filepath.Join(home, ".config", "nvim")
	lockedDir := filepath.Join(target, "plugin")
	if err := os.MkdirAll(lockedDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	localFile := filepath.Join(lockedDir, "local.lua")
	if err := os.WriteFile(localFile, []byte("-- local\n"), 0o400); err != nil {
		t.Fatalf("write local target file: %v", err)
	}
	if err := os.Chmod(lockedDir, 0o555); err != nil {
		t.Fatalf("chmod locked target dir: %v", err)
	}
	t.Cleanup(func() {
		makeTreeWritableForCleanup(home)
		makeTreeWritableForCleanup(stateRoot)
	})

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusConflict,
	}}}

	if err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		StateRoot:  stateRoot,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionReplace,
		},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink", info.Mode())
	}
	dest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink target: %v", err)
	}
	if dest != sourceDir {
		t.Fatalf("symlink dest = %q, want %q", dest, sourceDir)
	}

	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("Backup Sets = %d, want 1", len(meta.Sets))
	}
	preserved := filepath.Join(backups.FilePath(stateRoot, meta.Sets[0].ID, 1, target), "plugin", "local.lua")
	got, err := os.ReadFile(preserved)
	if err != nil {
		t.Fatalf("read preserved directory file: %v", err)
	}
	if string(got) != "-- local\n" {
		t.Fatalf("preserved directory file = %q, want local content", got)
	}
}

func TestApplyAdoptConflictCopiesTargetIntoSourceAndRecordsMetadata(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o640); err != nil {
		t.Fatalf("write local target: %v", err)
	}
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/git/gitconfig",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusConflict,
	}}}

	if err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		StateRoot:  stateRoot,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionAdopt,
		},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	gotSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read adopted source: %v", err)
	}
	if string(gotSource) != "local\n" {
		t.Fatalf("source contents = %q, want adopted local target", gotSource)
	}
	gotTarget, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(gotTarget) != "local\n" {
		t.Fatalf("target contents = %q, want local target left in place", gotTarget)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Installation Metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatalf("Installation Metadata missing adopted target %s", target)
	}
	wantHash, err := state.HashFile(sourcePath)
	if err != nil {
		t.Fatalf("hash adopted source: %v", err)
	}
	if rec.Hash != wantHash {
		t.Fatalf("record hash = %q, want adopted source hash %q", rec.Hash, wantHash)
	}
}

func makeTreeWritableForCleanup(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			_ = os.Chmod(path, 0o755)
		}
		return nil
	})
}

func TestApplyCopiesRegularFileForCreateAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("[user]\n\tname = Test\n"), 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "git", "config")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/git/gitconfig",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read copied target: %v", err)
	}
	if string(got) != "[user]\n\tname = Test\n" {
		t.Fatalf("copied contents = %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat copied target: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o640 {
		t.Fatalf("copied mode = %v, want %v", gotMode, os.FileMode(0o640))
	}
}

func TestApplyLeavesUnchangedActionUntouched(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/zsh/zshrc",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusUnchanged,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: filepath.Join(home, "missing-source-root"), Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "existing\n" {
		t.Fatalf("target contents = %q, want existing content", got)
	}
}

func TestApplyStatusUpdateMergesJSONSubsetAndRecordsMetadata(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte(`{
  "permissions": {
    "defaultMode": "bypassPermissions",
    "allow": ["Read", "Bash(git *)"]
  }
}`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{
  "permissions": {
    "allow": ["Read"],
    "deny": ["Bash(rm -rf *)"]
  },
  "enabledPlugins": {
    "chrome-devtools-mcp": true
  }
}`), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}

	p := plan.Plan{Profile: "core", Actions: []plan.Action{{
		Source:    "configs/claude/settings.json",
		Target:    target,
		Strategy:  "copy",
		Status:    plan.StatusUpdate,
		Ownership: "json-subset",
	}}}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	for _, want := range []string{
		`"defaultMode": "bypassPermissions"`,
		`"Bash(git *)"`,
		`"deny"`,
		`"enabledPlugins"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("target missing %q\ncontent:\n%s", want, got)
		}
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o640 {
		t.Fatalf("target mode = %v, want 0640", gotMode)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatalf("metadata missing target %s", target)
	}
	if rec.Source != "configs/claude/settings.json" {
		t.Fatalf("metadata source = %q, want Claude settings source", rec.Source)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(backupMeta.Sets) != 1 {
		t.Fatalf("backup sets = %d, want 1", len(backupMeta.Sets))
	}
}

func TestApplyReconcilesRecordedJSONContributionAndPreservesExternalContent(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "shared.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	previous := []byte(`{"owned":{"keep":true,"retired":"old"},"items":["old"]}`)
	current := []byte(`{"owned":{"keep":true,"added":"new"},"items":["new"]}`)
	if err := os.WriteFile(source, current, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".config", "shared.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"owned":{"keep":true,"retired":"old","external":"preserve"},"items":["old","external"],"targetOnly":true}`), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{{
		Target: target, Source: "configs/shared.json", Strategy: "copy", Ownership: "json-subset", OwnedContent: previous,
	}}}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	m := manifest.Manifest{
		Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{Source: "configs/shared.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"core"}}},
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	p, err := plan.Build(m, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: meta})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Actions) != 1 || p.Actions[0].Status != plan.StatusUpdate {
		t.Fatalf("actions = %+v, want reversible update", p.Actions)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	for _, want := range []string{`"added":"new"`, `"external":"preserve"`, `"targetOnly":true`, `"external"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("reconciled target missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), `"retired"`) || strings.Contains(string(got), `"old"`) {
		t.Fatalf("reconciled target retained retired contribution:\n%s", got)
	}
	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("reload metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Ownership != "json-subset" || !json.Valid(rec.OwnedContent) {
		t.Fatalf("metadata record = %+v, want valid owned JSON evidence", rec)
	}
	if meta.Version != state.CurrentVersion {
		t.Fatalf("metadata version = %d, want %d", meta.Version, state.CurrentVersion)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil || len(backupMeta.Sets) != 1 {
		t.Fatalf("Backup Sets = %+v, err = %v; want one", backupMeta.Sets, err)
	}
}

func TestApplyStatusUpdateMergesTOMLSubsetAndRecordsMetadata(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "codex", "config-codegraph.toml")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte(`sandbox_mode = "danger-full-access"
approval_policy = "never"

[[hooks.SessionStart]]
matcher = "startup|resume"

[[hooks.SessionStart.hooks]]
type = "command"
command = "codegraph init"
timeout = 120
`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`model = "gpt-5.5"
sandbox_mode = "danger-full-access"
approval_policy = "never"

[tui]
theme = "catppuccin-mocha"
`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:    "configs/codex/config-codegraph.toml",
		Target:    target,
		Strategy:  "copy",
		Status:    plan.StatusUpdate,
		Ownership: "toml-subset",
	}}}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	for _, want := range []string{`model = "gpt-5.5"`, `[tui]`, `command = "codegraph init"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("target missing %q\ncontent:\n%s", want, got)
		}
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatalf("metadata missing target %s", target)
	}
	if rec.Source != "configs/codex/config-codegraph.toml" {
		t.Fatalf("metadata source = %q, want CodeGraph source", rec.Source)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(backupMeta.Sets) != 1 {
		t.Fatalf("backup sets = %d, want 1", len(backupMeta.Sets))
	}
}

func TestApplyStatusUpdateRejectsSymlinkTargetBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "codex", "config-codegraph.toml")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte(`[[hooks.SessionStart]]
matcher = "startup|resume"
`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.toml")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	target := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatalf("symlink target: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:    "configs/codex/config-codegraph.toml",
		Target:    target,
		Strategy:  "copy",
		Status:    plan.StatusUpdate,
		Ownership: "toml-subset",
	}}}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil {
		t.Fatal("Apply() error = nil, want symlink update target rejection")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside target: %v", err)
	}
	if string(got) != "outside\n" {
		t.Fatalf("outside target changed to %q", got)
	}
}

func TestApplyRejectsMissingSourceWithoutMutatingAnyAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	wouldCreate := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: wouldCreate, Strategy: "symlink", Status: plan.StatusCreate},
		{Source: "configs/missing", Target: filepath.Join(home, ".missing"), Strategy: "copy", Status: plan.StatusMissingSource},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want missing-source error")
	}
	if _, err := os.Lstat(wouldCreate); !os.IsNotExist(err) {
		t.Fatalf("would-create target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsMismatchedContributionAttributionBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "safe")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte("safe\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".config", "safe")
	p := plan.Plan{Actions: []plan.Action{{
		Source:        "configs/safe",
		Contributions: []plan.Contribution{{Source: "configs/other", SelectorTags: []string{"safe"}}},
		Target:        target,
		Strategy:      "copy",
		Status:        plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil || !strings.Contains(err.Error(), "contribution source") {
		t.Fatalf("Apply() error = %v, want contribution source mismatch", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("mismatched attribution mutated target: %v", err)
	}
	if _, err := os.Lstat(state.Path(stateRoot)); !os.IsNotExist(err) {
		t.Fatalf("mismatched attribution wrote metadata: %v", err)
	}
}

func TestApplyRejectsUnsafeTargetWithoutMutatingAnyAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")

	sourcePath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	wouldCreate := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: wouldCreate, Strategy: "symlink", Status: plan.StatusCreate},
		{Source: "configs/zsh/zshrc", Target: outside, Strategy: "symlink", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want unsafe target error")
	}
	if _, err := os.Lstat(wouldCreate); !os.IsNotExist(err) {
		t.Fatalf("would-create target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsUnsafeSourceWithoutCopyingOutsideFile(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	outsidePath := filepath.Join(filepath.Dir(sourceRoot), "outside")
	if err := os.WriteFile(outsidePath, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("write outside source: %v", err)
	}

	target := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "../outside",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want unsafe source error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsTargetParentSymlinkEscape(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outsideHome := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.Symlink(outsideHome, filepath.Join(home, ".config")); err != nil {
		t.Fatalf("symlink escaped parent: %v", err)
	}

	target := filepath.Join(home, ".config", "zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/zsh/zshrc",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want target parent symlink escape error")
	}
	if _, err := os.Lstat(filepath.Join(outsideHome, "zshrc")); !os.IsNotExist(err) {
		t.Fatalf("outside-home target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsDuplicateCreateTargetsBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	firstSource := filepath.Join(sourceRoot, "configs", "first")
	if err := os.MkdirAll(filepath.Dir(firstSource), 0o755); err != nil {
		t.Fatalf("mkdir first source: %v", err)
	}
	if err := os.WriteFile(firstSource, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("write first source: %v", err)
	}
	secondSource := filepath.Join(sourceRoot, "configs", "second")
	if err := os.WriteFile(secondSource, []byte("second\n"), 0o600); err != nil {
		t.Fatalf("write second source: %v", err)
	}

	target := filepath.Join(home, ".dupe")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/first", Target: target, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "configs/second", Target: target, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil {
		t.Fatal("Apply() error = nil, want duplicate target error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("duplicate target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(state.Path(stateRoot)); !os.IsNotExist(err) {
		t.Fatalf("metadata exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsDuplicateCreateTargetsAfterNormalizingTargetPathBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	firstSource := filepath.Join(sourceRoot, "configs", "first")
	if err := os.MkdirAll(filepath.Dir(firstSource), 0o755); err != nil {
		t.Fatalf("mkdir first source: %v", err)
	}
	if err := os.WriteFile(firstSource, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("write first source: %v", err)
	}
	secondSource := filepath.Join(sourceRoot, "configs", "second")
	if err := os.WriteFile(secondSource, []byte("second\n"), 0o600); err != nil {
		t.Fatalf("write second source: %v", err)
	}

	target := filepath.Join(home, ".dupe")
	lexicalVariant := filepath.Join(home, "nested") + string(filepath.Separator) + ".." + string(filepath.Separator) + ".dupe"
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/first", Target: target, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "configs/second", Target: lexicalVariant, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want duplicate target error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("duplicate target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsSourceSymlinkEscapeWithoutMutatingAnyAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outsideRoot := t.TempDir()

	safeSource := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(safeSource), 0o755); err != nil {
		t.Fatalf("mkdir safe source: %v", err)
	}
	if err := os.WriteFile(safeSource, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write safe source: %v", err)
	}

	outsideSecret := filepath.Join(outsideRoot, "secret")
	if err := os.WriteFile(outsideSecret, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("write outside secret: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	if err := os.Symlink(outsideSecret, filepath.Join(sourceRoot, "configs", "link")); err != nil {
		t.Fatalf("symlink escaped source: %v", err)
	}

	wouldCreate := filepath.Join(home, ".zshrc")
	escapedTarget := filepath.Join(home, ".secret")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: wouldCreate, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "configs/link", Target: escapedTarget, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want source symlink escape error")
	}
	if _, err := os.Lstat(wouldCreate); !os.IsNotExist(err) {
		t.Fatalf("earlier safe target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(escapedTarget); !os.IsNotExist(err) {
		t.Fatalf("escaped source target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyStatusCreateCopyDoesNotOverwriteExistingTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("user-owned\n"), 0o640); err != nil {
		t.Fatalf("write existing target: %v", err)
	}
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/git/gitconfig",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want stale create error")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read existing target: %v", err)
	}
	if string(got) != "user-owned\n" {
		t.Fatalf("target contents = %q, want original user-owned contents", got)
	}
}

func TestApplyRecordsInstallationMetadataForCreatedTargets(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	symlinkSrc := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(symlinkSrc), 0o755); err != nil {
		t.Fatalf("mkdir symlink source: %v", err)
	}
	if err := os.WriteFile(symlinkSrc, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write symlink source: %v", err)
	}
	copySrc := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(copySrc), 0o755); err != nil {
		t.Fatalf("mkdir copy source: %v", err)
	}
	if err := os.WriteFile(copySrc, []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write copy source: %v", err)
	}

	symlinkTarget := filepath.Join(home, ".zshrc")
	copyTarget := filepath.Join(home, ".gitconfig")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: symlinkTarget, Strategy: "symlink", Status: plan.StatusCreate},
		{Source: "configs/git/gitconfig", Target: copyTarget, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(meta.Entries) != 2 {
		t.Fatalf("metadata entries = %d, want 2\n%+v", len(meta.Entries), meta.Entries)
	}

	rec, ok := meta.FindByTarget(copyTarget)
	if !ok {
		t.Fatalf("metadata missing copy target %s", copyTarget)
	}
	wantHash, err := state.HashFile(copySrc)
	if err != nil {
		t.Fatalf("hash copy source: %v", err)
	}
	if rec.Hash != wantHash {
		t.Fatalf("copy record hash = %q, want %q", rec.Hash, wantHash)
	}
	if rec.Strategy != "copy" || rec.Source != "configs/git/gitconfig" {
		t.Fatalf("copy record = %+v, want strategy copy / source configs/git/gitconfig", rec)
	}
	if rec.InstalledAt == "" {
		t.Fatalf("copy record InstalledAt is empty")
	}

	symlinkRec, ok := meta.FindByTarget(symlinkTarget)
	if !ok {
		t.Fatalf("metadata missing symlink target %s", symlinkTarget)
	}
	if symlinkRec.Hash != "" {
		t.Fatalf("symlink record hash = %q, want empty", symlinkRec.Hash)
	}
}

func TestApplyRejectsStateRootSymlinkEscapeBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outsideState := t.TempDir()

	src := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stateParent := filepath.Join(home, ".local", "state")
	if err := os.MkdirAll(stateParent, 0o755); err != nil {
		t.Fatalf("mkdir state parent: %v", err)
	}
	stateRoot := filepath.Join(stateParent, "dots")
	if err := os.Symlink(outsideState, stateRoot); err != nil {
		t.Fatalf("symlink state root: %v", err)
	}

	target := filepath.Join(home, ".gitconfig")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/git/gitconfig", Target: target, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil {
		t.Fatal("Apply() error = nil, want state root symlink escape error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(outsideState, "installed.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata was written outside home through state symlink; lstat err = %v", err)
	}
}

func TestApplyRejectsMetadataLeafSymlinkEscapeBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outsideState := t.TempDir()

	src := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stateRoot := filepath.Join(home, ".local", "state", "dots")
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir state root: %v", err)
	}
	outsideMetadata := filepath.Join(outsideState, "installed.json")
	if err := os.Symlink(outsideMetadata, filepath.Join(stateRoot, "installed.json")); err != nil {
		t.Fatalf("symlink metadata leaf: %v", err)
	}

	target := filepath.Join(home, ".gitconfig")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/git/gitconfig", Target: target, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil {
		t.Fatal("Apply() error = nil, want metadata leaf symlink escape error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(outsideMetadata); !os.IsNotExist(err) {
		t.Fatalf("metadata was written outside home through leaf symlink; lstat err = %v", err)
	}
}

func TestApplyWithoutStateRootWritesNoMetadata(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: target, Strategy: "symlink", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("expected target created even without state root: %v", err)
	}
}

func TestApplyMergesMetadataAcrossRunsWithoutDuplicating(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	src := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".gitconfig")
	create := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/git/gitconfig", Target: target, Strategy: "copy", Status: plan.StatusCreate},
	}}
	if err := install.Apply(create, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}

	// Re-running with the same target already in place yields an Unchanged
	// action; metadata must refresh in place rather than appending a duplicate.
	unchanged := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/git/gitconfig", Target: target, Strategy: "copy", Status: plan.StatusUnchanged},
	}}
	if err := install.Apply(unchanged, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(meta.Entries) != 1 {
		t.Fatalf("metadata entries = %d, want 1 (no duplicate)\n%+v", len(meta.Entries), meta.Entries)
	}
}

func TestApplyStatusCreateSymlinkRejectsMissingSource(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	target := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/zsh/zshrc",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want missing source error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("dangling symlink exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyCreatesDirectorySymlinkForDirectorySource(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	// Create a source directory with content (simulating configs/nvim/).
	sourceDirPath := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(filepath.Join(sourceDirPath, "lua"), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDirPath, "init.lua"), []byte("-- init\n"), 0o600); err != nil {
		t.Fatalf("write init.lua: %v", err)
	}

	target := filepath.Join(home, ".config", "nvim")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink", info.Mode())
	}
	gotDest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink target: %v", err)
	}
	if gotDest != sourceDirPath {
		t.Fatalf("symlink dest = %q, want %q", gotDest, sourceDirPath)
	}
}

func TestApplyCopyStrategyRejectsDirectorySourceBeforeInstall(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	// Create a source directory (not a file).
	sourceDirPath := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(sourceDirPath, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}

	target := filepath.Join(home, ".config", "nvim")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want error for directory source with copy strategy")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyAdoptConflictRejectsDirectoryTargetWithActionableError(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourceDir := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	target := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusConflict,
	}}}

	err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionAdopt,
		},
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want directory adopt error")
	}
	if !strings.Contains(err.Error(), "Adopting directory target") && !strings.Contains(err.Error(), "adopting directory target") {
		t.Fatalf("error %q does not explain directory adopt is unsupported", err)
	}
	if !strings.Contains(err.Error(), "replace") {
		t.Fatalf("error %q does not suggest replace", err)
	}
}
