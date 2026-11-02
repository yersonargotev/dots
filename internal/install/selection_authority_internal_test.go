package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func TestForwardMutationConsumesExactSelectionAuthority(t *testing.T) {
	for _, kind := range []string{"shared reconciliation", "symlink switch"} {
		for _, changed := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: " current", true: " concurrent"}[changed], func(t *testing.T) {
				home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
				target := filepath.Join(home, ".managed")
				source := filepath.Join(sourceRoot, "next")
				previous := []byte(`{"old":true}`)
				if err := os.WriteFile(source, []byte(`{"new":true}`), 0600); err != nil {
					t.Fatal(err)
				}
				expected := &state.InstalledSelection{Profiles: []string{"workstation"}, ResolvedTags: []string{"base", "official"}}
				var record state.Record
				var action plan.Action
				if kind == "shared reconciliation" {
					if err := os.WriteFile(target, previous, 0600); err != nil {
						t.Fatal(err)
					}
					record = recoveryAuthorityRecord(target, "next", previous)
					fp, err := state.RecordEvidenceFingerprint(record)
					if err != nil {
						t.Fatal(err)
					}
					action = recoveryAuthorityAction(target, "next", previous, fp)
				} else {
					oldSource := filepath.Join(sourceRoot, "old")
					if err := os.WriteFile(oldSource, []byte("old"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(oldSource, target); err != nil {
						t.Fatal(err)
					}
					record = state.Record{Target: target, Source: "old", Strategy: "symlink", Hash: state.HashBytes([]byte("old"))}
					action = plan.Action{Source: "next", Target: target, Strategy: "symlink", Status: plan.StatusConflict, Contributions: []plan.Contribution{{Source: "next"}}}
				}
				current := *expected
				if changed {
					current.ExcludedTags = []string{"official"}
				}
				if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{record}, InstalledSelection: &current}); err != nil {
					t.Fatal(err)
				}
				_, err := ApplyManagedEntries(plan.Plan{Actions: []plan.Action{action}}, Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot, CheckSelection: true, ExpectedSelection: expected, ConflictDecisions: map[string]ConflictDecision{target: DecisionReplace}})
				if changed {
					if err == nil || !strings.Contains(err.Error(), "selection changed concurrently") {
						t.Fatalf("stale authority error=%v", err)
					}
					if kind == "shared reconciliation" {
						data, err := os.ReadFile(target)
						if err != nil || string(data) != string(previous) {
							t.Fatalf("stale subtraction mutated target: %s %v", data, err)
						}
					} else {
						dest, err := os.Readlink(target)
						if err != nil || dest != filepath.Join(sourceRoot, "old") {
							t.Fatalf("stale source switch: %s %v", dest, err)
						}
					}
				} else if err != nil {
					t.Fatalf("current authority rejected: %v", err)
				}
			})
		}
	}
}
