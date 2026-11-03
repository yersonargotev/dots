package cli

import (
	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/upgrade"
)

func legacyUpgradeContinuation(cmd *cobra.Command, opts updateOptions, intent selection.Intent, binary upgrade.Plan, args []string) bool {
	// These fields recognize a protocol shape, never authority or confirmation.
	if cmd.Flags().Changed("selection-previous") || cmd.Flags().Changed("selection-exclude-tag") ||
		cmd.Flags().Changed("profile") || cmd.Flags().Changed("tag") || cmd.Flags().Changed("exclude-tag") ||
		opts.dryRun || len(args) != 0 || binary.CurrentVersion != "v0.96.1" ||
		binary.LatestVersion == "" || binary.Executable == "" ||
		(wantsJSON(cmd) && !opts.yes) ||
		(intent.Source != selection.SourceExplicit && intent.Source != selection.SourceRecorded) {
		return false
	}
	for _, field := range []string{"binary-channel", "binary-current-version", "binary-latest-version", "binary-action", "binary-executable", "binary-artifact", "binary-checksum"} {
		if !cmd.Flags().Changed(field) {
			return false
		}
	}
	switch binary.Channel {
	case upgrade.ChannelRelease:
		return binary.Action == upgrade.ActionReplaceBinary && binary.Artifact != "" && binary.Checksum != ""
	case upgrade.ChannelHomebrew:
		return binary.Action == upgrade.ActionHomebrewUpgrade && binary.Artifact == "" && binary.Checksum == ""
	default:
		return false
	}
}

func upgradeRecoveryCommand(cmd *cobra.Command, opts updateOptions, intent selection.Intent) string {
	args := []string{"upgrade"}
	if intent.Source != selection.SourceRecorded {
		for _, profile := range intent.Profiles {
			args = append(args, "--profile", profile)
		}
		for _, tag := range intent.ExtraTags {
			args = append(args, "--tag", tag)
		}
		for _, tag := range intent.ExcludedTags {
			args = append(args, "--exclude-tag", tag)
		}
	}
	if cmd.Flags().Changed("file") {
		args = append(args, "--file", opts.file)
	}
	if opts.sourceRoot != "" {
		args = append(args, "--source-root", opts.sourceRoot)
	}
	if opts.home != "" {
		args = append(args, "--home", opts.home)
	}
	if opts.stateRoot != "" {
		args = append(args, "--state-root", opts.stateRoot)
	}
	if opts.yes {
		args = append(args, "--yes", "--acknowledge-selection-change")
	}
	if opts.noTUI {
		args = append(args, "--no-tui")
	}
	if wantsJSON(cmd) {
		args = append(args, "--output", "json")
	}
	return commandHint(cmd.Root().Name(), args)
}
