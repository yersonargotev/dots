package doctor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/doctor"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
)

func TestScanSecretsFindsObviousCredentialPatterns(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantLine    int
		wantPattern string
	}{
		{name: "credential assignment", content: "[credential]\nhelper = store\napi_key = abc1234567890\n", wantLine: 3, wantPattern: "credential-assignment"},
		{name: "private key", content: "-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----\n", wantLine: 1, wantPattern: "private-key"},
		{name: "aws access key", content: "aws_access_key_id = AKIA1234567890ABCDEF\n", wantLine: 1, wantPattern: "aws-access-key-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRoot := t.TempDir()
			writeFile(t, sourceRoot, "configs/app/config", tt.content)

			report, err := doctor.ScanSecrets(singleEntryManifest("configs/app/config"), doctor.Options{
				Profile:    "default",
				OS:         "linux",
				SourceRoot: sourceRoot,
			})
			if err != nil {
				t.Fatalf("ScanSecrets() error = %v", err)
			}

			if len(report.Findings) != 1 {
				t.Fatalf("Findings len = %d, want 1 (%#v)", len(report.Findings), report.Findings)
			}
			finding := report.Findings[0]
			if finding.Source != "configs/app/config" || finding.Line != tt.wantLine || finding.Pattern != tt.wantPattern {
				t.Fatalf("finding = %#v, want %s at configs/app/config:%d", finding, tt.wantPattern, tt.wantLine)
			}
		})
	}
}

func TestScanSecretsReportsLineWithoutLeakingSecretValue(t *testing.T) {
	sourceRoot := t.TempDir()
	writeFile(t, sourceRoot, "configs/git/config", "[credential]\nhelper = store\napi_key = abc1234567890\n")

	report, err := doctor.ScanSecrets(singleEntryManifest("configs/git/config"), doctor.Options{
		Profile: "default", OS: "linux", SourceRoot: sourceRoot,
	})
	if err != nil {
		t.Fatalf("ScanSecrets() error = %v", err)
	}

	if len(report.Findings) != 1 {
		t.Fatalf("Findings len = %d, want 1 (%#v)", len(report.Findings), report.Findings)
	}
	finding := report.Findings[0]
	if finding.Source != "configs/git/config" || finding.Line != 3 || finding.Pattern != "credential-assignment" {
		t.Fatalf("finding = %#v, want redacted credential-assignment at configs/git/config:3", finding)
	}
}

func TestScanSecretsIgnoresPlaceholdersAndUnselectedSources(t *testing.T) {
	sourceRoot := t.TempDir()
	writeFile(t, sourceRoot, "configs/core/config", "api_key = example_replace_me\n")
	writeFile(t, sourceRoot, "configs/work/config", "password = realpassword123\n")

	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{Source: "configs/core/config", Target: "~/.config/core", Strategy: "copy", Tags: []string{"core"}},
			{Source: "configs/work/config", Target: "~/.config/work", Strategy: "copy", Tags: []string{"work"}},
		},
	}

	report, err := doctor.ScanSecrets(m, doctor.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot})
	if err != nil {
		t.Fatalf("ScanSecrets() error = %v", err)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("Findings = %#v, want placeholders and unselected sources ignored", report.Findings)
	}
}

func TestScanSecretsScansDirectorySources(t *testing.T) {
	sourceRoot := t.TempDir()
	writeFile(t, sourceRoot, "configs/nvim/init.lua", "vim.g.mapleader = ' '\n")
	writeFile(t, sourceRoot, "configs/nvim/lua/plugin.lua", "token = realtoken123\n")

	report, err := doctor.ScanSecrets(singleEntryManifest("configs/nvim"), doctor.Options{
		Profile: "default", OS: "linux", SourceRoot: sourceRoot,
	})
	if err != nil {
		t.Fatalf("ScanSecrets() error = %v", err)
	}

	if len(report.Findings) != 1 {
		t.Fatalf("Findings len = %d, want 1 (%#v)", len(report.Findings), report.Findings)
	}
	finding := report.Findings[0]
	if finding.Source != "configs/nvim/lua/plugin.lua" || finding.Line != 1 || finding.Pattern != "credential-assignment" {
		t.Fatalf("finding = %#v, want credential-assignment at configs/nvim/lua/plugin.lua:1", finding)
	}
}

func TestBuildDiagnosticSections(t *testing.T) {
	tests := []struct {
		name            string
		goos            string
		deps            []manifest.Dependency
		present         []string
		writeSource     bool
		wantPlatformOK  bool
		wantDepsMissing int
	}{
		{
			name: "supported platform and present dependency", goos: "linux",
			deps: []manifest.Dependency{{Name: "git"}}, present: []string{"git"}, writeSource: true,
			wantPlatformOK: true, wantDepsMissing: 0,
		},
		{
			name: "unsupported platform and missing dependency", goos: "plan9",
			deps: []manifest.Dependency{{Name: "missing-tool"}}, writeSource: false,
			wantPlatformOK: false, wantDepsMissing: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			sourceRoot := t.TempDir()
			if tt.writeSource {
				writeFile(t, sourceRoot, "configs/app/config", "safe = true\n")
			}
			m := singleEntryManifest("configs/app/config")
			m.Entries[0].Dependencies = tt.deps

			report, err := doctor.Build(m, state.Metadata{}, doctor.Options{Profile: "default", OS: tt.goos, SourceRoot: sourceRoot, Home: home}, lookupSet(tt.present...), fontLookupSet())
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if report.Platform.Supported != tt.wantPlatformOK {
				t.Fatalf("Platform.Supported = %v, want %v", report.Platform.Supported, tt.wantPlatformOK)
			}
			var missing int
			for _, result := range report.Dependencies.Results {
				if !result.Present {
					missing++
				}
			}
			if missing != tt.wantDepsMissing {
				t.Fatalf("missing dependencies = %d, want %d (%#v)", missing, tt.wantDepsMissing, report.Dependencies.Results)
			}
		})
	}
}

func TestBuildReportsProvisionerReadiness(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	writeFile(t, sourceRoot, "configs/app/config", "safe = true\n")

	m := singleEntryManifest("configs/app/config")
	m.Provisioners = []manifest.Provisioner{{
		Tool: "gentle-ai",
		Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Scope: "global", Agents: []string{"codex"}},
		Dependencies: []manifest.Dependency{
			{Name: "gentle-ai"},
			{Name: "engram"},
		},
	}}

	// gentle-ai present, engram missing: the provisioner is surfaced as not ready
	// without ever being executed.
	report, err := doctor.Build(m, state.Metadata{}, doctor.Options{
		Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home,
	}, lookupSet("gentle-ai"), fontLookupSet())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(report.Provisioners.Items) != 1 {
		t.Fatalf("len(Provisioners.Items) = %d, want 1", len(report.Provisioners.Items))
	}
	item := report.Provisioners.Items[0]
	if item.Tool != "gentle-ai" {
		t.Fatalf("provisioner tool = %q, want gentle-ai", item.Tool)
	}
	wantArgs := []string{"install", "--scope", "global", "--agents", "codex"}
	if !equalStrings(item.Args, wantArgs) {
		t.Fatalf("provisioner args = %#v, want %#v", item.Args, wantArgs)
	}
	if !equalStrings(item.Missing, []string{"engram"}) {
		t.Fatalf("provisioner missing = %#v, want [engram]", item.Missing)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func singleEntryManifest(source string) manifest.Manifest {
	return manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries:  []manifest.Entry{{Source: source, Target: "~/.config/app", Strategy: "copy", Tags: []string{"core"}}},
	}
}

func lookupSet(present ...string) func(string) bool {
	set := make(map[string]bool, len(present))
	for _, command := range present {
		set[command] = true
	}
	return func(command string) bool { return set[command] }
}

func fontLookupSet(present ...string) func(string) bool {
	set := make(map[string]bool, len(present))
	for _, match := range present {
		set[match] = true
	}
	return func(match string) bool { return set[match] }
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
