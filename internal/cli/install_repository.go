package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/bootstrap"
	"github.com/yersonargotev/dots/internal/gitrepo"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/repositoryrefresh"
	"github.com/yersonargotev/dots/internal/version"
)

type installRepositoryPreparation struct {
	SourceReadRoot   string
	LegacyMigrations map[string]plan.LegacyMigration
	Refresh          *gitrepo.Update
	cleanup          func()
}

func prepareInstallRepository(cmd *cobra.Command, paths resolvedPaths, dryRun bool) (installRepositoryPreparation, error) {
	result := installRepositoryPreparation{SourceReadRoot: paths.SourceRoot, LegacyMigrations: map[string]plan.LegacyMigration{}, cleanup: func() {}}
	ref := defaultInitRepositoryRef("", version.Value)
	existed := gitrepo.IsRepo(paths.SourceRoot)
	if dryRun {
		if !existed {
			if err := bootstrap.RequireCurrentRef(bootstrap.Options{SourceRoot: paths.SourceRoot, RepositoryRef: ref}); err != nil {
				return result, err
			}
			return result, nil
		}
	} else {
		if _, err := bootstrap.Ensure(bootstrap.Options{SourceRoot: paths.SourceRoot, RepositoryRef: ref}); err != nil {
			return result, err
		}
		if !existed || ref == "" {
			return result, nil
		}
	}
	if ref == "" {
		return result, nil
	}

	oldManifest, err := manifest.LoadFile(filepath.Join(paths.SourceRoot, "dots.yaml"))
	if err != nil {
		return result, err
	}
	meta, err := loadInstallationMetadata(paths, paths.StateRoot)
	if err != nil {
		return result, err
	}
	preview, err := gitrepo.RefPreview(paths.SourceRoot, ref)
	if err != nil {
		return result, err
	}
	captures, err := repositoryrefresh.CaptureLegacyTargets(*oldManifest, meta, paths.SourceRoot, paths.Home, preview.OldRev)
	if err != nil {
		return result, err
	}
	result.LegacyMigrations = captures
	result.Refresh = &preview
	if dryRun {
		if preview.Changed() {
			snapshot, err := os.MkdirTemp("", "dots-install-preview-*")
			if err != nil {
				return result, fmt.Errorf("create install preview snapshot: %w", err)
			}
			result.cleanup = func() { _ = os.RemoveAll(snapshot) }
			if err := gitrepo.ExportRevision(paths.SourceRoot, preview.NewRev, snapshot); err != nil {
				result.cleanup()
				return result, err
			}
			result.SourceReadRoot = snapshot
		}
		if !wantsJSON(cmd) {
			renderUpdate(cmd.OutOrStdout(), preview, true)
		}
		return result, nil
	}

	applied, err := gitrepo.CheckoutRef(paths.SourceRoot, preview)
	if err != nil {
		return result, err
	}
	result.Refresh = &applied
	if !wantsJSON(cmd) {
		renderUpdate(cmd.OutOrStdout(), applied, false)
	}
	return result, nil
}
