package cli

import (
	"fmt"
	"io"

	"github.com/yersonargotev/dots/internal/agentinstructions"
	"github.com/yersonargotev/dots/internal/state"
)

var retiredDelegationSelectors = map[string]bool{
	"codex-delegation":               true,
	"codex-spark-delegation":         true,
	"without-codex-delegation":       true,
	"without-codex-spark-delegation": true,
}

var retiredDelegationSkillsArgs = []string{
	"--yes", "skills@1.5.12", "add", "yersonargotev/dots/skills/delegation",
	"--agent", "codex", "--skill", "delegation", "--global", "--copy",
}

// retireHistoricalDelegation runs only after the install/update path succeeded.
// Current manifests cannot select the retired surface, so metadata is the sole
// migration authority.
func retireHistoricalDelegation(meta state.Metadata, home string) (*agentinstructions.RetirementReport, error) {
	if !hasHistoricalDelegationEvidence(meta) {
		return nil, nil
	}
	report, err := agentinstructions.RetireCodexDelegation(home)
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func hasHistoricalDelegationEvidence(meta state.Metadata) bool {
	if selectedHistoricalDelegation(meta.InstalledSelection) {
		return true
	}
	for _, record := range meta.Provisioners {
		if record.Tool == "skills" && record.Executable == "npx" && stringSlicesEqual(record.Args, retiredDelegationSkillsArgs) {
			return true
		}
	}
	return false
}

func selectedHistoricalDelegation(selection *state.InstalledSelection) bool {
	if selection == nil {
		return false
	}
	for _, values := range [][]string{selection.Profiles, selection.ExtraTags, selection.ResolvedTags} {
		for _, value := range values {
			if retiredDelegationSelectors[value] {
				return true
			}
		}
	}
	return false
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func renderDelegationRetirement(w io.Writer, report *agentinstructions.RetirementReport) {
	if report == nil {
		return
	}
	fmt.Fprintln(w, "Delegation retirement:")
	if len(report.Removed) == 0 {
		fmt.Fprintln(w, "  Removed: (none)")
	} else {
		for _, item := range report.Removed {
			fmt.Fprintf(w, "  Removed: %s\n", item)
		}
	}
	if len(report.ManualCleanup) == 0 {
		fmt.Fprintln(w, "  Manual cleanup: (none)")
	} else {
		for _, item := range report.ManualCleanup {
			fmt.Fprintf(w, "  Manual cleanup: %s\n", item)
		}
	}
}
