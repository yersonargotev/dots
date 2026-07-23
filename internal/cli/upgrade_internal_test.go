package cli

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/upgrade"
)

func TestUpgradeContinuationArgsPreserveUpdateFlags(t *testing.T) {
	binPlan := upgrade.Plan{Channel: "homebrew", CurrentVersion: "v0.18.0", LatestVersion: "v0.19.0", Action: "homebrew-upgrade", Executable: "/bin/dots", Artifact: "dots_v0.19.0_darwin_arm64", Checksum: "abc123"}
	intent := selection.Intent{Source: selection.SourceRecorded, Profiles: []string{"core", "work"}, ExtraTags: []string{"core", "desktop"}}
	got := upgradeContinuationArgs("custom.yaml", true, intent, "/src", "/home", "/state", true, true, true, binPlan)
	want := []string{"dots", "upgrade", "--continue", "--file", "custom.yaml", "--selection-source", "recorded", "--selection-profile", "core", "--selection-profile", "work", "--selection-tag", "core", "--selection-tag", "desktop", "--source-root", "/src", "--home", "/home", "--state-root", "/state", "--yes", "--no-tui", "--output", "json", "--binary-channel", "homebrew", "--binary-current-version", "v0.18.0", "--binary-latest-version", "v0.19.0", "--binary-action", "homebrew-upgrade", "--binary-executable", "/bin/dots", "--binary-artifact", "dots_v0.19.0_darwin_arm64", "--binary-checksum", "abc123"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestUpgradeContinuationArgsDoNotMarkDefaultFileChanged(t *testing.T) {
	got := upgradeContinuationArgs("dots.yaml", false, selection.Intent{Source: selection.SourceExplicit, Profiles: []string{"default"}}, "/src", "/home", "", true, false, false, upgrade.Plan{})
	for i, arg := range got {
		if arg == "--file" || arg == "-f" {
			t.Fatalf("args[%d] = %q; default manifest must not be forwarded as an explicit --file: %#v", i, arg, got)
		}
	}
}
