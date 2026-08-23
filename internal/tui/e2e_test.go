package tui

import (
	"bytes"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/yersonargotev/dots/internal/install"
)

// TestE2EResolveConflictsFlow drives the real Bubble Tea program end to end:
// it opens a diff, navigates with vim keys, picks replace and adopt, applies,
// and asserts both the rendered output and the resolved decisions.
func TestE2EResolveConflictsFlow(t *testing.T) {
	conflicts := sampleConflicts()
	var diffCalls atomic.Int32
	diff := func(c Conflict) string {
		diffCalls.Add(1)
		return "TARGET vs SOURCE for " + c.Target
	}

	tm := teatest.NewTestModel(t, New(conflicts, diff), teatest.WithInitialTermSize(100, 40))

	// The list view renders first.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("resolve conflicts"))
	}, teatest.WithDuration(2*time.Second))

	// Open the diff for the highlighted conflict and confirm injected content shows.
	tm.Send(key('d'))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("TARGET vs SOURCE for /home/u/.gitconfig"))
	}, teatest.WithDuration(2*time.Second))

	// Close the diff, replace the first conflict, move down, adopt the second, apply.
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Send(key('r'))
	tm.Send(key('j'))
	tm.Send(key('a'))
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	final := tm.FinalModel(t).(Model)
	if final.Canceled() {
		t.Fatalf("flow ended canceled, want applied")
	}
	decisions := final.Decisions()
	if got := decisions["/home/u/.gitconfig"]; got != install.DecisionReplace {
		t.Fatalf("gitconfig decision = %q, want replace", got)
	}
	if got := decisions["/home/u/.zshrc"]; got != install.DecisionAdopt {
		t.Fatalf("zshrc decision = %q, want adopt", got)
	}
	if got := diffCalls.Load(); got != 1 {
		t.Fatalf("diff provider calls = %d, want exactly 1", got)
	}
}

// TestE2ECancelAppliesNothing proves ctrl+c aborts with no decisions applied.
func TestE2ECancelAppliesNothing(t *testing.T) {
	tm := teatest.NewTestModel(t, New(sampleConflicts(), nil), teatest.WithInitialTermSize(100, 40))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("resolve conflicts"))
	}, teatest.WithDuration(2*time.Second))

	tm.Send(key('r')) // choose a destructive action...
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	final := tm.FinalModel(t).(Model)
	if !final.Canceled() {
		t.Fatalf("ctrl+c should cancel the session")
	}
	if len(final.Decisions()) != 0 {
		t.Fatalf("canceled session must yield no decisions, got %v", final.Decisions())
	}
}

func TestE2EResizesFromStandardThroughTinyAndWide(t *testing.T) {
	tm := teatest.NewTestModel(t, New(sampleConflicts(), nil), teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("resolve conflicts"))
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.WindowSizeMsg{Width: 18, Height: 6})
	tm.Send(tea.WindowSizeMsg{Width: 160, Height: 30})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	final := tm.FinalModel(t).(Model)
	if final.width != 160 || final.height != 30 {
		t.Fatalf("final size = %dx%d, want 160x30", final.width, final.height)
	}
	if final.listViewport.Width != 160 || final.listViewport.Height <= 0 {
		t.Fatalf("list viewport not resized: %dx%d", final.listViewport.Width, final.listViewport.Height)
	}
	if final.Canceled() {
		t.Fatalf("resize flow should apply, not cancel")
	}
}
