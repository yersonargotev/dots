package provision

import "testing"

func TestCheckReportHasFindings(t *testing.T) {
	ready := CheckReport{Items: []Readiness{{Tool: "claude"}}}
	if ready.HasFindings() {
		t.Fatal("a Provisioner with no missing dependencies must not report findings")
	}

	notReady := CheckReport{Items: []Readiness{{Tool: "codex"}, {Tool: "claude", Missing: []string{"claude"}}}}
	if !notReady.HasFindings() {
		t.Fatal("a Provisioner missing a dependency must report findings")
	}

	if (CheckReport{}).HasFindings() {
		t.Fatal("an empty report must not report findings")
	}
}
