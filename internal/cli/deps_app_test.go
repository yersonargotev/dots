package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAppDirectoriesUseSelectedHomeAndSystemApplicationsOnDarwin(t *testing.T) {
	home := t.TempDir()
	want := []string{filepath.Join(home, "Applications"), "/Applications"}
	if got := appDirectories("darwin", home); !reflect.DeepEqual(got, want) {
		t.Fatalf("appDirectories() = %#v, want %#v", got, want)
	}
	if got := appDirectories("linux", home); len(got) != 0 {
		t.Fatalf("Linux appDirectories() = %#v, want none", got)
	}
}

func TestAppInstalledInSandboxDirectories(t *testing.T) {
	applications := filepath.Join(t.TempDir(), "Applications")
	if err := os.MkdirAll(filepath.Join(applications, "Ghostty.app"), 0o755); err != nil {
		t.Fatalf("create sandbox Ghostty app: %v", err)
	}
	lookup := appInstalledIn([]string{applications})

	if !lookup("Ghostty.app") {
		t.Fatal("Ghostty.app = missing, want present in sandbox Applications")
	}
	if lookup("Missing.app") {
		t.Fatal("Missing.app = present, want missing in sandbox Applications")
	}
}
