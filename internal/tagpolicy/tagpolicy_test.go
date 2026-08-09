package tagpolicy

import (
	"reflect"
	"testing"
)

func TestActionsUsesClosedPolicyAndRemovalPrecedence(t *testing.T) {
	got := Actions([]string{"agents", "codex-delegation", "without-codex-spark-delegation", "unrelated"})
	want := []Action{ActionRetireGentleAIState, ActionRemoveCodexDelegation}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Actions() = %#v, want %#v", got, want)
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
	for tag, want := range map[string]string{
		"agents":                         "surface",
		"codex-delegation":               "surface",
		"without-codex-delegation":       "cleanup",
		"codex-spark-delegation":         "compatibility",
		"without-codex-spark-delegation": "compatibility",
	} {
		got, ok := ExpectedKind(tag)
		if !ok || got != want {
			t.Errorf("ExpectedKind(%q) = %q, %t; want %q, true", tag, got, ok, want)
		}
	}
	if _, ok := ExpectedKind("unrelated"); ok {
		t.Fatal("ExpectedKind(unrelated) reports a behavior policy")
	}
}
