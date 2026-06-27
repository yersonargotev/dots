package cli

import "testing"

func TestCommandHintShellQuotesUnsafeArguments(t *testing.T) {
	got := commandHint("sh", []string{"-c", "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path"})
	want := "sh -c 'curl --proto '\\''=https'\\'' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path'"
	if got != want {
		t.Fatalf("commandHint() = %q, want %q", got, want)
	}
}

func TestCommandHintLeavesSafeArgumentsReadable(t *testing.T) {
	got := commandHint("brew", []string{"install", "starship"})
	if got != "brew install starship" {
		t.Fatalf("commandHint() = %q, want brew install starship", got)
	}
}
