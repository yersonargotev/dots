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

	"github.com/charmbracelet/bubbles/help"
	bubbleskey "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/yersonargotev/dots/internal/install"
	uitheme "github.com/yersonargotev/dots/internal/tui/theme"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
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

	canceled  bool
	confirmed bool
	quitting  bool

	width        int
	height       int
	listViewport viewport.Model
	diffViewport viewport.Model
	help         help.Model
	keys         conflictKeyMap
	styles       Styles
}

// New builds a conflict-resolution model. Every conflict defaults to skip so an
// abandoned session never mutates existing workstation files.
func New(conflicts []Conflict, diff DiffFunc) Model {
	return NewWithTheme(conflicts, diff, uitheme.Default())
}

// NewWithTheme builds a conflict-resolution model with an explicit theme. It
// provides a deterministic no-color path for terminals and tests that require
// it while New retains the default shared Mocha presentation.
func NewWithTheme(conflicts []Conflict, diff DiffFunc, screenTheme uitheme.Theme) Model {
	decisions := make([]install.ConflictDecision, len(conflicts))
	for i := range decisions {
		decisions[i] = install.DecisionSkip
	}

	styles := stylesFromTheme(screenTheme)
	helpModel := help.New()
	styles.Theme.ApplyHelp(&helpModel)
	keys := newConflictKeyMap(diff != nil)
	listViewport := viewport.New(0, 0)
	listViewport.MouseWheelEnabled = false
	listViewport.KeyMap = disabledViewportKeyMap()
	diffViewport := viewport.New(0, 0)
	diffViewport.KeyMap = diffViewportKeyMap(keys.diff)

	m := Model{
		conflicts:    append([]Conflict(nil), conflicts...),
		decisions:    decisions,
		diff:         diff,
		listViewport: listViewport,
		diffViewport: diffViewport,
		help:         helpModel,
		keys:         keys,
		styles:       styles,
	}
	m.setSize(defaultWidth, defaultHeight)
	return m
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
// d shows the injected diff, enter confirms, and ctrl+c cancels globally.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && bubbleskey.Matches(keyMsg, m.keys.globalCancel) {
		m.canceled = true
		m.quitting = true
		return m, tea.Quit
	}

	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.setSize(size.Width, size.Height)
	}

	if m.showDiff {
		return m.updateDiff(msg)
	}
	return m.updateList(msg)
}

func (m Model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case bubbleskey.Matches(keyMsg, m.keys.list.down):
			m.moveCursor(1)
		case bubbleskey.Matches(keyMsg, m.keys.list.up):
			m.moveCursor(-1)
		case bubbleskey.Matches(keyMsg, m.keys.list.first):
			m.cursor = 0
			m.resizeViewports()
		case bubbleskey.Matches(keyMsg, m.keys.list.last):
			m.cursor = len(m.conflicts) - 1
			m.resizeViewports()
		case bubbleskey.Matches(keyMsg, m.keys.list.skip):
			m.setDecision(install.DecisionSkip)
		case bubbleskey.Matches(keyMsg, m.keys.list.replace):
			m.setDecision(install.DecisionReplace)
		case bubbleskey.Matches(keyMsg, m.keys.list.adopt):
			m.setDecision(install.DecisionAdopt)
		case bubbleskey.Matches(keyMsg, m.keys.list.diff):
			m.openDiff()
			return m, nil
		case bubbleskey.Matches(keyMsg, m.keys.list.apply):
			m.confirmed = true
			m.quitting = true
			return m, tea.Quit
		case bubbleskey.Matches(keyMsg, m.keys.list.cancel):
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.listViewport, cmd = m.listViewport.Update(msg)
	cmds = append(cmds, cmd)
	m.help, cmd = m.help.Update(msg)
	cmds = append(cmds, cmd)
	return m, batch(cmds...)
}

func (m Model) updateDiff(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case bubbleskey.Matches(keyMsg, m.keys.diff.cancel):
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		case bubbleskey.Matches(keyMsg, m.keys.diff.close):
			m.closeDiff()
			return m, nil
		case bubbleskey.Matches(keyMsg, m.keys.diff.first):
			m.diffViewport.GotoTop()
			return m, nil
		case bubbleskey.Matches(keyMsg, m.keys.diff.last):
			m.diffViewport.GotoBottom()
			return m, nil
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.diffViewport, cmd = m.diffViewport.Update(msg)
	cmds = append(cmds, cmd)
	m.help, cmd = m.help.Update(msg)
	cmds = append(cmds, cmd)
	return m, batch(cmds...)
}

func batch(cmds ...tea.Cmd) tea.Cmd {
	filtered := cmds[:0]
	for _, cmd := range cmds {
		if cmd != nil {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return tea.Batch(filtered...)
}

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if last := len(m.conflicts) - 1; m.cursor > last {
		m.cursor = last
	}
	m.resizeViewports()
}

func (m *Model) setDecision(d install.ConflictDecision) {
	if m.cursor < 0 || m.cursor >= len(m.decisions) {
		return
	}
	m.decisions[m.cursor] = d
	m.resizeViewports()
}

func (m *Model) openDiff() {
	if m.diff == nil || m.cursor < 0 || m.cursor >= len(m.conflicts) {
		return
	}
	m.diffText = m.diff(m.conflicts[m.cursor])
	m.showDiff = true
	m.diffViewport.SetContent(m.diffContent())
	m.diffViewport.GotoTop()
	m.resizeViewports()
}

func (m *Model) closeDiff() {
	m.showDiff = false
	m.diffText = ""
	m.diffViewport.SetContent("")
	m.diffViewport.GotoTop()
	m.resizeViewports()
}

// Decisions returns the current per-target decision draft. Skip is omitted so
// the install layer treats any missing target as skip, matching the text-prompt
// path's map convention. ResolveConflicts separately requires explicit list
// confirmation before this draft can become installation authority.
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
	if m.quitting || len(m.conflicts) == 0 {
		return ""
	}
	var content string
	if m.showDiff {
		content = m.viewDiff()
	} else {
		content = m.viewList()
	}
	return m.styles.Theme.Paint(content, m.width, m.height)
}

func (m Model) viewList() string {
	if m.compactListLayout() {
		return m.compactListView()
	}
	sections := []string{
		m.listHeader(),
		m.listViewport.View(),
		m.listStatus(),
		m.consequences(),
		m.help.View(m.keys.list),
	}
	return strings.Join(sections, "\n")
}

func (m Model) viewDiff() string {
	if m.compactDiffLayout() {
		return m.compactDiffView()
	}
	sections := []string{
		m.diffHeader(),
		m.diffViewport.View(),
		m.diffStatus(),
		m.help.View(m.keys.diff),
	}
	return strings.Join(sections, "\n")
}

func (m Model) listHeader() string {
	title := m.wrap(m.styles.Theme.Title.Render("dots | resolve conflicts"))
	summary := m.wrap(m.styles.Theme.Secondary.Render(fmt.Sprintf("%d conflict(s) | choose one action for each target", len(m.conflicts))))
	return title + "\n" + summary
}

func (m Model) diffHeader() string {
	title := m.wrap(m.styles.Theme.Title.Render("dots | conflict diff"))
	if m.cursor < 0 || m.cursor >= len(m.conflicts) {
		return title
	}
	c := m.conflicts[m.cursor]
	summary := m.wrap(m.styles.Theme.Secondary.Render(c.Target + " (" + c.Strategy + " <- " + c.Source + ")"))
	return title + "\n" + summary
}

func (m Model) listStatus() string {
	position := 0
	if len(m.conflicts) > 0 {
		position = m.cursor + 1
	}
	skip, replace, adopt := m.decisionCounts()
	return m.wrap(m.styles.Theme.Focus.Render(fmt.Sprintf(
		"Conflict %d/%d | tentative: skip %d | replace %d | adopt %d",
		position, len(m.conflicts), skip, replace, adopt,
	)))
}

func (m Model) diffStatus() string {
	total := m.diffViewport.TotalLineCount()
	start, end := 0, 0
	if total > 0 {
		start = min(m.diffViewport.YOffset+1, total)
		end = min(m.diffViewport.YOffset+m.diffViewport.VisibleLineCount(), total)
	}
	return m.wrap(m.styles.Theme.Focus.Render(fmt.Sprintf(
		"Diff lines %d-%d/%d | %d%%",
		start, end, total, int(m.diffViewport.ScrollPercent()*100),
	)))
}

func (m Model) consequences() string {
	return strings.Join([]string{
		m.consequence(install.DecisionSkip),
		m.consequence(install.DecisionReplace),
		m.consequence(install.DecisionAdopt),
	}, "\n")
}

func (m Model) activeConsequence() string {
	if m.cursor < 0 || m.cursor >= len(m.decisions) {
		return ""
	}
	return m.consequence(m.decisions[m.cursor])
}

func (m Model) consequence(decision install.ConflictDecision) string {
	switch decision {
	case install.DecisionReplace:
		return m.wrapConsequence(m.styles.Theme.Warning.Render("replace backs it up and installs the Source of Truth"))
	case install.DecisionAdopt:
		return m.wrapConsequence(m.styles.Theme.Caution.Render("adopt copies supported local regular-file content into the Source of Truth"))
	default:
		return m.wrapConsequence(m.styles.Theme.Secondary.Render("skip leaves the local target untouched"))
	}
}

func (m Model) decisionCounts() (skip, replace, adopt int) {
	for _, decision := range m.decisions {
		switch decision {
		case install.DecisionReplace:
			replace++
		case install.DecisionAdopt:
			adopt++
		default:
			skip++
		}
	}
	return skip, replace, adopt
}

func (m *Model) setSize(width, height int) {
	m.width = uitheme.Clamp(width)
	m.height = uitheme.Clamp(height)
	m.help.Width = m.width
	// Standard and wide terminals can show the complete generated key map;
	// narrow/tiny screens keep Bubbles' single-line, width-aware help.
	m.help.ShowAll = m.width >= 72 && m.height >= 16
	m.resizeViewports()
}

func (m *Model) resizeViewports() {
	if m.width == 0 || m.height == 0 {
		m.listViewport.Width = 0
		m.listViewport.Height = 0
		m.diffViewport.Width = 0
		m.diffViewport.Height = 0
		return
	}

	listReserved := sectionHeight(m.listHeader()) + sectionHeight(m.listStatus()) +
		sectionHeight(m.consequences()) + sectionHeight(m.help.View(m.keys.list))
	diffReserved := sectionHeight(m.diffHeader()) + sectionHeight(m.diffStatus()) +
		sectionHeight(m.help.View(m.keys.diff))

	m.listViewport.Width, m.listViewport.Height = uitheme.InnerSize(m.width, m.height, 0, listReserved)
	m.diffViewport.Width, m.diffViewport.Height = uitheme.InnerSize(m.width, m.height, 0, diffReserved)
	if listReserved >= m.height && m.height >= 1 {
		m.listViewport.Height = m.compactListPlan().viewportHeight
	}
	if diffReserved >= m.height && m.height >= 2 {
		m.diffViewport.Height = max(1, m.height-m.compactFixedHeight(m.keys.diff))
	}
	m.syncListContent()
	m.diffViewport.SetContent(m.diffContent())
}

func sectionHeight(s string) int {
	if s == "" {
		return 0
	}
	return lipgloss.Height(s)
}

func (m Model) compactListLayout() bool {
	fixed := sectionHeight(m.listHeader()) + sectionHeight(m.listStatus()) +
		sectionHeight(m.consequences()) + sectionHeight(m.help.View(m.keys.list))
	return fixed >= m.height
}

func (m Model) compactDiffLayout() bool {
	fixed := sectionHeight(m.diffHeader()) + sectionHeight(m.diffStatus()) +
		sectionHeight(m.help.View(m.keys.diff))
	return fixed >= m.height
}

func (m Model) compactListView() string {
	plan := m.compactListPlan()
	sections := make([]string, 0, 5)
	if plan.showHeader {
		sections = append(sections, oneLine(m.listHeader()))
	}
	sections = append(sections, m.listViewport.View())
	if plan.showStatus {
		sections = append(sections, oneLine(m.listStatus()))
	}
	if plan.showConsequence {
		sections = append(sections, m.activeConsequence())
	}
	if plan.showFooter {
		sections = append(sections, m.help.View(m.keys.list))
	}
	return strings.Join(sections, "\n")
}

func (m Model) compactDiffView() string {
	return m.compactView(m.diffHeader(), m.diffViewport.View(), m.diffStatus(), m.help.View(m.keys.diff))
}

type compactListLayoutPlan struct {
	viewportHeight  int
	showHeader      bool
	showStatus      bool
	showConsequence bool
	showFooter      bool
}

func (m Model) compactListPlan() compactListLayoutPlan {
	plan := compactListLayoutPlan{viewportHeight: min(1, m.height)}
	if m.height <= 1 {
		return plan
	}

	consequenceHeight := sectionHeight(m.activeConsequence())
	if consequenceHeight > m.height-1 {
		// At physically smaller sizes, keep the selected conflict usable and
		// let the bounded canvas show as much consequence text as can fit.
		plan.showConsequence = true
		return plan
	}

	plan.showConsequence = true
	used := plan.viewportHeight + consequenceHeight
	helpHeight := sectionHeight(m.help.View(m.keys.list))
	if helpHeight > 0 && used+helpHeight <= m.height {
		plan.showFooter = true
		used += helpHeight
	}
	if used < m.height {
		plan.showStatus = true
		used++
	}
	if used < m.height {
		plan.showHeader = true
		used++
	}
	plan.viewportHeight += m.height - used
	return plan
}

func (m Model) compactView(header, primary, status, footer string) string {
	if m.height <= 0 {
		return ""
	}
	if m.height == 1 {
		return oneLine(footer)
	}
	sections := []string{primary, footer}
	if m.height >= 3 {
		sections = append([]string{oneLine(header)}, sections...)
	}
	if m.height >= 4 {
		sections = append(sections[:len(sections)-1], oneLine(status), sections[len(sections)-1])
	}
	return strings.Join(sections, "\n")
}

func (m Model) compactFixedHeight(keys help.KeyMap) int {
	helpHeight := sectionHeight(m.help.View(keys))
	if m.height <= 1 {
		return helpHeight
	}
	fixed := helpHeight
	if m.height >= 3 {
		fixed++
	}
	if m.height >= 4 {
		fixed++
	}
	return fixed
}

func (m Model) wrap(value string) string {
	return uitheme.Wrap(value, m.width)
}

func (m Model) wrapConsequence(value string) string {
	if m.width <= 0 {
		return ""
	}
	return ansi.Wrap(value, m.width, "")
}

func oneLine(value string) string {
	line, _, _ := strings.Cut(value, "\n")
	return line
}

func (m *Model) syncListContent() {
	var b strings.Builder
	for i, conflict := range m.conflicts {
		cursor := "  "
		rowStyle := m.styles.Theme.Body
		if i == m.cursor {
			cursor = m.styles.Theme.Glyphs.Cursor + " "
			rowStyle = m.styles.Theme.Selected
		}
		badge := m.styles.Decision(m.decisions[i]).Render(fmt.Sprintf("[%s]", m.decisions[i]))
		line := fmt.Sprintf("%s%s %s (%s <- %s)", cursor, badge, conflict.Target, conflict.Strategy, conflict.Source)
		b.WriteString(rowStyle.Render(line))
		if i < len(m.conflicts)-1 {
			b.WriteByte('\n')
		}
	}
	m.listViewport.SetContent(b.String())
	m.ensureCursorVisible()
}

func (m *Model) ensureCursorVisible() {
	if m.listViewport.Height <= 0 || len(m.conflicts) == 0 {
		m.listViewport.SetYOffset(0)
		return
	}
	if m.cursor < m.listViewport.YOffset {
		m.listViewport.SetYOffset(m.cursor)
		return
	}
	if m.cursor >= m.listViewport.YOffset+m.listViewport.Height {
		m.listViewport.SetYOffset(m.cursor - m.listViewport.Height + 1)
	}
}

func (m Model) diffContent() string {
	if !m.showDiff {
		return ""
	}
	if m.diffText == "" {
		return m.styles.Theme.Secondary.Render("(no diff output)")
	}
	return m.styles.Theme.Body.Render(m.diffText)
}
