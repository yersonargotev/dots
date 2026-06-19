package cli

import (
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func loadInstallationMetadata(paths resolvedPaths, requestedStateRoot string) (state.Metadata, error) {
	if requestedStateRoot == "" || plan.InsideRoot(paths.StateRoot, paths.Home) {
		if err := plan.ValidatePathInsideHomeNoSymlinkEscape(paths.StateRoot, paths.Home, "state root"); err != nil {
			return state.Metadata{}, err
		}
		if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(state.Path(paths.StateRoot), paths.Home, "installation metadata"); err != nil {
			return state.Metadata{}, err
		}
	}
	return state.Load(state.Path(paths.StateRoot))
}
