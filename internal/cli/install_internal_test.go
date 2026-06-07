package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/tui"
)

// TestConflictDiffProviderRendersValidatedContent verifies the Conflict-to-Action
// mapping used by the TUI diff: the provider must look up the correct action by
// target and render the path-safety-validated target and source contents. This
// is the deterministic counterpart to the Bubble Tea renderer, whose frame
// throttling makes asserting on rendered output flaky.
func TestConflictDiffProviderRendersValidatedContent(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()

	gitTarget := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(gitTarget, []byte("local-git-line\n"), 0o600); err != nil {
		t.Fatalf("write git target: %v", err)
	}
	zshTarget := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(zshTarget, []byte("local-zsh-line\n"), 0o600); err != nil {
		t.Fatalf("write zsh target: %v", err)
	}
	writeCLISourceInternal(t, sourceRoot, "configs/git/gitconfig", "managed-git-line\n")
	writeCLISourceInternal(t, sourceRoot, "configs/zsh/zshrc", "managed-zsh-line\n")

	actions := []plan.Action{
		{Status: plan.StatusConflict, Target: gitTarget, Source: "configs/git/gitconfig", Strategy: "copy"},
		{Status: plan.StatusConflict, Target: zshTarget, Source: "configs/zsh/zshrc", Strategy: "copy"},
	}

	diff := conflictDiffProvider(actions, home, sourceRoot)

	// Each conflict must resolve to ITS OWN action, not another target's.
	gitDiff := diff(tui.Conflict{Target: gitTarget, Source: "configs/git/gitconfig", Strategy: "copy"})
	if !strings.Contains(gitDiff, "local-git-line") || !strings.Contains(gitDiff, "managed-git-line") {
		t.Fatalf("git diff missing expected content:\n%s", gitDiff)
	}
	if strings.Contains(gitDiff, "zsh") {
		t.Fatalf("git diff leaked the zsh action's content:\n%s", gitDiff)
	}

	zshDiff := diff(tui.Conflict{Target: zshTarget, Source: "configs/zsh/zshrc", Strategy: "copy"})
	if !strings.Contains(zshDiff, "local-zsh-line") || !strings.Contains(zshDiff, "managed-zsh-line") {
		t.Fatalf("zsh diff missing expected content:\n%s", zshDiff)
	}
}

func writeCLISourceInternal(t *testing.T, sourceRoot, rel, content string) {
	t.Helper()
	path := filepath.Join(sourceRoot, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
}
