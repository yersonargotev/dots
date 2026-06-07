// Package tui implements the Bubble Tea conflict-resolution interface for
// dots install. The model is intentionally free of filesystem and security
// concerns: diff content is injected through DiffFunc, and the resolved
// decisions are consumed by the cli layer to drive install.Apply. This keeps
// the existing safety model (explicit conflicts, backups on replace,
// deliberate adoption, conservative non-interactive installs) unchanged.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yersonargotev/dots/internal/install"
)

// Conflict is a single conflicting target presented for resolution. It mirrors
// the fields of a plan conflict action that the user needs to decide on.
type Conflict struct {
	Target   string
	Source   string
	Strategy string
}

// DiffFunc returns human-readable preview/diff text for a conflict. The cli
// layer injects it so the TUI never reads files directly; all path-safety
// validation stays where the rest of the install safety checks live.
type DiffFunc func(Conflict) string

// Model is the Bubble Tea model for resolving install conflicts.
type Model struct {
	conflicts []Conflict
	decisions []install.ConflictDecision
	cursor    int

	diff     DiffFunc
	diffText string
	showDiff bool

	canceled bool
	quitting bool

	styles Styles
}

// New builds a conflict-resolution model. Every conflict defaults to skip so an
// abandoned session never mutates existing workstation files.
func New(conflicts []Conflict, diff DiffFunc) Model {
	decisions := make([]install.ConflictDecision, len(conflicts))
	for i := range decisions {
		decisions[i] = install.DecisionSkip
	}
	return Model{
		conflicts: conflicts,
		decisions: decisions,
		diff:      diff,
		styles:    DefaultStyles(),
	}
}

// Init quits immediately when there are no conflicts to resolve.
func (m Model) Init() tea.Cmd {
	if len(m.conflicts) == 0 {
		return tea.Quit
	}
	return nil
}

// Update advances the model in response to messages. Navigation is vim-style
// (j/k plus g/G) with arrow keys as aliases; s/r/a set the per-target decision,
// d shows the injected diff, enter confirms, and ctrl+c cancels.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		return m.handleKey(key)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		m.canceled = true
		m.quitting = true
		return m, tea.Quit
	}

	if m.showDiff {
		switch msg.String() {
		case "esc", "q", "d":
			m.showDiff = false
			m.diffText = ""
		}
		return m, nil
	}

	switch msg.String() {
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.conflicts) - 1
	case "s":
		m.setDecision(install.DecisionSkip)
	case "r":
		m.setDecision(install.DecisionReplace)
	case "a":
		m.setDecision(install.DecisionAdopt)
	case "d":
		m.openDiff()
	case "enter":
		m.quitting = true
		return m, tea.Quit
	case "q", "esc":
		m.canceled = true
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if last := len(m.conflicts) - 1; m.cursor > last {
		m.cursor = last
	}
}

func (m *Model) setDecision(d install.ConflictDecision) {
	if m.cursor < 0 || m.cursor >= len(m.decisions) {
		return
	}
	m.decisions[m.cursor] = d
}

func (m *Model) openDiff() {
	if m.diff == nil || m.cursor < 0 || m.cursor >= len(m.conflicts) {
		return
	}
	m.diffText = m.diff(m.conflicts[m.cursor])
	m.showDiff = true
}

// Decisions returns the resolved per-target decisions. Skip is omitted so the
// install layer treats any missing target as skip, matching the text-prompt
// path's map convention.
func (m Model) Decisions() map[string]install.ConflictDecision {
	out := map[string]install.ConflictDecision{}
	if m.canceled {
		// A canceled session must never yield usable decisions so the caller
		// applies nothing, regardless of what was tentatively selected.
		return out
	}
	for i, c := range m.conflicts {
		if m.decisions[i] != install.DecisionSkip {
			out[c.Target] = m.decisions[i]
		}
	}
	return out
}

// Canceled reports whether the user aborted conflict resolution. When true the
// caller must not apply any conflict decisions.
func (m Model) Canceled() bool {
	return m.canceled
}

// View renders the conflict list or the diff panel.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if len(m.conflicts) == 0 {
		return ""
	}
	if m.showDiff {
		return m.viewDiff()
	}
	return m.viewList()
}

func (m Model) viewList() string {
	var b strings.Builder
	b.WriteString(m.styles.Title.Render("dots · resolve conflicts"))
	b.WriteString("\n")
	b.WriteString(m.styles.Subtitle.Render(fmt.Sprintf("%d conflict(s) — choose an action for each target", len(m.conflicts))))
	b.WriteString("\n\n")

	for i, c := range m.conflicts {
		cursor := "  "
		if i == m.cursor {
			cursor = m.styles.Cursor.Render("▸ ")
		}
		badge := m.styles.Decision(m.decisions[i]).Render(fmt.Sprintf("[%s]", m.decisions[i]))
		line := fmt.Sprintf("%s%s %s %s",
			cursor,
			badge,
			m.styles.Target.Render(c.Target),
			m.styles.Source.Render("("+c.Strategy+" ← "+c.Source+")"),
		)
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.styles.Help.Render("j/k move · s skip · r replace · a adopt · d diff · enter apply · q cancel"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewDiff() string {
	c := m.conflicts[m.cursor]
	var b strings.Builder
	b.WriteString(m.styles.DiffTitle.Render("diff · " + c.Target))
	b.WriteString("\n\n")
	b.WriteString(m.styles.DiffBody.Render(m.diffText))
	b.WriteString("\n\n")
	b.WriteString(m.styles.Help.Render("esc/q/d close diff"))
	b.WriteString("\n")
	return b.String()
}
