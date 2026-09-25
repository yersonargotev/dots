package manifest_test

import (
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestTerminalAtomicTagsSelectRequiredCapabilities(t *testing.T) {
	root := repositoryRoot(t)
	m, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		tag          string
		osName       string
		arch         string
		sources      []string
		dependencies []string
	}{
		{
			tag: "zsh", osName: "linux", arch: "amd64",
			sources:      []string{"configs/zsh/loader.zsh", "configs/zsh/zshrc", "configs/zsh/zshenv"},
			dependencies: []string{"zsh", "fzf"},
		},
		{
			tag: "starship", osName: "linux", arch: "amd64",
			sources:      []string{"configs/starship/starship.toml"},
			dependencies: []string{"starship"},
		},
		{
			tag: "herdr", osName: "darwin", arch: "arm64",
			sources:      []string{"configs/herdr/config.toml", "configs/herdr/plugins/tabby.toml"},
			dependencies: []string{"herdr", "lazygit", "fzf", "fd", "bat"},
		},
	}
	for _, test := range tests {
		t.Run(test.tag, func(t *testing.T) {
			surface := selectedsurface.EvaluateForPlatform(*m, []string{test.tag}, test.osName, test.arch)
			for _, source := range test.sources {
				found := false
				for _, entry := range surface.Entries {
					if entry.Source == source {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Tag %q omits Managed Entry %q", test.tag, source)
				}
			}
			for _, name := range test.dependencies {
				if !selectedDependencyNamed(surface, name) {
					t.Errorf("Tag %q omits Dependency %q", test.tag, name)
				}
			}
		})
	}
}
