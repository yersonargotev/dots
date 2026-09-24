package manifest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestTerminalSelectedSurfaceIntegration(t *testing.T) {
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

	zsh, zshErr := exec.LookPath("zsh")
	starship, starshipErr := exec.LookPath("starship")
	if zshErr != nil || starshipErr != nil {
		t.Skipf("terminal runtime acceptance needs zsh and starship: zsh=%v, starship=%v", zshErr, starshipErr)
	}

	home := t.TempDir()
	for target, source := range map[string]string{
		".zshenv":                "configs/zsh/zshenv",
		".config/dots/zsh/zshrc": "configs/zsh/zshrc",
		".config/starship.toml":  "configs/starship/starship.toml",
	} {
		path := filepath.Join(home, target)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, source), path); err != nil {
			t.Fatal(err)
		}
	}
	loader, err := os.ReadFile(filepath.Join(root, "configs/zsh/loader.zsh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), loader, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(zsh, "-i", "-c", `print -r -- "STARSHIP_HOOK=${precmd_functions[(r)prompt_starship_precmd]}"; print -P -- "$PROMPT"`)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"ZDOTDIR="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"STARSHIP_CONFIG="+filepath.Join(home, ".config", "starship.toml"),
		"TERM=xterm-256color",
		"PATH="+filepath.Dir(starship)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sandboxed Zsh startup: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "STARSHIP_HOOK=prompt_starship_precmd") || !strings.Contains(string(output), "❯") {
		t.Fatalf("selected Zsh and Starship configuration did not initialize and render the prompt:\n%s", output)
	}
}
