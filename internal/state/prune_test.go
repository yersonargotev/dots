package state_test

import (
	"testing"

	"github.com/yersonargotev/dots/internal/state"
)

func TestRemoveDropsMatchingTargets(t *testing.T) {
	meta := state.Metadata{Version: 1, Entries: []state.Record{
		{Target: "/home/user/.zshrc", Strategy: "symlink"},
		{Target: "/home/user/.gitconfig", Strategy: "copy"},
		{Target: "/home/user/.tmux.conf", Strategy: "symlink"},
	}}

	got := meta.Remove("/home/user/.zshrc", "/home/user/.tmux.conf")

	if got.Version != 1 {
		t.Fatalf("Remove() version = %d, want 1 preserved", got.Version)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("Remove() entries = %d, want 1", len(got.Entries))
	}
	if got.Entries[0].Target != "/home/user/.gitconfig" {
		t.Fatalf("Remove() kept = %q, want /home/user/.gitconfig", got.Entries[0].Target)
	}
}

func TestRemoveLeavesUnmatchedMetadataUnchanged(t *testing.T) {
	meta := state.Metadata{Version: 1, Entries: []state.Record{
		{Target: "/home/user/.zshrc", Strategy: "symlink"},
	}}

	got := meta.Remove("/home/user/.missing")

	if len(got.Entries) != 1 {
		t.Fatalf("Remove() entries = %d, want 1 when nothing matches", len(got.Entries))
	}
	if got.Entries[0].Target != "/home/user/.zshrc" {
		t.Fatalf("Remove() entry = %q, want unchanged", got.Entries[0].Target)
	}
}

func TestRemoveDoesNotMutateReceiver(t *testing.T) {
	meta := state.Metadata{Version: 1, Entries: []state.Record{
		{Target: "/home/user/.zshrc", Strategy: "symlink"},
		{Target: "/home/user/.gitconfig", Strategy: "copy"},
	}}

	_ = meta.Remove("/home/user/.zshrc")

	if len(meta.Entries) != 2 {
		t.Fatalf("Remove() mutated receiver: entries = %d, want 2", len(meta.Entries))
	}
}

func TestRemoveAllReturnsEmptyEntries(t *testing.T) {
	meta := state.Metadata{Version: 1, Entries: []state.Record{
		{Target: "/home/user/.zshrc", Strategy: "symlink"},
	}}

	got := meta.Remove("/home/user/.zshrc")

	if len(got.Entries) != 0 {
		t.Fatalf("Remove() entries = %d, want 0 when all removed", len(got.Entries))
	}
}
