package tagpolicy

import "testing"

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
