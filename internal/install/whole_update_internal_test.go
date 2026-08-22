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
