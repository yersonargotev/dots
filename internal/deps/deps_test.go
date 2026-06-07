package deps_test

import (
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

func lookupSet(present ...string) deps.Lookup {
	set := make(map[string]bool, len(present))
	for _, p := range present {
		set[p] = true
	}
	return func(command string) bool { return set[command] }
}

func TestCheckReportsPresentAndMissingForProfile(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/tmux/tmux.conf", Target: "~/.tmux.conf", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "tmux", Brew: "tmux"},
					{Name: "starship", Brew: "starship"},
				},
			},
		},
	}

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if report.Profile != "default" {
		t.Fatalf("Profile = %q, want default", report.Profile)
	}
	if len(report.Results) != 2 {
		t.Fatalf("Results len = %d, want 2 (%#v)", len(report.Results), report.Results)
	}
	if report.Results[0] != (deps.Result{Name: "tmux", Command: "tmux", Present: true}) {
		t.Fatalf("Results[0] = %#v, want present tmux", report.Results[0])
	}
	if report.Results[1] != (deps.Result{Name: "starship", Command: "starship", Present: false}) {
		t.Fatalf("Results[1] = %#v, want missing starship", report.Results[1])
	}
}

func TestCheckDedupesAndSkipsEntriesFilteredOutByOS(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{{Name: "zsh"}},
			},
			{
				// Same dependency declared again: must be deduplicated.
				Source: "configs/zsh/aliases", Target: "~/.aliases", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{{Name: "zsh"}, {Name: "starship"}},
			},
			{
				// Linux-only entry: its dependency must not appear on darwin.
				Source: "configs/i3/config", Target: "~/.config/i3/config", Strategy: "symlink", Tags: []string{"core"}, OS: []string{"linux"},
				Dependencies: []manifest.Dependency{{Name: "i3"}},
			},
			{
				// Not in profile: excluded.
				Source: "configs/work/x", Target: "~/.work", Strategy: "symlink", Tags: []string{"work"},
				Dependencies: []manifest.Dependency{{Name: "slack"}},
			},
		},
	}

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("zsh"))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	var names []string
	for _, r := range report.Results {
		names = append(names, r.Name)
	}
	want := []string{"zsh", "starship"}
	if len(names) != len(want) {
		t.Fatalf("dependency names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("dependency names = %v, want %v", names, want)
		}
	}
}

func TestCheckTrimsAndDedupesPaddedNames(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/a", Target: "~/.a", Strategy: "symlink", Tags: []string{"core"},
				// Same logical dependency declared with and without padding: must
				// deduplicate to a single trimmed result.
				Dependencies: []manifest.Dependency{{Name: " tmux "}, {Name: "tmux"}},
			},
		},
	}

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("Results len = %d, want 1 deduped result (%#v)", len(report.Results), report.Results)
	}
	if report.Results[0].Name != "tmux" {
		t.Fatalf("Results[0].Name = %q, want trimmed %q", report.Results[0].Name, "tmux")
	}
}

func TestCheckFailsOnUnknownProfile(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries:  []manifest.Entry{{Source: "s", Target: "~/.t", Strategy: "symlink", Tags: []string{"core"}}},
	}

	if _, err := deps.Check(m, deps.Options{Profile: "missing", OS: "darwin"}, lookupSet()); err == nil {
		t.Fatal("Check() error = nil, want error for unknown profile")
	}
}
