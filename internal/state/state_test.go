package state_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yersonargotev/dots/internal/state"
)

func TestLoadReturnsEmptyMetadataWhenFileAbsent(t *testing.T) {
	stateRoot := t.TempDir()

	got, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for absent file", err)
	}
	if len(got.Entries) != 0 {
		t.Fatalf("Load() entries = %d, want 0 for absent file", len(got.Entries))
	}
}

func TestLockedMetadataSerializesReadModifyWrite(t *testing.T) {
	path := state.Path(t.TempDir())
	if err := state.Save(path, state.Metadata{Version: state.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	locked, err := state.LockMetadata(path)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	done := make(chan error, 1)
	go func() {
		defer wg.Done()
		done <- state.Update(path, func(meta *state.Metadata) error {
			meta.Provisioners = append(meta.Provisioners, state.ProvisionerRecord{Tool: "serialized"})
			return nil
		})
	}()
	select {
	case err := <-done:
		t.Fatalf("Update() completed before lock release: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, err := state.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Provisioners) != 1 || got.Provisioners[0].Tool != "serialized" {
		t.Fatalf("serialized metadata = %#v", got)
	}
}

func TestLockMetadataRejectsSymlinkLockFile(t *testing.T) {
	stateRoot := t.TempDir()
	path := state.Path(stateRoot)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path+".lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.LockMetadata(path); err == nil {
		t.Fatal("LockMetadata() error = nil, want symlink rejection")
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "unchanged" {
		t.Fatalf("outside lock target = %q, %v", got, err)
	}
}

func TestRecordCloneDoesNotAliasEvidence(t *testing.T) {
	original := state.Record{
		Sources:      []string{"source"},
		OwnedContent: []byte("owned"),
		Contributions: []state.Contribution{{
			SelectorTags: []string{"tag"}, OwnedBytes: []byte("bytes"),
		}},
	}
	cloned := original.Clone()
	cloned.Sources[0] = "changed"
	cloned.OwnedContent[0] = 'X'
	cloned.Contributions[0].SelectorTags[0] = "changed"
	cloned.Contributions[0].OwnedBytes[0] = 'X'
	if original.Sources[0] != "source" || string(original.OwnedContent) != "owned" || original.Contributions[0].SelectorTags[0] != "tag" || string(original.Contributions[0].OwnedBytes) != "bytes" {
		t.Fatalf("Clone() aliases original evidence: %#v", original)
	}
}

func TestSaveThenLoadRoundTripsRecords(t *testing.T) {
	stateRoot := t.TempDir()
	path := state.Path(stateRoot)

	want := state.Metadata{
		Version:    2,
		Provenance: state.Provenance{SourceRoot: "/src/dots", SourceRevision: "abc123", DotsVersion: "v0.test", RecordedAt: "2026-06-06T00:00:00Z"},
		Entries: []state.Record{{
			Target:      "/home/user/.zshrc",
			Source:      "configs/zsh/zshrc",
			Strategy:    "symlink",
			Hash:        "abc123",
			InstalledAt: "2026-06-06T00:00:00Z",
			Profiles:    []string{"core"},
			Tags:        []string{"core"},
		}},
		Provisioners: []state.ProvisionerRecord{{
			Profile:    "core",
			Profiles:   []string{"core"},
			Tags:       []string{"core"},
			Tool:       "claude",
			Executable: "claude",
			Args:       []string{"install"},
			Status:     "provisioned",
			LastRunAt:  "2026-06-06T00:00:00Z",
		}},
	}

	if err := state.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("Load() entries = %d, want 1", len(got.Entries))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() metadata = %+v, want %+v", got, want)
	}
}

func TestSaveThenLoadRoundTripsInstalledSelectionV3(t *testing.T) {
	path := state.Path(t.TempDir())
	want := state.Metadata{
		Version: 3,
		InstalledSelection: &state.InstalledSelection{
			Profiles:     []string{"core", "agents"},
			ExtraTags:    []string{"agents", "web"},
			ResolvedTags: []string{"core", "agents", "web"},
			Provenance: state.Provenance{
				SourceRoot:     "/src/dots",
				SourceRevision: "abc123",
				DotsVersion:    "v0.test",
				RecordedAt:     "2026-07-23T12:00:00Z",
			},
		},
		Entries: []state.Record{{Target: "/home/user/.zshrc"}},
	}

	if err := state.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() metadata = %+v, want %+v", got, want)
	}
}

func TestSaveThenLoadRoundTripsOwnedJSONContributionV4(t *testing.T) {
	path := state.Path(t.TempDir())
	want := state.Metadata{
		Version: state.CurrentVersion,
		Entries: []state.Record{{
			Target:       "/home/user/.config/shared.json",
			Source:       "configs/shared.json",
			Strategy:     "copy",
			Ownership:    "json-subset",
			OwnedContent: []byte(`{"owned":true}`),
		}},
	}

	if err := state.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Version != want.Version || len(got.Entries) != 1 || got.Entries[0].Ownership != "json-subset" {
		t.Fatalf("Load() metadata = %+v, want v4 owned JSON record", got)
	}
	var gotOwned, wantOwned any
	if err := json.Unmarshal(got.Entries[0].OwnedContent, &gotOwned); err != nil {
		t.Fatalf("decode loaded owned content: %v", err)
	}
	if err := json.Unmarshal(want.Entries[0].OwnedContent, &wantOwned); err != nil {
		t.Fatalf("decode wanted owned content: %v", err)
	}
	if !reflect.DeepEqual(gotOwned, wantOwned) {
		t.Fatalf("owned content = %v, want %v", gotOwned, wantOwned)
	}
}

func TestSaveThenLoadRoundTripsOpaqueSeededBaselineV5(t *testing.T) {
	path := state.Path(t.TempDir())
	want := state.Metadata{
		Version: state.CurrentVersion,
		Entries: []state.Record{{
			Target:         "/home/user/.local/state/app/runtime.lock",
			Source:         "configs/app/runtime.lock",
			Strategy:       "copy",
			Ownership:      "seeded",
			SeededBaseline: []byte{0xff, 0x00, 'a', '\n'},
		}},
	}
	if err := state.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() metadata = %+v, want %+v", got, want)
	}
}

func TestSaveThenLoadRoundTripsOrderedContributionOwnershipV7(t *testing.T) {
	path := state.Path(t.TempDir())
	want := state.Metadata{
		Version: state.CurrentVersion,
		Entries: []state.Record{{
			Target:       "/home/user/.config/shared.json",
			Source:       "configs/opencode/settings.json",
			Sources:      []string{"configs/opencode/settings.json", "configs/antigravity/settings.json"},
			Strategy:     "copy",
			Ownership:    "json-subset",
			OwnedContent: []byte(`{"agents":{"antigravity":true,"opencode":true}}`),
			Contributions: []state.Contribution{
				{
					Source:           "configs/opencode/settings.json",
					SelectorTags:     []string{"opencode"},
					Ownership:        "json-subset",
					EvidenceRecorded: true,
					Hash:             "opencode-hash",
					OwnedContent:     []byte(`{"agents":{"opencode":true}}`),
				},
				{
					Source:           "configs/antigravity/settings.json",
					SelectorTags:     []string{"antigravity"},
					Ownership:        "json-subset",
					EvidenceRecorded: true,
					Hash:             "antigravity-hash",
					OwnedContent:     []byte(`{"agents":{"antigravity":true}}`),
				},
			},
		}},
	}

	if err := state.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Version != want.Version || len(got.Entries) != 1 {
		t.Fatalf("Load() metadata = %+v, want one v%d record", got, want.Version)
	}
	gotRecord, wantRecord := got.Entries[0], want.Entries[0]
	if gotRecord.Source != wantRecord.Source || !reflect.DeepEqual(gotRecord.Sources, wantRecord.Sources) || len(gotRecord.Contributions) != len(wantRecord.Contributions) {
		t.Fatalf("Load() record = %+v, want ordered sources and contributions %+v", gotRecord, wantRecord)
	}
	for i := range wantRecord.Contributions {
		gotContribution, wantContribution := gotRecord.Contributions[i], wantRecord.Contributions[i]
		if gotContribution.Source != wantContribution.Source ||
			!reflect.DeepEqual(gotContribution.SelectorTags, wantContribution.SelectorTags) ||
			gotContribution.Ownership != wantContribution.Ownership ||
			gotContribution.EvidenceRecorded != wantContribution.EvidenceRecorded ||
			gotContribution.Hash != wantContribution.Hash {
			t.Fatalf("Load() contribution[%d] = %+v, want %+v", i, gotContribution, wantContribution)
		}
		var gotOwned, wantOwned any
		if err := json.Unmarshal(gotContribution.OwnedContent, &gotOwned); err != nil {
			t.Fatalf("decode loaded contribution[%d]: %v", i, err)
		}
		if err := json.Unmarshal(wantContribution.OwnedContent, &wantOwned); err != nil {
			t.Fatalf("decode wanted contribution[%d]: %v", i, err)
		}
		if !reflect.DeepEqual(gotOwned, wantOwned) {
			t.Fatalf("Load() contribution[%d] owned content = %v, want %v", i, gotOwned, wantOwned)
		}
	}
}

func TestLoadLegacyRecordDoesNotGainPartialOwnershipEvidence(t *testing.T) {
	path := state.Path(t.TempDir())
	data := `{"version":3,"entries":[{"target":"/home/user/.config/shared.json","source":"configs/shared.json","strategy":"copy","hash":"abc","installedAt":"now"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write legacy metadata: %v", err)
	}
	got, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Ownership != "" || len(got.Entries[0].OwnedContent) != 0 {
		t.Fatalf("Load() legacy record = %+v, want no partial ownership evidence", got.Entries)
	}
	if len(got.Entries[0].Contributions) != 0 {
		t.Fatalf("Load() legacy contributions = %+v, want none synthesized", got.Entries[0].Contributions)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read legacy metadata after Load(): %v", err)
	}
	if string(unchanged) != data {
		t.Fatalf("Load() rewrote legacy metadata = %q, want %q", unchanged, data)
	}
}

func TestLoadPreviousMetadataVersionsDoesNotRewriteOrAttribute(t *testing.T) {
	for version := 1; version < state.CurrentVersion; version++ {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			path := state.Path(t.TempDir())
			data := []byte(fmt.Sprintf(`{"version":%d,"entries":[{"target":"/home/user/.config/app","source":"configs/app","strategy":"copy","hash":"abc","installedAt":"now"}]}`, version))
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatalf("write previous metadata: %v", err)
			}
			got, err := state.Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.Version != version || len(got.Entries) != 1 || len(got.Entries[0].Contributions) != 0 {
				t.Fatalf("Load() metadata = %+v, want unchanged v%d record without attribution", got, version)
			}
			unchanged, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read previous metadata after Load(): %v", err)
			}
			if !reflect.DeepEqual(unchanged, data) {
				t.Fatalf("Load() rewrote v%d metadata = %q, want %q", version, unchanged, data)
			}
		})
	}
}

func TestPendingReconciliationMatchesOnlyExactTargetAndOrderedSources(t *testing.T) {
	target := []byte("target bytes\n")
	first := []byte("first source\n")
	second := []byte("second source\n")
	record := state.Record{PendingReconciliation: &state.ReconciliationReceipt{
		TargetHash:   state.HashBytes(target),
		Sources:      []string{"first", "second"},
		SourceHashes: []string{state.HashBytes(first), state.HashBytes(second)},
		Strategy:     "copy",
		Ownership:    "json-subset",
	}}
	if !record.PendingReconciliationMatches(target, "copy", "json-subset", []string{"first", "second"}, [][]byte{first, second}) {
		t.Fatal("exact reconciliation receipt did not match")
	}
	tests := []struct {
		name      string
		target    []byte
		strategy  string
		ownership string
		sources   []string
		contents  [][]byte
	}{
		{name: "target drift", target: []byte("changed\n"), strategy: "copy", ownership: "json-subset", sources: []string{"first", "second"}, contents: [][]byte{first, second}},
		{name: "source drift", target: target, strategy: "copy", ownership: "json-subset", sources: []string{"first", "second"}, contents: [][]byte{first, []byte("changed\n")}},
		{name: "source order", target: target, strategy: "copy", ownership: "json-subset", sources: []string{"second", "first"}, contents: [][]byte{second, first}},
		{name: "strategy", target: target, strategy: "symlink", ownership: "json-subset", sources: []string{"first", "second"}, contents: [][]byte{first, second}},
		{name: "ownership", target: target, strategy: "copy", ownership: "whole", sources: []string{"first", "second"}, contents: [][]byte{first, second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if record.PendingReconciliationMatches(tt.target, tt.strategy, tt.ownership, tt.sources, tt.contents) {
				t.Fatal("mismatched reconciliation receipt was accepted")
			}
		})
	}
}

func TestLoadLegacyMetadataHasNoInstalledSelection(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			path := state.Path(t.TempDir())
			data := fmt.Sprintf("{\"version\":%d,\"entries\":[]}\n", version)
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatalf("write legacy metadata: %v", err)
			}
			got, err := state.Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.InstalledSelection != nil {
				t.Fatalf("Load() selection = %+v, want nil", got.InstalledSelection)
			}
		})
	}
}

func TestSaveFailurePreservesPreviousMetadata(t *testing.T) {
	dir := t.TempDir()
	path := state.Path(dir)
	previous := []byte("{\"version\":2,\"entries\":[]}\n")
	if err := os.WriteFile(path, previous, 0o600); err != nil {
		t.Fatalf("write previous metadata: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("make state directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := state.Save(path, state.Metadata{Version: 3}); err == nil {
		t.Skip("filesystem permits writes to a read-only directory")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read previous metadata: %v", err)
	}
	if !reflect.DeepEqual(got, previous) {
		t.Fatalf("metadata after failed Save() = %q, want previous %q", got, previous)
	}
}

func TestRemovePreservesIndependentInstalledSelection(t *testing.T) {
	original := state.Metadata{
		Entries: []state.Record{{Target: "/drop"}, {Target: "/keep"}},
		InstalledSelection: &state.InstalledSelection{
			Profiles:     []string{"core"},
			ExtraTags:    []string{"web"},
			ResolvedTags: []string{"core", "web"},
		},
	}

	pruned := original.Remove("/drop")
	if pruned.InstalledSelection == nil || !reflect.DeepEqual(pruned.InstalledSelection, original.InstalledSelection) {
		t.Fatalf("Remove() selection = %+v, want %+v", pruned.InstalledSelection, original.InstalledSelection)
	}
	pruned.InstalledSelection.Profiles[0] = "changed"
	if original.InstalledSelection.Profiles[0] != "core" {
		t.Fatal("Remove() selection aliases the original metadata")
	}
}

func TestFindByTargetReturnsRecordWhenPresent(t *testing.T) {
	meta := state.Metadata{Entries: []state.Record{
		{Target: "/home/user/.zshrc", Strategy: "symlink"},
		{Target: "/home/user/.gitconfig", Strategy: "copy"},
	}}

	got, ok := meta.FindByTarget("/home/user/.gitconfig")
	if !ok {
		t.Fatal("FindByTarget() ok = false, want true for present target")
	}
	if got.Strategy != "copy" {
		t.Fatalf("FindByTarget() strategy = %q, want copy", got.Strategy)
	}

	if _, ok := meta.FindByTarget("/home/user/.missing"); ok {
		t.Fatal("FindByTarget() ok = true, want false for absent target")
	}
}

func TestRecordSourceListPreservesLegacyAndCompositeContributors(t *testing.T) {
	legacy := state.Record{Source: "configs/base.json"}
	if got := legacy.SourceList(); !reflect.DeepEqual(got, []string{"configs/base.json"}) {
		t.Fatalf("legacy SourceList() = %#v, want singular source", got)
	}
	if !legacy.HasSource("configs/base.json") {
		t.Fatal("legacy HasSource() = false, want singular source match")
	}

	composite := state.Record{
		Source:  "configs/base.json",
		Sources: []string{"configs/base.json", "configs/mobile.json"},
	}
	if got := composite.SourceList(); !reflect.DeepEqual(got, composite.Sources) {
		t.Fatalf("composite SourceList() = %#v, want %#v", got, composite.Sources)
	}
	if !composite.HasSource("configs/mobile.json") {
		t.Fatal("composite HasSource() = false, want contributor match")
	}
	meta := state.Metadata{Entries: []state.Record{{
		Target: "/home/user/.config/shared.json", Source: composite.Source, Sources: composite.Sources, Strategy: "copy",
	}}}
	if !meta.MatchesEntry("/home/user/.config/shared.json", "configs/mobile.json", "copy") {
		t.Fatal("MatchesEntry() = false, want composite contributor ownership proof")
	}
	if meta.MatchesEntry("/home/user/.config/shared.json", "configs/mobile.json", "symlink") {
		t.Fatal("MatchesEntry() = true for wrong strategy")
	}
}

func TestHashFileIsStableAndContentSensitive(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte("same\n"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, []byte("same\n"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	ha, err := state.HashFile(a)
	if err != nil {
		t.Fatalf("HashFile(a) error = %v", err)
	}
	hb, err := state.HashFile(b)
	if err != nil {
		t.Fatalf("HashFile(b) error = %v", err)
	}
	if ha != hb {
		t.Fatalf("HashFile equal content mismatch: %q vs %q", ha, hb)
	}

	if err := os.WriteFile(b, []byte("different\n"), 0o600); err != nil {
		t.Fatalf("rewrite b: %v", err)
	}
	hb2, err := state.HashFile(b)
	if err != nil {
		t.Fatalf("HashFile(b) error = %v", err)
	}
	if ha == hb2 {
		t.Fatal("HashFile() returned same hash for different content")
	}
}

func TestHashFileRejectsDirectories(t *testing.T) {
	dir := t.TempDir()

	_, err := state.HashFile(dir)
	if err == nil {
		t.Fatal("HashFile() error = nil, want directory rejection")
	}
	if !strings.Contains(err.Error(), "directories are not supported") {
		t.Fatalf("HashFile() error = %q, want directory rejection", err)
	}
}
