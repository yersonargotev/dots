package profilesel_test

import (
	"errors"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/profilesel"
)

// fakeSelector builds a Selector from a fixed profile -> selected-index set,
// ignoring OS so the math is exercised independently of any real item slice.
func fakeSelector(selections map[string]map[int]bool) profilesel.Selector {
	return func(profileName, _ string) (map[int]bool, error) {
		sel, ok := selections[profileName]
		if !ok {
			return nil, errors.New("unknown profile")
		}
		return sel, nil
	}
}

func profileNames(names ...string) map[string]manifest.Profile {
	m := make(map[string]manifest.Profile, len(names))
	for _, name := range names {
		m[name] = manifest.Profile{Tags: []string{name}}
	}
	return m
}

func TestSkipped(t *testing.T) {
	tests := []struct {
		name          string
		profiles      map[string]manifest.Profile
		selections    map[string]map[int]bool
		active        string
		wantOK        bool
		wantCount     int
		wantSuggested string
	}{
		{
			name:          "nested profile recovers everything the active one skips",
			profiles:      profileNames("default", "desktop"),
			selections:    map[string]map[int]bool{"default": {0: true}, "desktop": {0: true, 1: true, 2: true}},
			active:        "default",
			wantOK:        true,
			wantCount:     2,
			wantSuggested: "desktop",
		},
		{
			name:       "active profile already selects everything",
			profiles:   profileNames("default", "desktop"),
			selections: map[string]map[int]bool{"default": {0: true, 1: true}, "desktop": {0: true, 1: true}},
			active:     "desktop",
			wantOK:     false,
		},
		{
			name:          "non-superset: Count is suggested coverage, not the union of omissions",
			profiles:      profileNames("default", "a", "b"),
			selections:    map[string]map[int]bool{"default": {0: true}, "a": {0: true, 1: true}, "b": {0: true, 2: true}},
			active:        "default",
			wantOK:        true,
			wantCount:     1,   // union of skipped is {1,2}=2, but each other profile recovers only 1
			wantSuggested: "a", // alphabetical tie-break on equal coverage
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint, ok, err := profilesel.Skipped(tt.profiles, tt.active, "darwin", fakeSelector(tt.selections))
			if err != nil {
				t.Fatalf("Skipped() error = %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("Skipped() ok = %v, want %v (hint=%+v)", ok, tt.wantOK, hint)
			}
			if !tt.wantOK {
				return
			}
			if hint.Profile != tt.active {
				t.Fatalf("hint.Profile = %q, want %q", hint.Profile, tt.active)
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

// TestSkippedPropagatesSelectorError proves an error from the active selection
// (e.g. an unknown active profile) surfaces instead of being swallowed.
func TestSkippedPropagatesSelectorError(t *testing.T) {
	profiles := profileNames("default", "desktop")
	sel := fakeSelector(map[string]map[int]bool{"default": {0: true}, "desktop": {0: true, 1: true}})
	if _, _, err := profilesel.Skipped(profiles, "ghost", "darwin", sel); err == nil {
		t.Fatal("Skipped() error = nil, want selector error for unknown active profile")
	}
}
