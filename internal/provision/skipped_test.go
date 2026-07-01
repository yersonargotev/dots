package provision_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
)

func TestSkippedProvisioners(t *testing.T) {
	coreProv := manifest.Provisioner{
		Tool: "gentle-ai", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Scope: "global"},
	}
	desktopMarketplace := manifest.Provisioner{
		Tool: "claude", Tags: []string{"desktop"}, OS: []string{"darwin", "linux"},
		Spec: manifest.ProvisionerSpec{Marketplace: "ChromeDevTools/chrome-devtools-mcp"},
	}
	desktopPlugin := manifest.Provisioner{
		Tool: "claude", Tags: []string{"desktop"}, OS: []string{"darwin", "linux"},
		Spec: manifest.ProvisionerSpec{Plugin: "chrome-devtools-mcp", From: "chrome-devtools-plugins"},
	}

	tests := []struct {
		name          string
		m             manifest.Manifest
		opts          provision.Options
		wantOK        bool
		wantCount     int
		wantSuggested string
	}{
		{
			name:          "default profile reports the desktop-only provisioners it skips",
			m:             manifestWithProvisioners(coreProv, desktopMarketplace, desktopPlugin),
			opts:          provision.Options{Profile: "default", OS: "darwin"},
			wantOK:        true,
			wantCount:     2,
			wantSuggested: "desktop",
		},
		{
			name:   "desktop profile selects everything so there is no hint",
			m:      manifestWithProvisioners(coreProv, desktopMarketplace, desktopPlugin),
			opts:   provision.Options{Profile: "desktop", OS: "darwin"},
			wantOK: false,
		},
		{
			name:   "no hint when the skipped provisioners are excluded on this OS",
			m:      manifestWithProvisioners(coreProv, desktopMarketplace, desktopPlugin),
			opts:   provision.Options{Profile: "default", OS: "windows"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint, ok, err := provision.SkippedProvisioners(tt.m, tt.opts)
			if err != nil {
				t.Fatalf("SkippedProvisioners() error = %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("SkippedProvisioners() ok = %v, want %v (hint=%+v)", ok, tt.wantOK, hint)
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

// TestSkippedProvisionersCountReflectsSuggestedCoverage proves Count is the
// suggested profile's coverage of the skipped set, not the union of omissions
// across every profile, so the "run --profile S to include them" message never
// promises more than S delivers when no single profile is a superset.
func TestSkippedProvisionersCountReflectsSuggestedCoverage(t *testing.T) {
	coreProv := manifest.Provisioner{
		Tool: "gentle-ai", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Scope: "global"},
	}
	onlyAProv := manifest.Provisioner{
		Tool: "claude", Tags: []string{"a"},
		Spec: manifest.ProvisionerSpec{Marketplace: "owner/repo"},
	}
	onlyBProv := manifest.Provisioner{
		Tool: "codex", Tags: []string{"b"},
		Spec: manifest.ProvisionerSpec{MCP: "srv", Command: []string{"npx"}},
	}
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
		Entries: []manifest.Entry{{
			Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"},
		}},
		Provisioners: []manifest.Provisioner{coreProv, onlyAProv, onlyBProv},
	}

	hint, ok, err := provision.SkippedProvisioners(m, provision.Options{Profile: "default", OS: "darwin"})
	if err != nil {
		t.Fatalf("SkippedProvisioners() error = %v", err)
	}
	if !ok {
		t.Fatalf("SkippedProvisioners() ok = false, want true")
	}
	if hint.Count != 1 {
		t.Fatalf("hint.Count = %d, want 1 (suggested profile's coverage, not the union of 2)", hint.Count)
	}
	if hint.SuggestedProfile != "a" {
		t.Fatalf("hint.SuggestedProfile = %q, want %q (alphabetical tie-break on equal coverage)", hint.SuggestedProfile, "a")
	}
}

func TestSkippedProvisionersUnknownProfileErrors(t *testing.T) {
	m := manifestWithProvisioners(manifest.Provisioner{
		Tool: "gentle-ai", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Scope: "global"},
	})
	if _, _, err := provision.SkippedProvisioners(m, provision.Options{Profile: "ghost", OS: "darwin"}); err == nil {
		t.Fatal("SkippedProvisioners() error = nil, want unknown-profile error")
	}
}

func TestSkippedProvisionersComposedSelectionSuggestsAddingProfile(t *testing.T) {
	coreProv := manifest.Provisioner{Tool: "gentle-ai", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Scope: "global"}}
	webProv := manifest.Provisioner{Tool: "codex", Tags: []string{"web"}, Spec: manifest.ProvisionerSpec{MCP: "web", Command: []string{"npx"}}}
	desktopProv := manifest.Provisioner{Tool: "claude", Tags: []string{"desktop"}, Spec: manifest.ProvisionerSpec{Marketplace: "owner/repo"}}
	m := manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"core":    {Tags: []string{"core"}},
			"web":     {Tags: []string{"web"}},
			"desktop": {Tags: []string{"desktop"}},
		},
		Provisioners: []manifest.Provisioner{coreProv, webProv, desktopProv},
	}

	hint, ok, err := provision.SkippedProvisioners(m, provision.Options{Profiles: []string{"core", "web"}, OS: "darwin"})
	if err != nil {
		t.Fatalf("SkippedProvisioners() error = %v", err)
	}
	if !ok {
		t.Fatal("SkippedProvisioners() ok = false, want true")
	}
	if hint.SuggestedProfile != "desktop" {
		t.Fatalf("SuggestedProfile = %q, want desktop", hint.SuggestedProfile)
	}
	if got, want := hint.SuggestedProfiles, []string{"core", "web", "desktop"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SuggestedProfiles = %#v, want %#v", got, want)
	}
}
