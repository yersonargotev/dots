package plan

import (
	"testing"

	"github.com/yersonargotev/dots/internal/selectionreconciliation"
)

func TestPlanHasFindings(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		want   bool
	}{
		{"create is not a finding", StatusCreate, false},
		{"update is not a finding", StatusUpdate, false},
		{"migrate is not a finding", StatusMigrate, false},
		{"unchanged is not a finding", StatusUnchanged, false},
		{"conflict is a finding", StatusConflict, true},
		{"missing-source is a finding", StatusMissingSource, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Plan{Actions: []Action{{Status: StatusCreate}, {Status: tc.status}}}
			if got := p.HasFindings(); got != tc.want {
				t.Fatalf("HasFindings() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlanHasFindingsEmpty(t *testing.T) {
	if (Plan{}).HasFindings() {
		t.Fatal("an empty plan must not report findings")
	}
}

func TestPlanHasFindingsIncludesSelectionReconciliation(t *testing.T) {
	clean := &selectionreconciliation.Report{Actions: []selectionreconciliation.Action{{Outcome: selectionreconciliation.OutcomeRetainedExternalState}}}
	if (Plan{SelectionReconciliation: clean}).HasFindings() {
		t.Fatal("Retained External State must not be a finding")
	}
	blocked := &selectionreconciliation.Report{Actions: []selectionreconciliation.Action{{Outcome: selectionreconciliation.OutcomeBlocked}}}
	if !(Plan{SelectionReconciliation: blocked}).HasFindings() {
		t.Fatal("blocked reconciliation action must be a finding")
	}
}
