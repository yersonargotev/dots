package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func TestUpdateWholeTargetRejectsLastMomentDrift(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".config")
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(target, []byte("changed\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	action := plan.Action{Target: target, PreviousHash: state.HashBytes([]byte("previous\n"))}

	err := updateWholeTarget(action, source, home)
	if err == nil || !strings.Contains(err.Error(), "changed before update") {
		t.Fatalf("updateWholeTarget() error = %v, want stale target rejection", err)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "changed\n" {
		t.Fatalf("drifted target = %q, %v; want unchanged", got, readErr)
	}
}

func TestUpdateWholeTargetRejectsSymlinkReplacementOutsideHome(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".config")
	outside := filepath.Join(t.TempDir(), "outside")
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(outside, []byte("previous\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}
	action := plan.Action{Target: target, PreviousHash: state.HashBytes([]byte("previous\n"))}

	err := updateWholeTarget(action, source, home)
	if err == nil || !strings.Contains(err.Error(), "non-symlink regular file") {
		t.Fatalf("updateWholeTarget() error = %v, want root confinement rejection", err)
	}
	if got, readErr := os.ReadFile(outside); readErr != nil || string(got) != "previous\n" {
		t.Fatalf("outside file = %q, %v; want unchanged", got, readErr)
	}
}

func TestUpdateWholeTargetRejectsSymlinkReplacementInsideHome(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".config")
	other := filepath.Join(home, ".other")
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(other, []byte("previous\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(other), target); err != nil {
		t.Fatal(err)
	}
	action := plan.Action{Target: target, PreviousHash: state.HashBytes([]byte("previous\n"))}

	err := updateWholeTarget(action, source, home)
	if err == nil || !strings.Contains(err.Error(), "non-symlink regular file") {
		t.Fatalf("updateWholeTarget() error = %v, want in-home symlink rejection", err)
	}
	if got, readErr := os.ReadFile(other); readErr != nil || string(got) != "previous\n" {
		t.Fatalf("in-home destination = %q, %v; want unchanged", got, readErr)
	}
}

func TestApplyPartialUpdateRejectsSymlinkTargets(t *testing.T) {
	markers := "# >>> dots managed >>>\nold\n# <<< dots managed <<<\n"
	tests := []struct {
		name      string
		ownership string
		previous  []byte
		current   []byte
	}{
		{name: "JSON", ownership: "json-subset", previous: []byte(`{"old":true}`), current: []byte(`{"new":true}`)},
		{name: "JSONC", ownership: "jsonc-subset", previous: []byte("{\n  // old\n  \"old\": true\n}\n"), current: []byte("{\n  \"new\": true\n}\n")},
		{name: "TOML", ownership: "toml-subset", previous: []byte("old = true\n"), current: []byte("new = true\n")},
		{name: "marked block", ownership: "marked-block", previous: []byte(markers), current: []byte("new\n")},
		{name: "seeded", ownership: "seeded", previous: []byte("old\n"), current: []byte("new\n")},
	}
	for _, test := range tests {
		for _, destinationScope := range []string{"outside", "inside"} {
			t.Run(test.name+"/"+destinationScope, func(t *testing.T) {
				home := t.TempDir()
				destinationRoot := t.TempDir()
				if destinationScope == "inside" {
					destinationRoot = home
				}
				destination := filepath.Join(destinationRoot, ".destination")
				target := filepath.Join(home, ".target")
				source := filepath.Join(t.TempDir(), "source")
				if err := os.WriteFile(destination, test.previous, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(source, test.current, 0o600); err != nil {
					t.Fatal(err)
				}
				linkDestination := destination
				if destinationScope == "inside" {
					linkDestination = filepath.Base(destination)
				}
				if err := os.Symlink(linkDestination, target); err != nil {
					t.Fatal(err)
				}
				action := plan.Action{
					Target: target, Strategy: "copy", Ownership: test.ownership,
					PreviousContent: test.previous,
				}

				err := applyUpdate(action, source, Options{Home: home, StateRoot: t.TempDir()})
				if err == nil || !strings.Contains(err.Error(), "non-symlink regular file") {
					t.Fatalf("applyUpdate() error = %v, want confined symlink rejection", err)
				}
				got, readErr := os.ReadFile(destination)
				if readErr != nil || string(got) != string(test.previous) {
					t.Fatalf("destination = %q, %v; want unchanged", got, readErr)
				}
			})
		}
	}
}
