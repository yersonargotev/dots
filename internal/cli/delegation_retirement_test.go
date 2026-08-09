package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/agentinstructions"
	"github.com/yersonargotev/dots/internal/state"
)

func TestHistoricalDelegationEvidenceIsNarrow(t *testing.T) {
	exact := append([]string(nil), retiredDelegationSkillsArgs...)
	for _, tc := range []struct {
		name string
		meta state.Metadata
		want bool
	}{
		{name: "no evidence", meta: state.Metadata{InstalledSelection: &state.InstalledSelection{Profiles: []string{"agents"}, ResolvedTags: []string{"agents"}}}, want: false},
		{name: "retired selector", meta: state.Metadata{InstalledSelection: &state.InstalledSelection{ExtraTags: []string{"codex-delegation"}}}, want: true},
		{name: "exact historical skills record", meta: state.Metadata{Provisioners: []state.ProvisionerRecord{{Tool: "skills", Executable: "npx", Args: exact, Status: "failed"}}}, want: true},
		{name: "near match is not evidence", meta: state.Metadata{Provisioners: []state.ProvisionerRecord{{Tool: "skills", Executable: "npx", Args: exact[:len(exact)-1]}}}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasHistoricalDelegationEvidence(tc.meta); got != tc.want {
				t.Fatalf("hasHistoricalDelegationEvidence() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestHistoricalGentleAIEvidenceRequiresSuccessfulInstallReceipt(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta state.Metadata
		want bool
	}{
		{name: "no evidence", meta: state.Metadata{}, want: false},
		{name: "Installed Selection agents only", meta: state.Metadata{InstalledSelection: &state.InstalledSelection{Profiles: []string{"agents"}, ResolvedTags: []string{"agents"}}}, want: false},
		{name: "legacy receipt", meta: state.Metadata{Provisioners: []state.ProvisionerRecord{{Profile: "agents", Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--scope", "global"}, Status: "provisioned"}}}, want: true},
		{name: "current receipt", meta: state.Metadata{Provisioners: []state.ProvisionerRecord{{Profiles: []string{"agents"}, Tags: []string{"agents"}, Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--agents", "codex"}, Status: "provisioned"}}}, want: true},
		{name: "failed install", meta: state.Metadata{Provisioners: []state.ProvisionerRecord{{Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install"}, Status: "failed"}}}, want: false},
		{name: "missing dependency", meta: state.Metadata{Provisioners: []state.ProvisionerRecord{{Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install"}, Status: "missing-dependencies"}}}, want: false},
		{name: "uninstall receipt", meta: state.Metadata{Provisioners: []state.ProvisionerRecord{{Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"uninstall", "--yes"}, Status: "provisioned"}}}, want: false},
		{name: "wrong executable", meta: state.Metadata{Provisioners: []state.ProvisionerRecord{{Tool: "gentle-ai", Executable: "npx", Args: []string{"install"}, Status: "provisioned"}}}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasHistoricalGentleAIEvidence(tc.meta); got != tc.want {
				t.Fatalf("hasHistoricalGentleAIEvidence() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestRenderDelegationRetirementIncludesRemovedAndManualCleanup(t *testing.T) {
	var out bytes.Buffer
	renderHistoricalRetirement(&out, &agentinstructions.RetirementReport{
		Removed:       []string{"~/.codex/AGENTS.md delegation blocks"},
		ManualCleanup: []string{"~/.agents/skills/delegation"},
	})
	for _, want := range []string{"Historical retirement:", "Removed: ~/.codex/AGENTS.md delegation blocks", "Manual cleanup: ~/.agents/skills/delegation"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report missing %q:\n%s", want, out.String())
		}
	}
}
