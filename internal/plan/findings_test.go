package plan

import "testing"

func TestPlanHasFindings(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		want   bool
	}{
		{"create is not a finding", StatusCreate, false},
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
