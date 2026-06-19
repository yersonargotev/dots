package deps_test

import (
	"errors"
	"strings"
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

func fontLookupSet(present ...string) deps.FontLookup {
	set := make(map[string]bool, len(present))
	for _, p := range present {
		set[p] = true
	}
	return func(match string) bool { return set[match] }
}

func TestCheckDetectsFontDependencyByScanNotPath(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"desktop"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zed/settings.json", Target: "~/.config/zed/settings.json", Strategy: "symlink", Tags: []string{"desktop"},
				Dependencies: []manifest.Dependency{
					{Name: "zed", Command: "zed"},
					{Name: "CascadiaCode NF", FontMatch: "CascadiaCodeNF*", Brew: "font-cascadia-code-nf"},
				},
			},
		},
	}

	// The font is installed on disk (font scan hits) but has no executable on
	// PATH; the command tool is the inverse. Detection must use the right probe
	// for each.
	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"},
		lookupSet("zed"), fontLookupSet("CascadiaCodeNF*"))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(report.Results) != 2 {
		t.Fatalf("Results len = %d, want 2 (%#v)", len(report.Results), report.Results)
	}
	if !report.Results[1].Present {
		t.Fatalf("Results[1] = %#v, want font present via scan", report.Results[1])
	}
	if report.Results[1].Name != "CascadiaCode NF" {
		t.Fatalf("Results[1].Name = %q, want CascadiaCode NF", report.Results[1].Name)
	}
}

func TestCheckDetectsFontDependencyByFallbackMatch(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"desktop"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zed/settings.json", Target: "~/.config/zed/settings.json", Strategy: "symlink", Tags: []string{"desktop"},
				Dependencies: []manifest.Dependency{
					{
						Name:                "Desktop Nerd Font",
						FontMatch:           "CascadiaCodeNF*",
						FontFallbackMatches: []string{"CaskaydiaCoveNerdFont*"},
						BrewCask:            "font-cascadia-code-nf",
					},
				},
			},
		},
	}

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"},
		lookupSet(), fontLookupSet("CaskaydiaCoveNerdFont*"))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("Results len = %d, want 1 (%#v)", len(report.Results), report.Results)
	}
	if !report.Results[0].Present {
		t.Fatalf("Results[0] = %#v, want present through compatible fallback font", report.Results[0])
	}
}

func TestCheckIncludesProfileDependenciesBeforeEntryDependencies(t *testing.T) {
	m := manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"default": {
				Tags: []string{"desktop"},
				Dependencies: []manifest.Dependency{
					{Name: "Desktop Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*"},
				},
			},
		},
		Entries: []manifest.Entry{
			{
				Source: "configs/ghostty/config.ghostty", Target: "~/.config/ghostty/config.ghostty", Strategy: "symlink", Tags: []string{"desktop"},
				Dependencies: []manifest.Dependency{{Name: "ghostty", Command: "ghostty", Brew: "ghostty"}},
			},
		},
	}

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("ghostty"), fontLookupSet("CascadiaCodeNF*"))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	want := []deps.Result{
		{Name: "Desktop Nerd Font", Command: "CascadiaCodeNF*", Present: true},
		{Name: "ghostty", Command: "ghostty", Present: true},
	}
	if len(report.Results) != len(want) {
		t.Fatalf("Results len = %d, want %d (%#v)", len(report.Results), len(want), report.Results)
	}
	for i := range want {
		if report.Results[i] != want[i] {
			t.Fatalf("Results[%d] = %#v, want %#v", i, report.Results[i], want[i])
		}
	}
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

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"), fontLookupSet())
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

func TestCheckWithToolProbesReportsGitToolchainWarnings(t *testing.T) {
	errProbe := errors.New("exit status 1")
	xcrunOutput := "xcrun: error: invalid active developer path (/Library/Developer/CommandLineTools), missing xcrun"

	tests := []struct {
		name          string
		goos          string
		present       []string
		output        string
		runErr        error
		wantPresent   bool
		wantWarning   bool
		wantHint      bool
		wantDetail    string
		wantProbeRuns int
	}{
		{name: "git probe succeeds", goos: "darwin", present: []string{"git"}, wantPresent: true, wantProbeRuns: 1},
		{name: "git missing skips probe", goos: "darwin", present: nil, wantPresent: false},
		{name: "git probe fails", goos: "darwin", present: []string{"git"}, runErr: errProbe, wantPresent: true, wantWarning: true, wantDetail: "exit status 1", wantProbeRuns: 1},
		{name: "darwin xcrun failure includes repair hint", goos: "darwin", present: []string{"git"}, output: xcrunOutput, runErr: errProbe, wantPresent: true, wantWarning: true, wantHint: true, wantDetail: xcrunOutput, wantProbeRuns: 1},
		{name: "linux xcrun-looking failure keeps generic warning", goos: "linux", present: []string{"git"}, output: xcrunOutput, runErr: errProbe, wantPresent: true, wantWarning: true, wantDetail: xcrunOutput, wantProbeRuns: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := manifest.Manifest{
				Version:  1,
				Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
				Entries: []manifest.Entry{{
					Source: "configs/git/config", Target: "~/.gitconfig", Strategy: "copy", Tags: []string{"core"},
					Dependencies: []manifest.Dependency{{Name: "git"}},
				}},
			}

			var probeRuns int
			report, err := deps.CheckWithToolProbes(m, deps.Options{Profile: "default", OS: tt.goos}, lookupSet(tt.present...), fontLookupSet(),
				func(command string, args ...string) (string, error) {
					probeRuns++
					if command != "git" {
						t.Fatalf("probe command = %q, want git", command)
					}
					if len(args) != 1 || args[0] != "--version" {
						t.Fatalf("probe args = %#v, want [--version]", args)
					}
					return tt.output, tt.runErr
				})
			if err != nil {
				t.Fatalf("CheckWithToolProbes() error = %v", err)
			}

			if probeRuns != tt.wantProbeRuns {
				t.Fatalf("probe runs = %d, want %d", probeRuns, tt.wantProbeRuns)
			}
			if len(report.Results) != 1 {
				t.Fatalf("Results len = %d, want 1 (%#v)", len(report.Results), report.Results)
			}
			result := report.Results[0]
			if result.Present != tt.wantPresent {
				t.Fatalf("Present = %v, want %v (%#v)", result.Present, tt.wantPresent, result)
			}
			if (result.Warning != "") != tt.wantWarning {
				t.Fatalf("Warning = %q, want warning=%v", result.Warning, tt.wantWarning)
			}
			if (result.Hint != "") != tt.wantHint {
				t.Fatalf("Hint = %q, want hint=%v", result.Hint, tt.wantHint)
			}
			if result.ProbeDetail != tt.wantDetail {
				t.Fatalf("ProbeDetail = %q, want %q", result.ProbeDetail, tt.wantDetail)
			}
		})
	}
}

func TestCheckWithToolProbesSanitizesAndTruncatesGitProbeDetail(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source: "configs/git/config", Target: "~/.gitconfig", Strategy: "copy", Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{Name: "git"}},
		}},
	}
	output := "  xcrun: error: invalid active developer path\n\t(/Library/Developer/CommandLineTools), missing xcrun  " + strings.Repeat(" detail", 80)

	report, err := deps.CheckWithToolProbes(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("git"), fontLookupSet(),
		func(command string, args ...string) (string, error) {
			return output, errors.New("exit status 1")
		})
	if err != nil {
		t.Fatalf("CheckWithToolProbes() error = %v", err)
	}

	detail := report.Results[0].ProbeDetail
	if strings.ContainsAny(detail, "\n\r\t") {
		t.Fatalf("ProbeDetail contains raw whitespace: %q", detail)
	}
	if !strings.Contains(detail, "xcrun: error: invalid active developer path (/Library/Developer/CommandLineTools), missing xcrun") {
		t.Fatalf("ProbeDetail = %q, want sanitized xcrun error", detail)
	}
	if len(detail) > 240 {
		t.Fatalf("ProbeDetail length = %d, want <= 240", len(detail))
	}
	if !strings.HasSuffix(detail, "...") {
		t.Fatalf("ProbeDetail = %q, want truncation marker", detail)
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

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("zsh"), fontLookupSet())
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

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet(), fontLookupSet())
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

	if _, err := deps.Check(m, deps.Options{Profile: "missing", OS: "darwin"}, lookupSet(), fontLookupSet()); err == nil {
		t.Fatal("Check() error = nil, want error for unknown profile")
	}
}

func TestCheckIncludesSelectedProvisionerDependencies(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{{Name: "zsh"}},
			},
		},
		Provisioners: []manifest.Provisioner{
			{
				Tool: "gentle-ai",
				Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "gentle-ai", Brew: "gentleman-programming/tap/gentle-ai"},
					{Name: "engram", Brew: "gentleman-programming/tap/engram"},
				},
			},
		},
	}

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("zsh", "engram"), fontLookupSet())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	want := []deps.Result{
		{Name: "zsh", Command: "zsh", Present: true},
		{Name: "gentle-ai", Command: "gentle-ai", Present: false},
		{Name: "engram", Command: "engram", Present: true},
	}
	if len(report.Results) != len(want) {
		t.Fatalf("Results len = %d, want %d (%#v)", len(report.Results), len(want), report.Results)
	}
	for i := range want {
		if report.Results[i] != want[i] {
			t.Fatalf("Results[%d] = %#v, want %#v", i, report.Results[i], want[i])
		}
	}
}

func TestCheckFiltersProvisionerDependenciesByProfileAndOS(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Provisioners: []manifest.Provisioner{
			{Tool: "gentle-ai", Tags: []string{"core"}, OS: []string{"darwin"}, Dependencies: []manifest.Dependency{{Name: "gentle-ai"}}},
			{Tool: "opencode", Tags: []string{"work"}, Dependencies: []manifest.Dependency{{Name: "opencode"}}},
			{Tool: "claude", Tags: []string{"core"}, OS: []string{"linux"}, Dependencies: []manifest.Dependency{{Name: "claude"}}},
		},
	}

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet(), fontLookupSet())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if len(report.Results) != 1 {
		t.Fatalf("Results len = %d, want 1 (%#v)", len(report.Results), report.Results)
	}
	if report.Results[0].Name != "gentle-ai" {
		t.Fatalf("Results[0].Name = %q, want gentle-ai", report.Results[0].Name)
	}
}

func TestCheckDedupesProvisionerDependenciesWithEntries(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/a", Target: "~/.a", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{{Name: "gentle-ai", Brew: "gentleman-programming/tap/gentle-ai"}},
			},
		},
		Provisioners: []manifest.Provisioner{
			{
				Tool:         "gentle-ai",
				Tags:         []string{"core"},
				Dependencies: []manifest.Dependency{{Name: " gentle-ai "}, {Name: "engram"}},
			},
		},
	}

	report, err := deps.Check(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet(), fontLookupSet())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	got := make([]string, 0, len(report.Results))
	for _, result := range report.Results {
		got = append(got, result.Name)
	}
	want := []string{"gentle-ai", "engram"}
	if len(got) != len(want) {
		t.Fatalf("dependency names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dependency names = %v, want %v", got, want)
		}
	}
}
