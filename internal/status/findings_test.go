package status

import "testing"

func TestReportHasFindings(t *testing.T) {
	cases := []struct {
		name  string
		entry Entry
		want  bool
	}{
		{"ok is not a finding", Entry{State: StateOK}, false},
		{"skipped is intentional, not a finding", Entry{State: StateSkipped}, false},
		{"missing is a finding", Entry{State: StateMissing}, true},
		{"conflict is a finding", Entry{State: StateConflict}, true},
		{"drifted is a finding", Entry{State: StateDrifted}, true},
		{"unsupported is a finding", Entry{State: StateUnsupported}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := Report{Entries: []Entry{{State: StateOK}, tc.entry}}
			if got := report.HasFindings(); got != tc.want {
				t.Fatalf("HasFindings() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReportHasFindingsEmpty(t *testing.T) {
	if (Report{}).HasFindings() {
		t.Fatal("an empty report must not report findings")
	}
}
