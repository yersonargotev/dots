package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

type resolvedPaths struct {
	Home       string
	SourceRoot string
	StateRoot  string
}

func resolvePaths(home, sourceRoot, stateRoot string) (resolvedPaths, error) {
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return resolvedPaths{}, fmt.Errorf("resolve home directory: %w", err)
		}
		home = resolved
	}

	if sourceRoot == "" {
		sourceRoot = defaultSourceRoot(home)
	}

	if stateRoot == "" {
		stateRoot = defaultStateRoot(home)
	}

	return resolvedPaths{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot}, nil
}

// defaultSourceRoot is the default location of the Installed Repository: the
// checked-out copy of the dotfiles source that the installer reads from.
func defaultSourceRoot(home string) string {
	return filepath.Join(home, ".local", "share", "dots")
}

// defaultStateRoot is the default location of the state directory where
// Installation Metadata (installed.json) is recorded.
func defaultStateRoot(home string) string {
	return filepath.Join(home, ".local", "state", "dots")
}
