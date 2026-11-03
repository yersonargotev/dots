package cli

import (
	"fmt"

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

func upgradeRecoveryHint(cmd *cobra.Command, opts updateOptions, intent selection.Intent) string {
	public := len(opts.profiles)+len(opts.extraTags)+len(opts.excludedTags) > 0
	carried := intent.Source != "" || len(intent.Profiles)+len(intent.ExtraTags)+len(intent.ExcludedTags) > 0
	if public && carried {
		return fmt.Sprintf("; mixed public and internal selection (public Profiles=%q Tags=%q exclusions=%q; internal source=%q Profiles=%q Tags=%q exclusions=%q); restate one complete request with public selection flags",
			opts.profiles, opts.extraTags, opts.excludedTags, intent.Source, intent.Profiles, intent.ExtraTags, intent.ExcludedTags)
	}
	return "; restart with " + upgradeRecoveryCommand(cmd, opts, intent)
}

func upgradeRecoveryCommand(cmd *cobra.Command, opts updateOptions, intent selection.Intent) string {
	if intent.Source == "" && len(intent.Profiles)+len(intent.ExtraTags)+len(intent.ExcludedTags) == 0 {
		intent = selection.Intent{Source: selection.SourceExplicit, Profiles: opts.profiles, ExtraTags: opts.extraTags, ExcludedTags: opts.excludedTags}
	}
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
	if opts.dryRun {
		args = append(args, "--dry-run")
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
