package tagpolicy

import "testing"

func TestActionsUsesClosedPolicy(t *testing.T) {
	got := Actions([]string{"agents", "codex-delegation", "without-codex-spark-delegation", "unrelated"})
	if len(got) != 1 || got[0] != ActionRetireGentleAIState {
		t.Fatalf("Actions() = %#v, want only %q", got, ActionRetireGentleAIState)
	}
}

func TestRegistryValuesAreAllowlisted(t *testing.T) {
	for _, kind := range []string{"surface", "cleanup", "compatibility"} {
		if !IsAllowedKind(kind) {
			t.Errorf("IsAllowedKind(%q) = false, want true", kind)
		}
	}
	if IsAllowedKind("command") {
		t.Error("IsAllowedKind(command) = true, want false")
	}
	for _, status := range []string{"current", "legacy"} {
		if !IsAllowedStatus(status) {
			t.Errorf("IsAllowedStatus(%q) = false, want true", status)
		}
	}
	if IsAllowedStatus("retired") {
		t.Error("IsAllowedStatus(retired) = true, want false")
	}
}

func TestExpectedKindMatchesEachBehaviorTag(t *testing.T) {
	for tag, want := range map[string]string{"agents": "surface"} {
		got, ok := ExpectedKind(tag)
		if !ok || got != want {
			t.Errorf("ExpectedKind(%q) = %q, %t; want %q, true", tag, got, ok, want)
		}
	}
	for _, retired := range []string{"codex-delegation", "without-codex-delegation", "codex-spark-delegation", "without-codex-spark-delegation"} {
		if _, ok := ExpectedKind(retired); ok {
			t.Fatalf("ExpectedKind(%q) reports retired behavior", retired)
		}
	}
}
