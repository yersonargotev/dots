package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yersonargotev/dots/internal/install"
	uitheme "github.com/yersonargotev/dots/internal/tui/theme"
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

func TestListViewExplainsDecisionTradeoffs(t *testing.T) {
	m := New(sampleConflicts(), nil)
	view := m.View()
	for _, want := range []string{
		"skip keeps the local file untouched",
		"replace backs up then installs the Source of Truth",
		"adopt copies supported regular-file local content into the Source of Truth",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q\nview:\n%s", want, view)
		}
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

func TestWindowSizeBoundsEveryRenderAndKeepsHelpAtTheFooter(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "zero", width: 0, height: 0},
		{name: "single_cell", width: 1, height: 1},
		{name: "tiny", width: 18, height: 6},
		{name: "standard", width: 80, height: 24},
		{name: "wide", width: 180, height: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewWithTheme(sampleConflicts(), nil, uitheme.NoColor())
			m = update(t, m, tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			view := m.View()
			if tt.width == 0 || tt.height == 0 {
				if view != "" {
					t.Fatalf("View() = %q at zero size, want empty", view)
				}
				return
			}

			lines := strings.Split(view, "\n")
			if len(lines) != tt.height {
				t.Fatalf("rendered height = %d, want %d\n%s", len(lines), tt.height, view)
			}
			for i, line := range lines {
				if got := uitheme.Width(line); got != tt.width {
					t.Fatalf("line %d width = %d, want %d: %q", i, got, tt.width, line)
				}
			}
			if tt.width >= 18 && (!strings.Contains(view, "up/k") || strings.TrimSpace(lines[len(lines)-1]) == "") {
				t.Fatalf("footer help not kept at the bottom at %dx%d:\n%s", tt.width, tt.height, view)
			}
			if tt.width == 80 && tt.height == 24 && !strings.Contains(view, "adopt copies supported regular-file local content into the Source of Truth") {
				t.Fatalf("80x24 view lost consequence copy:\n%s", view)
			}
		})
	}
}

func TestListViewportScrollsToKeepSelectedConflictVisible(t *testing.T) {
	conflicts := make([]Conflict, 12)
	for i := range conflicts {
		conflicts[i] = Conflict{
			Target:   fmt.Sprintf("/home/u/item-%02d", i),
			Source:   fmt.Sprintf("source/item-%02d", i),
			Strategy: "copy",
		}
	}
	m := NewWithTheme(conflicts, nil, uitheme.NoColor())
	m = update(t, m, tea.WindowSizeMsg{Width: 60, Height: 12}, key('G'))
	if m.cursor != len(conflicts)-1 {
		t.Fatalf("cursor = %d, want last row", m.cursor)
	}
	if m.listViewport.YOffset == 0 {
		t.Fatalf("list viewport did not scroll to the last selected row")
	}
	if !strings.Contains(m.listViewport.View(), "/home/u/item-11") {
		t.Fatalf("selected row is not visible:\n%s", m.listViewport.View())
	}

	m = update(t, m, key('g'))
	if m.listViewport.YOffset != 0 || !strings.Contains(m.listViewport.View(), "/home/u/item-00") {
		t.Fatalf("first row is not visible after g: offset=%d\n%s", m.listViewport.YOffset, m.listViewport.View())
	}
}

func TestListStatusReportsPositionAndTentativeDecisionCounts(t *testing.T) {
	conflicts := append(sampleConflicts(), Conflict{Target: "/home/u/.tmux.conf", Source: "configs/tmux/tmux.conf", Strategy: "copy"})
	m := NewWithTheme(conflicts, nil, uitheme.NoColor())
	m = update(t, m, key('r'), key('j'), key('a'))
	view := m.View()
	if want := "Conflict 2/3 | tentative: skip 1 | replace 1 | adopt 1"; !strings.Contains(view, want) {
		t.Fatalf("view missing %q:\n%s", want, view)
	}
}

func TestGeneratedHelpShowsActiveAliases(t *testing.T) {
	m := NewWithTheme(sampleConflicts(), func(Conflict) string { return "diff" }, uitheme.NoColor())
	m = update(t, m, tea.WindowSizeMsg{Width: 320, Height: 24})
	listView := m.View()
	listHelp := strings.Join(strings.Fields(listView), " ")
	for _, want := range []string{
		"up/k up",
		"down/j down",
		"home/g first",
		"end/G last",
		"enter apply",
		"q/esc cancel",
		"ctrl+c cancel",
	} {
		if !strings.Contains(listHelp, want) {
			t.Fatalf("list help missing %q:\n%s", want, listView)
		}
	}

	m = update(t, m, key('d'))
	diffView := m.View()
	diffHelp := strings.Join(strings.Fields(diffView), " ")
	for _, want := range []string{
		"up/k line up",
		"down/j line down",
		"pgup/b page up",
		"pgdn/f page down",
		"home/g top",
		"end/G bottom",
		"q/esc/d close",
		"ctrl+c cancel",
	} {
		if !strings.Contains(diffHelp, want) {
			t.Fatalf("diff help missing %q:\n%s", want, diffView)
		}
	}
}

func TestNoColorViewKeepsMeaningWithoutANSIOrUnicodeOnlyCues(t *testing.T) {
	m := NewWithTheme(sampleConflicts(), nil, uitheme.NoColor())
	view := m.View()
	if strings.Contains(view, "\x1b[") {
		t.Fatalf("no-color view contains ANSI escapes: %q", view)
	}
	for _, r := range view {
		if r > 127 {
			t.Fatalf("no-color view contains non-ASCII cue %q", r)
		}
	}
	for _, want := range []string{"> [skip]", "[skip]", "skip keeps", "replace backs up", "adopt copies"} {
		if !strings.Contains(view, want) {
			t.Fatalf("no-color view missing textual/glyph meaning %q:\n%s", want, view)
		}
	}
}

func TestDiffViewportUsesInjectedProviderOnceAndSupportsNavigation(t *testing.T) {
	called := 0
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("diff line %02d", i+1)
	}
	m := NewWithTheme(sampleConflicts(), func(c Conflict) string {
		called++
		return strings.Join(lines, "\n")
	}, uitheme.NoColor())
	m = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 12})

	// Resizing and rendering must never request diff content.
	_ = m.View()
	if called != 0 {
		t.Fatalf("diff provider called before d: %d", called)
	}
	m = update(t, m, key('d'))
	if called != 1 {
		t.Fatalf("diff provider calls after d = %d, want 1", called)
	}
	if m.diffViewport.YOffset != 0 {
		t.Fatalf("new diff offset = %d, want top", m.diffViewport.YOffset)
	}

	m = update(t, m, key('j'))
	if m.diffViewport.YOffset != 1 {
		t.Fatalf("offset after line down = %d, want 1", m.diffViewport.YOffset)
	}
	m = update(t, m, key('k'))
	if m.diffViewport.YOffset != 0 {
		t.Fatalf("offset after line up = %d, want 0", m.diffViewport.YOffset)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = update(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.diffViewport.YOffset <= 1 {
		t.Fatalf("page down did not advance diff: offset=%d", m.diffViewport.YOffset)
	}
	afterPageDown := m.diffViewport.YOffset
	m = update(t, m, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.diffViewport.YOffset >= afterPageDown {
		t.Fatalf("page up did not move toward top: before=%d after=%d", afterPageDown, m.diffViewport.YOffset)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if !m.diffViewport.AtBottom() || !strings.Contains(m.View(), "100%") {
		t.Fatalf("end did not reach bottom: offset=%d\n%s", m.diffViewport.YOffset, m.View())
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyHome})
	if !m.diffViewport.AtTop() || !strings.Contains(m.View(), "Diff lines 1-") {
		t.Fatalf("home did not return to top: offset=%d\n%s", m.diffViewport.YOffset, m.View())
	}

	m = update(t, m, key('q'))
	if m.showDiff || m.Canceled() {
		t.Fatalf("q should close diff without canceling the list")
	}
	if called != 1 {
		t.Fatalf("closing or navigating called provider again: %d", called)
	}
}

func TestDiffWindowSizeBoundsEveryRenderWithoutCallingProviderAgain(t *testing.T) {
	called := 0
	m := NewWithTheme(sampleConflicts(), func(Conflict) string {
		called++
		return strings.Repeat("long diff line\n", 30)
	}, uitheme.NoColor())
	m = update(t, m, key('d'))

	for _, size := range []tea.WindowSizeMsg{
		{Width: 0, Height: 0},
		{Width: 18, Height: 6},
		{Width: 80, Height: 24},
		{Width: 180, Height: 30},
	} {
		m = update(t, m, size)
		view := m.View()
		if size.Width == 0 || size.Height == 0 {
			if view != "" {
				t.Fatalf("zero-sized diff view = %q, want empty", view)
			}
			continue
		}
		lines := strings.Split(view, "\n")
		if len(lines) != size.Height {
			t.Fatalf("diff height at %dx%d = %d, want %d", size.Width, size.Height, len(lines), size.Height)
		}
		for i, line := range lines {
			if got := uitheme.Width(line); got != size.Width {
				t.Fatalf("diff line %d width at %dx%d = %d", i, size.Width, size.Height, got)
			}
		}
	}
	if called != 1 {
		t.Fatalf("provider calls after diff resizes = %d, want 1", called)
	}
}

func TestCtrlCCancelsBeforeActiveDiffHandlesTheKey(t *testing.T) {
	m := New(sampleConflicts(), func(Conflict) string { return "diff" })
	m = update(t, m, key('r'), key('d'))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(Model)
	if cmd == nil || !m.Canceled() || !m.quitting {
		t.Fatalf("ctrl+c in diff must globally cancel and quit")
	}
	if decisions := m.Decisions(); len(decisions) != 0 {
		t.Fatalf("ctrl+c in diff leaked tentative decisions: %v", decisions)
	}
}

func TestDiffCloseAliasesReturnToList(t *testing.T) {
	closeKeys := []tea.KeyMsg{{Type: tea.KeyEsc}, key('q'), key('d')}
	for _, closeKey := range closeKeys {
		t.Run(closeKey.String(), func(t *testing.T) {
			m := New(sampleConflicts(), func(Conflict) string { return "diff" })
			m = update(t, m, key('d'), closeKey)
			if m.showDiff || m.Canceled() {
				t.Fatalf("%s should close diff without canceling", closeKey.String())
			}
		})
	}
}
