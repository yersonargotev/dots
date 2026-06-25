package cli

import "testing"

func TestDefaultInitRepositoryRefPinsReleasedBinary(t *testing.T) {
	if got, want := defaultInitRepositoryRef("", "v0.22.0"), "v0.22.0"; got != want {
		t.Fatalf("defaultInitRepositoryRef() = %q, want %q", got, want)
	}
}

func TestDefaultInitRepositoryRefLeavesDevelopmentBuildOnDefaultBranch(t *testing.T) {
	if got := defaultInitRepositoryRef("", "dev"); got != "" {
		t.Fatalf("defaultInitRepositoryRef() = %q, want empty ref for dev build", got)
	}
}

func TestDefaultInitRepositoryRefHonorsExplicitRef(t *testing.T) {
	if got, want := defaultInitRepositoryRef("main", "v0.22.0"), "main"; got != want {
		t.Fatalf("defaultInitRepositoryRef() = %q, want %q", got, want)
	}
}
