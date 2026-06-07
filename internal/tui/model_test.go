package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yersonargotev/dots/internal/install"
)

func key(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func sampleConflicts() []Conflict {
	return []Conflict{
		{Target: "/home/u/.gitconfig", Source: "configs/git/gitconfig", Strategy: "copy"},
		{Target: "/home/u/.zshrc", Source: "configs/zsh/zshrc", Strategy: "symlink"},
	}
}

func update(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestNewDefaultsEveryConflictToSkip(t *testing.T) {
	m := New(sampleConflicts(), nil)
	decisions := m.Decisions()
	if len(decisions) != 0 {
		t.Fatalf("Decisions() = %v, want empty map so missing targets default to skip", decisions)
	}
	if m.Canceled() {
		t.Fatalf("new model should not be canceled")
	}
	// Guard against a default other than skip leaking through the skip filter:
	// every per-conflict decision must literally be skip, not merely absent.
	for i, d := range m.decisions {
		if d != install.DecisionSkip {
			t.Fatalf("decisions[%d] = %q, want skip", i, d)
		}
	}
}

func TestListViewQuitKeysCancelWithoutApplyingDecisions(t *testing.T) {
	for _, k := range []rune{'q'} {
		t.Run("rune_"+string(k), func(t *testing.T) {
			m := New(sampleConflicts(), nil)
			m = update(t, m, key('r')) // choose a destructive decision first
			m = update(t, m, key(k))
			if !m.Canceled() {
				t.Fatalf("%q in list view should cancel the session", k)
			}
			if len(m.Decisions()) != 0 {
				t.Fatalf("canceled session must apply nothing, got %v", m.Decisions())
			}
		})
	}
	t.Run("esc", func(t *testing.T) {
		m := New(sampleConflicts(), nil)
		m = update(t, m, key('r'))
		m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		if !m.Canceled() {
			t.Fatalf("esc in list view should cancel the session")
		}
		if len(m.Decisions()) != 0 {
			t.Fatalf("canceled session must apply nothing, got %v", m.Decisions())
		}
	})
}

func TestVimNavigationMovesCursorWithinBounds(t *testing.T) {
	m := New(sampleConflicts(), nil)
	// k at top stays at top.
	m = update(t, m, key('k'))
	if m.cursor != 0 {
		t.Fatalf("cursor = %d after k at top, want 0", m.cursor)
	}
	// j moves down.
	m = update(t, m, key('j'))
	if m.cursor != 1 {
		t.Fatalf("cursor = %d after j, want 1", m.cursor)
	}
	// j at bottom stays at bottom.
	m = update(t, m, key('j'))
	if m.cursor != 1 {
		t.Fatalf("cursor = %d after j at bottom, want 1", m.cursor)
	}
	// G jumps to last, g jumps to first.
	m = update(t, m, key('g'))
	if m.cursor != 0 {
		t.Fatalf("cursor = %d after g, want 0", m.cursor)
	}
	m = update(t, m, key('G'))
	if m.cursor != 1 {
		t.Fatalf("cursor = %d after G, want 1", m.cursor)
	}
}

func TestArrowKeysAlsoNavigate(t *testing.T) {
	m := New(sampleConflicts(), nil)
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d after down, want 1", m.cursor)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Fatalf("cursor = %d after up, want 0", m.cursor)
	}
}

func TestDecisionKeysSetPerTargetDecision(t *testing.T) {
	m := New(sampleConflicts(), nil)
	// Replace the first conflict, adopt the second.
	m = update(t, m, key('r'))
	m = update(t, m, key('j'), key('a'))

	decisions := m.Decisions()
	if got := decisions["/home/u/.gitconfig"]; got != install.DecisionReplace {
		t.Fatalf("gitconfig decision = %q, want replace", got)
	}
	if got := decisions["/home/u/.zshrc"]; got != install.DecisionAdopt {
		t.Fatalf("zshrc decision = %q, want adopt", got)
	}
}

func TestSkipDecisionIsOmittedFromResult(t *testing.T) {
	m := New(sampleConflicts(), nil)
	// Pick replace then explicitly switch back to skip.
	m = update(t, m, key('r'), key('s'))
	decisions := m.Decisions()
	if _, ok := decisions["/home/u/.gitconfig"]; ok {
		t.Fatalf("skip decision must be omitted so install treats missing as skip, got %v", decisions)
	}
}

func TestDiffViewToggles(t *testing.T) {
	called := 0
	diff := func(c Conflict) string {
		called++
		return "DIFF for " + c.Target
	}
	m := New(sampleConflicts(), diff)
	m = update(t, m, key('d'))
	if !m.showDiff {
		t.Fatalf("showDiff = false after d, want true")
	}
	if called != 1 {
		t.Fatalf("diff provider called %d times, want 1", called)
	}
	if m.diffText != "DIFF for /home/u/.gitconfig" {
		t.Fatalf("diffText = %q, want diff for highlighted conflict", m.diffText)
	}
	// esc closes the diff and returns to the list.
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showDiff {
		t.Fatalf("showDiff = true after esc, want false")
	}
}

func TestEnterConfirmsAndQuits(t *testing.T) {
	m := New(sampleConflicts(), nil)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.quitting {
		t.Fatalf("quitting = false after enter, want true")
	}
	if m.Canceled() {
		t.Fatalf("enter should confirm, not cancel")
	}
	if cmd == nil {
		t.Fatalf("enter should return a quit command")
	}
}

func TestCtrlCCancels(t *testing.T) {
	m := New(sampleConflicts(), nil)
	m = update(t, m, key('r')) // even with a chosen decision, ctrl+c cancels
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(Model)
	if !m.Canceled() {
		t.Fatalf("ctrl+c should cancel")
	}
	if !m.quitting {
		t.Fatalf("ctrl+c should quit")
	}
	if cmd == nil {
		t.Fatalf("ctrl+c should return a quit command")
	}
}

func TestNoConflictsModelQuitsImmediately(t *testing.T) {
	m := New(nil, nil)
	if cmd := m.Init(); cmd == nil {
		t.Fatalf("empty model Init should request quit")
	}
}
