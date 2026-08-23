package deps_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
)

func TestInstallPreparedUsesExactProviderSelectedDuringPrepare(t *testing.T) {
	present := map[string]bool{"tmux": true, "brew": true}
	look := func(command string) bool { return present[command] }
	prepared, err := deps.PrepareInstall(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, look, fontLookupSet(), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("PrepareInstall() error = %v", err)
	}

	// Provider availability changes after review. The accepted action remains
	// the Homebrew action and must not be resolved again to the newly available
	// Debian provider.
	present["brew"] = false
	present["sudo"] = true
	present["apt-get"] = true
	runner := &recordingRunner{afterRun: func() { present["starship"] = true }}
	report, err := deps.InstallPrepared(prepared, deps.Options{Profile: "default", OS: "darwin"}, look, fontLookupSet(), runner)
	if err != nil {
		t.Fatalf("InstallPrepared() error = %v", err)
	}

	wantCalls := []runnerCall{{executable: "brew", args: []string{"install", "starship"}}}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner calls = %#v, want exact prepared calls %#v", runner.calls, wantCalls)
	}
	if len(report.Items) != 1 || report.Items[0].Provider != deps.TierHomebrew || report.Items[0].Status != deps.InstallStatusInstalled {
		t.Fatalf("report items = %#v, want prepared Homebrew action installed", report.Items)
	}
}

func TestInstallPreparedSkipsActionThatBecamePresent(t *testing.T) {
	present := map[string]bool{"tmux": true, "brew": true}
	look := func(command string) bool { return present[command] }
	prepared, err := deps.PrepareInstall(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, look, fontLookupSet(), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("PrepareInstall() error = %v", err)
	}

	present["starship"] = true
	runner := &recordingRunner{}
	report, err := deps.InstallPrepared(prepared, deps.Options{Profile: "default", OS: "darwin"}, look, fontLookupSet(), runner)
	if err != nil {
		t.Fatalf("InstallPrepared() error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %#v, want no calls for dependency that became present", runner.calls)
	}
	if len(report.Items) != 0 {
		t.Fatalf("report items = %#v, want skipped action omitted", report.Items)
	}
}
