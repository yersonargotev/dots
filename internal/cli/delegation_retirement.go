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

// retireHistoricalAgentState runs only after the install/update path succeeded.
// Historical Provisioner receipts are the sole migration authority; current
// Tag selection never authorizes cleanup.
func retireHistoricalAgentState(meta state.Metadata, home string) (*agentinstructions.RetirementReport, error) {
	gentleAI := hasHistoricalGentleAIEvidence(meta)
	delegation := hasHistoricalDelegationEvidence(meta)
	if !gentleAI && !delegation {
		return nil, nil
	}
	report := agentinstructions.RetirementReport{Removed: []string{}, ManualCleanup: []string{}}
	if gentleAI {
		gentleReport, err := agentinstructions.RetireGentleAIState(home)
		if err != nil {
			return nil, err
		}
		appendRetirementReport(&report, gentleReport)
	}
	if delegation {
		delegationReport, err := agentinstructions.RetireCodexDelegation(home)
		if err != nil {
			return nil, err
		}
		appendRetirementReport(&report, delegationReport)
	}
	return &report, nil
}

func hasHistoricalGentleAIEvidence(meta state.Metadata) bool {
	for _, record := range meta.Provisioners {
		if record.Tool == "gentle-ai" && record.Executable == "gentle-ai" && record.Status == "provisioned" && len(record.Args) > 0 && record.Args[0] == "install" {
			return true
		}
	}
	return false
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

func appendRetirementReport(report *agentinstructions.RetirementReport, addition agentinstructions.RetirementReport) {
	report.Removed = append(report.Removed, addition.Removed...)
	report.ManualCleanup = append(report.ManualCleanup, addition.ManualCleanup...)
}

func renderHistoricalRetirement(w io.Writer, report *agentinstructions.RetirementReport) {
	if report == nil {
		return
	}
	fmt.Fprintln(w, "Historical retirement:")
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
