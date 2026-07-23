package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/upgrade"
	"github.com/yersonargotev/dots/internal/version"
)

var (
	currentExecutable = os.Executable
	execBinary        = syscall.Exec
)

func newUpgradeCommand() *cobra.Command {
	var (
		file       string
		profiles   []string
		extraTags  []string
		sourceRoot string
		home       string
		stateRoot  string
		dryRun     bool
		yes        bool
		noTUI      bool
		continue_  bool

		binaryChannel        string
		binaryCurrentVersion string
		binaryLatestVersion  string
		binaryAction         string
		binaryExecutable     string
		binaryArtifact       string
		binaryChecksum       string
		selectionSource      string
		selectionProfiles    []string
		selectionTags        []string
	)

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade the dots binary and refresh dots-owned configuration",
		Long: "upgrade updates only dots-owned surfaces: the dots binary, the Installed Repository, " +
			"Managed Entries, and Provisioners. It never performs a broad system package upgrade.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := updateOptions{file: file, profiles: profiles, extraTags: extraTags, sourceRoot: sourceRoot, home: home, stateRoot: stateRoot, dryRun: dryRun, yes: yes, noTUI: noTUI}
			if continue_ {
				if selectionSource != "" {
					intent := selection.Intent{Source: selection.Source(selectionSource), Profiles: selectionProfiles, ExtraTags: selectionTags}
					opts.selectionIntent = &intent
				}
				updateReport, err := runUpdateWorkflow(cmd, opts, !wantsJSON(cmd))
				if err != nil {
					return err
				}
				if wantsJSON(cmd) {
					binPlan := upgrade.Plan{Channel: binaryChannel, CurrentVersion: binaryCurrentVersion, LatestVersion: binaryLatestVersion, Action: binaryAction, Executable: binaryExecutable, Artifact: binaryArtifact, Checksum: binaryChecksum}
					return emitOK(cmd, upgradeReport{DryRun: false, Selection: updateReport.Selection, Binary: binPlan, Update: updateReport.Update, Plan: updateReport.Plan, Provisioners: updateReport.Provisioners})
				}
				return nil
			}
			if wantsJSON(cmd) && !dryRun && !yes {
				return rejectInteractiveJSON(cmd)
			}
			exe, err := currentExecutable()
			if err != nil {
				return err
			}
			binOpts := upgrade.Options{CurrentVersion: version.Value, Executable: exe}
			if dryRun {
				binPlan, err := upgrade.Preview(cmd.Context(), binOpts)
				if err != nil {
					return err
				}
				if !wantsJSON(cmd) {
					renderUpgradeBinary(cmd.OutOrStdout(), binPlan, true)
				}
				updateReport, err := runUpdateWorkflow(cmd, opts, false)
				if err != nil {
					return err
				}
				if wantsJSON(cmd) {
					return emitOK(cmd, upgradeReport{DryRun: true, Selection: updateReport.Selection, Binary: binPlan, Update: updateReport.Update, Plan: updateReport.Plan, Provisioners: updateReport.Provisioners})
				}
				return nil
			}

			preflightPlan, err := upgrade.Preview(cmd.Context(), binOpts)
			if err != nil {
				return err
			}
			if preflightPlan.Action == upgrade.ActionManualRebuild {
				_, err := upgrade.Execute(cmd.Context(), binOpts)
				return err
			}
			effective, err := resolveUpgradeSelection(cmd, opts)
			if err != nil {
				return err
			}
			intent := effective.Intent()
			opts.selectionIntent = &intent
			binPlan, err := upgrade.Execute(cmd.Context(), binOpts)
			if err != nil {
				return err
			}
			if !wantsJSON(cmd) {
				renderUpgradeBinary(cmd.OutOrStdout(), binPlan, false)
			}
			if binPlan.Action == upgrade.ActionHomebrewUpgrade || binPlan.Action == upgrade.ActionReplaceBinary {
				return execBinary(exe, upgradeContinuationArgs(file, cmd.Flags().Changed("file"), intent, sourceRoot, home, stateRoot, yes, noTUI, wantsJSON(cmd), binPlan), os.Environ())
			}
			updateReport, err := runUpdateWorkflow(cmd, opts, false)
			if err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return emitOK(cmd, upgradeReport{DryRun: false, Selection: updateReport.Selection, Binary: binPlan, Update: updateReport.Update, Plan: updateReport.Plan, Provisioners: updateReport.Provisioners})
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to install after upgrading")
	cmd.Flags().StringArrayVarP(&profiles, "profile", "p", nil, "profile to install after upgrading")
	cmd.Flags().StringArrayVar(&extraTags, "tag", nil, "include an additional manifest tag; repeat to include multiple tags")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root to update (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to install into (default: the current user's home); use a sandbox path to avoid touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata and Backup Sets (default ~/.local/state/dots)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the binary upgrade and Source of Truth update without modifying either phase")
	cmd.Flags().BoolVar(&yes, "yes", false, "apply safe install actions without prompting; conflicts default to skip")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "use text prompts instead of the interactive TUI for conflict resolution")
	cmd.Flags().BoolVar(&continue_, "continue", false, "continue Source of Truth upgrade after binary replacement")
	cmd.Flags().StringVar(&binaryChannel, "binary-channel", "", "binary phase channel preserved across upgrade continuation")
	cmd.Flags().StringVar(&binaryCurrentVersion, "binary-current-version", "", "binary phase current version preserved across upgrade continuation")
	cmd.Flags().StringVar(&binaryLatestVersion, "binary-latest-version", "", "binary phase latest version preserved across upgrade continuation")
	cmd.Flags().StringVar(&binaryAction, "binary-action", "", "binary phase action preserved across upgrade continuation")
	cmd.Flags().StringVar(&binaryExecutable, "binary-executable", "", "binary phase executable path preserved across upgrade continuation")
	cmd.Flags().StringVar(&binaryArtifact, "binary-artifact", "", "binary phase artifact preserved across upgrade continuation")
	cmd.Flags().StringVar(&binaryChecksum, "binary-checksum", "", "binary phase checksum preserved across upgrade continuation")
	cmd.Flags().StringVar(&selectionSource, "selection-source", "", "selection source preserved across upgrade continuation")
	cmd.Flags().StringArrayVar(&selectionProfiles, "selection-profile", nil, "selection Profile preserved across upgrade continuation")
	cmd.Flags().StringArrayVar(&selectionTags, "selection-tag", nil, "selection extra Tag preserved across upgrade continuation")
	_ = cmd.Flags().MarkHidden("continue")
	_ = cmd.Flags().MarkHidden("binary-channel")
	_ = cmd.Flags().MarkHidden("binary-current-version")
	_ = cmd.Flags().MarkHidden("binary-latest-version")
	_ = cmd.Flags().MarkHidden("binary-action")
	_ = cmd.Flags().MarkHidden("binary-executable")
	_ = cmd.Flags().MarkHidden("binary-artifact")
	_ = cmd.Flags().MarkHidden("binary-checksum")
	_ = cmd.Flags().MarkHidden("selection-source")
	_ = cmd.Flags().MarkHidden("selection-profile")
	_ = cmd.Flags().MarkHidden("selection-tag")
	return cmd
}

func resolveUpgradeSelection(cmd *cobra.Command, opts updateOptions) (selection.Effective, error) {
	paths, err := resolvePaths(opts.home, opts.sourceRoot, opts.stateRoot)
	if err != nil {
		return selection.Effective{}, err
	}
	manifestPath := opts.file
	if !cmd.Flags().Changed("file") {
		manifestPath = filepath.Join(paths.SourceRoot, opts.file)
	}
	return resolveUpdateSelection(manifestPath, paths, opts)
}

func upgradeContinuationArgs(file string, fileChanged bool, intent selection.Intent, sourceRoot, home, stateRoot string, yes, noTUI, json bool, binPlan upgrade.Plan) []string {
	args := []string{"dots", "upgrade", "--continue"}
	if fileChanged {
		args = append(args, "--file", file)
	}
	args = append(args, "--selection-source", string(intent.Source))
	for _, profile := range intent.Profiles {
		args = append(args, "--selection-profile", profile)
	}
	for _, tag := range intent.ExtraTags {
		args = append(args, "--selection-tag", tag)
	}
	if sourceRoot != "" {
		args = append(args, "--source-root", sourceRoot)
	}
	if home != "" {
		args = append(args, "--home", home)
	}
	if stateRoot != "" {
		args = append(args, "--state-root", stateRoot)
	}
	if yes {
		args = append(args, "--yes")
	}
	if noTUI {
		args = append(args, "--no-tui")
	}
	if json {
		args = append(args, "--output", "json")
	}
	args = append(args,
		"--binary-channel", binPlan.Channel,
		"--binary-current-version", binPlan.CurrentVersion,
		"--binary-latest-version", binPlan.LatestVersion,
		"--binary-action", binPlan.Action,
		"--binary-executable", binPlan.Executable,
		"--binary-artifact", binPlan.Artifact,
		"--binary-checksum", binPlan.Checksum,
	)
	return args
}

func renderUpgradeBinary(out interface{ Write([]byte) (int, error) }, plan upgrade.Plan, dryRun bool) {
	prefix := "Binary upgrade"
	if dryRun {
		prefix = "Binary upgrade preview"
	}
	fmt.Fprintf(out, "%s: channel=%s current=%s", prefix, plan.Channel, plan.CurrentVersion)
	if plan.LatestVersion != "" {
		fmt.Fprintf(out, " latest=%s", plan.LatestVersion)
	}
	fmt.Fprintf(out, " action=%s\n\n", plan.Action)
}
