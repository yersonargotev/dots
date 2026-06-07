package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

type resolvedPaths struct {
	Home       string
	SourceRoot string
}

func resolvePaths(home, sourceRoot string) (resolvedPaths, error) {
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

	return resolvedPaths{Home: home, SourceRoot: sourceRoot}, nil
}

// defaultSourceRoot is the default location of the Installed Repository: the
// checked-out copy of the dotfiles source that the installer reads from.
func defaultSourceRoot(home string) string {
	return filepath.Join(home, ".local", "share", "dots")
}
