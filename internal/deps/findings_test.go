package deps

import "testing"

func TestCheckReportHasFindings(t *testing.T) {
	present := CheckReport{Results: []Result{{Name: "git", Present: true}}}
	if present.HasFindings() {
		t.Fatal("all-present report must not report findings")
	}

	missing := CheckReport{Results: []Result{{Name: "git", Present: true}, {Name: "rg", Present: false}}}
	if !missing.HasFindings() {
		t.Fatal("a report with an absent Dependency must report findings")
	}

	warned := CheckReport{Results: []Result{{Name: "git", Present: true, Warning: "git is present but the toolchain is broken"}}}
	if !warned.HasFindings() {
		t.Fatal("a present Dependency with a probe warning must report findings")
	}

	if (CheckReport{}).HasFindings() {
		t.Fatal("an empty report must not report findings")
	}
}

func TestPlanReportHasFindings(t *testing.T) {
	if (PlanReport{}).HasFindings() {
		t.Fatal("a plan with no missing Dependencies must not report findings")
	}

	withGuidance := PlanReport{Items: []Guidance{{Name: "rg"}}}
	if !withGuidance.HasFindings() {
		t.Fatal("a plan that lists a missing Dependency must report findings")
	}
}
