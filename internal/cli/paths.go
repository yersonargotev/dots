package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/manifest"
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

// resolveManifestPath returns the Install Manifest path for a command. When
// --file is omitted, the manifest belongs to the Installed Repository instead
// of the caller's current directory, so a released binary can use the shared
// Source of Truth at ~/.local/share/dots/dots.yaml from any cwd. Explicit
// --file values keep normal caller-relative behavior for development and tests.
func resolveManifestPath(cmd *cobra.Command, file, sourceRoot string) string {
	if cmd.Flags().Changed("file") {
		return file
	}
	return filepath.Join(sourceRoot, file)
}

func loadManifestForCommand(cmd *cobra.Command, file, sourceRoot string) (*manifest.Manifest, error) {
	m, err := manifest.LoadFile(resolveManifestPath(cmd, file, sourceRoot))
	if err != nil {
		return nil, manifestErrorWithInitHint(cmd, sourceRoot, err)
	}
	return m, nil
}

func manifestErrorWithInitHint(cmd *cobra.Command, sourceRoot string, err error) error {
	if cmd.Flags().Changed("file") || !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return fmt.Errorf("%w\nInstalled Repository not found or missing dots.yaml at %s.\nRun `dots init`, then retry `dots %s`.", err, sourceRoot, commandName(cmd))
}
