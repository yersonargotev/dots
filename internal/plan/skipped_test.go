package plan_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
)

func coreEntry() manifest.Entry {
	return manifest.Entry{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"}}
}

func desktopEntry(source string) manifest.Entry {
	return manifest.Entry{Source: source, Target: "~/" + source, Strategy: "symlink", Tags: []string{"desktop"}, OS: []string{"darwin", "linux"}}
}

func nestedProfiles() map[string]manifest.Profile {
	return map[string]manifest.Profile{
		"default": {Tags: []string{"core"}},
		"desktop": {Tags: []string{"core", "desktop"}},
	}
}

func TestSkippedEntries(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: nestedProfiles(),
		Entries:  []manifest.Entry{coreEntry(), desktopEntry("configs/ghostty/config.ghostty"), desktopEntry("configs/zed/settings.json")},
	}

	tests := []struct {
		name          string
		opts          plan.Options
		wantOK        bool
		wantCount     int
		wantSuggested string
	}{
		{
			name:          "default profile reports the desktop-only entries it skips",
			opts:          plan.Options{Profile: "default", OS: "darwin"},
			wantOK:        true,
			wantCount:     2,
			wantSuggested: "desktop",
		},
		{
			name:   "desktop profile selects everything so there is no hint",
			opts:   plan.Options{Profile: "desktop", OS: "darwin"},
			wantOK: false,
		},
		{
			name:   "no hint when the skipped entries are excluded on this OS",
			opts:   plan.Options{Profile: "default", OS: "windows"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint, ok, err := plan.SkippedEntries(m, tt.opts)
			if err != nil {
				t.Fatalf("SkippedEntries() error = %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("SkippedEntries() ok = %v, want %v (hint=%+v)", ok, tt.wantOK, hint)
			}
			if !tt.wantOK {
				return
			}
			if hint.Profile != tt.opts.Profile {
				t.Fatalf("hint.Profile = %q, want %q", hint.Profile, tt.opts.Profile)
			}
			if hint.Count != tt.wantCount {
				t.Fatalf("hint.Count = %d, want %d", hint.Count, tt.wantCount)
			}
			if hint.SuggestedProfile != tt.wantSuggested {
				t.Fatalf("hint.SuggestedProfile = %q, want %q", hint.SuggestedProfile, tt.wantSuggested)
			}
		})
	}
}

// TestSkippedEntriesCountReflectsSuggestedCoverage proves Count is the suggested
// profile's coverage of the skipped set, not the union of omissions across every
// profile, so the "run --profile S to include them" message never promises more
// than S delivers when no single profile is a superset.
func TestSkippedEntriesCountReflectsSuggestedCoverage(t *testing.T) {
	// Three non-nested profiles: neither "a" nor "b" is a superset of the two
	// extras the default profile omits. The union of omissions is 2, but each
	// other profile recovers only 1.
	m := manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"default": {Tags: []string{"core"}},
			"a":       {Tags: []string{"core", "a"}},
			"b":       {Tags: []string{"core", "b"}},
		},
		Entries: []manifest.Entry{
			coreEntry(),
			{Source: "configs/a", Target: "~/.a", Strategy: "symlink", Tags: []string{"a"}},
			{Source: "configs/b", Target: "~/.b", Strategy: "symlink", Tags: []string{"b"}},
		},
	}

	hint, ok, err := plan.SkippedEntries(m, plan.Options{Profile: "default", OS: "darwin"})
	if err != nil {
		t.Fatalf("SkippedEntries() error = %v", err)
	}
	if !ok {
		t.Fatalf("SkippedEntries() ok = false, want true")
	}
	if hint.Count != 1 {
		t.Fatalf("hint.Count = %d, want 1 (suggested profile's coverage, not the union of 2)", hint.Count)
	}
	if hint.SuggestedProfile != "a" {
		t.Fatalf("hint.SuggestedProfile = %q, want %q (alphabetical tie-break on equal coverage)", hint.SuggestedProfile, "a")
	}
}

func TestSkippedEntriesUnknownProfileErrors(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: nestedProfiles(),
		Entries:  []manifest.Entry{coreEntry()},
	}
	if _, _, err := plan.SkippedEntries(m, plan.Options{Profile: "ghost", OS: "darwin"}); err == nil {
		t.Fatal("SkippedEntries() error = nil, want unknown-profile error")
	}
}

func TestSkippedEntriesComposedSelectionSuggestsAddingProfile(t *testing.T) {
	m := manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"core":    {Tags: []string{"core"}},
			"web":     {Tags: []string{"web"}},
			"desktop": {Tags: []string{"desktop"}},
		},
		Entries: []manifest.Entry{
			coreEntry(),
			{Source: "configs/opencode", Target: "~/.config/opencode", Strategy: "copy", Tags: []string{"web"}},
			desktopEntry("configs/ghostty/config.ghostty"),
		},
	}

	hint, ok, err := plan.SkippedEntries(m, plan.Options{Profiles: []string{"core", "web"}, OS: "darwin"})
	if err != nil {
		t.Fatalf("SkippedEntries() error = %v", err)
	}
	if !ok {
		t.Fatal("SkippedEntries() ok = false, want true")
	}
	if hint.SuggestedProfile != "desktop" {
		t.Fatalf("SuggestedProfile = %q, want desktop", hint.SuggestedProfile)
	}
	if got, want := hint.SuggestedProfiles, []string{"core", "web", "desktop"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SuggestedProfiles = %#v, want %#v", got, want)
	}
}
