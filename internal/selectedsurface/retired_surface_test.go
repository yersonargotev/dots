package selectedsurface_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestRepositoryRetiredCapabilitiesAreNotOffered(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	retired := []string{"tmux", "zellij", "tuicr", "warp", "zed", "copilot", "opencode", "antigravity", "codegraph", "playwright", "frontend-design", "vercel-web-skills", "claude-chrome-devtools", "codex-chrome-devtools", "opencode-chrome-devtools", "web"}
	for _, tag := range retired {
		if _, ok := m.Tags[tag]; ok {
			t.Errorf("retired Tag %q is still offered", tag)
		}
		if _, err := manifest.ResolveReadOnlySelection(*m, nil, []string{tag}); err == nil {
			t.Errorf("retired Tag %q can still be selected", tag)
		}
	}
	if _, err := manifest.ResolveReadOnlySelection(*m, []string{"web"}, nil); err == nil {
		t.Error("retired web Profile can still be selected")
	}
	for _, platform := range []struct{ os, arch string }{{"darwin", "arm64"}, {"linux", "arm64"}, {"linux", "amd64"}} {
		t.Run(platform.os+"/"+platform.arch, func(t *testing.T) {
			for name := range m.Profiles {
				selection, err := manifest.ResolveReadOnlySelection(*m, []string{name}, []string{"adaptive-theme", "theme-carbonfox"})
				if err != nil {
					t.Fatal(err)
				}
				surface := selectedsurface.EvaluateForPlatform(*m, selection.Tags, platform.os, platform.arch)
				data, err := json.Marshal(surface)
				if err != nil {
					t.Fatal(err)
				}
				for _, word := range retired {
					if strings.Contains(strings.ToLower(string(data)), word) {
						t.Errorf("Profile %s still selects retired capability %q", name, word)
					}
				}
			}
		})
	}
}
