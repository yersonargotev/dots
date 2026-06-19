package provision_test

import (
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

func TestSkippedProvisionersUnknownProfileErrors(t *testing.T) {
	m := manifestWithProvisioners(manifest.Provisioner{
		Tool: "gentle-ai", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Scope: "global"},
	})
	if _, _, err := provision.SkippedProvisioners(m, provision.Options{Profile: "ghost", OS: "darwin"}); err == nil {
		t.Fatal("SkippedProvisioners() error = nil, want unknown-profile error")
	}
}
