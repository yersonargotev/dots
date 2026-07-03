package testrepo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const herdrManifest = `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/herdr/config.toml
    target: ~/.config/herdr/config.toml
    strategy: symlink
    tags: [core]
`

// TagWithHerdrManifest adds a Herdr-managed entry to a test Source of Truth and
// tags the resulting commit. The repository must already be initialized and
// configured for commits.
func TagWithHerdrManifest(repo, tag string) error {
	if err := writeFile(filepath.Join(repo, "dots.yaml"), herdrManifest); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(repo, "configs", "herdr", "config.toml"), "[keys]\nprefix = \"ctrl+a\"\n"); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"add", "."},
		{"commit", "-m", "add herdr"},
		{"tag", tag},
	} {
		if err := git(repo, args...); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func git(repo string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v failed: %w\n%s", args, err, output)
	}
	return nil
}
