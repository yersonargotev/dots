// Package tagselector implements the interactive Tag selection model used by
// installation flows. It owns draft intent only; callers inject preview work.
package tagselector

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yersonargotev/dots/internal/tui/theme"
)

// State is an observed selector component state.
type State string

const (
	StateAligned       State = "aligned"
	StateMissing       State = "missing"
	StateDrift         State = "drift"
	StateConflict      State = "conflict"
	StateNotApplicable State = "not-applicable"
)

// Component describes one observed part of a Tag's selected surface.
type Component struct {
	Kind   string
	Name   string
	State  State
	Detail string
}

// Tag is one atomic selectable capability in canonical browse order.
type Tag struct {
	Name                   string
	Description            string
	Group                  string
	Profiles               []string
	ManagedEntries         []string
	Dependencies           []string
	Provisioners           []string
	Components             []Component
	State                  State
	ExternalEffectsPresent bool
}

// Profile is a named preset of Tags.
type Profile struct {
	Name        string
	Description string
	Tags        []string
}

// BrowseData combines portable Tag descriptions with read-only workstation
// observations for presentation by the selector.
type BrowseData struct {
	Tags     []Tag
	Profiles []Profile
}

// Confirmation identifies the acknowledgement required after reviewing a
// preview. The preview provider derives this from the shared selection domain
// decision rather than the model inferring it from visible Tags.
type Confirmation string

const (
	ConfirmationNone      Confirmation = ""
	ConfirmationReduction Confirmation = "reduction"
	ConfirmationClear     Confirmation = "clear"
)

// Preview is the opaque preview value returned across the CLI seam.
type Preview struct {
	Text           string
	SemanticDigest string
	// CandidateToken binds an accepted UI value to one process-local candidate.
	// It is opaque presentation data and is not part of the semantic digest.
	CandidateToken string
	ForwardOnly    bool
	Confirmation   Confirmation
}

// PreviewFunc computes a preview for a detached canonical Tag snapshot. The
// request ID is assigned synchronously in UI request order and is opaque to the
// presentation layer.
type PreviewFunc func(uint64, []string) (Preview, error)

// Result is a successfully confirmed selection and its accepted preview.
type Result struct {
	Tags                    []string
	Preview                 Preview
	AcknowledgementAccepted bool
}

// ErrCanceled means the operator canceled without producing usable intent.
var ErrCanceled = errors.New("tag selection canceled")

type screen uint8

const (
	screenList screen = iota
	screenSearch
	screenProfiles
	screenDetail
	screenLoading
	screenPreview
	screenReductionConfirmation
	screenClearConfirmation
)

// Model is the Bubble Tea model for selecting Tags.
type Model struct {
	data        BrowseData
	selected    []bool
	initial     []bool
	cursor      int
	previewFunc PreviewFunc
	screen      screen
	profile     int
	detail      int
	width       int
	height      int
	sized       bool
	theme       theme.Theme
	keys        keyMap
	help        help.Model
	searchInput textinput.Model
	clearEntry  textinput.Model
	spinner     spinner.Model
	browse      viewport.Model
	profiles    viewport.Model
	detailView  viewport.Model
	previewView viewport.Model
	confirmView viewport.Model

	nextRequest  uint64
	pending      *previewRequest
	accepted     Preview
	previewTags  []string
	previewError string
	clearInput   string
	clearError   string
	acknowledged bool
	canceled     bool
	finished     bool
	quitting     bool
}

type previewRequest struct {
	id          uint64
	fingerprint string
	tags        []string
}

type previewResponse struct {
	id          uint64
	fingerprint string
	preview     Preview
	err         error
}

// New builds a model with detached browse data and desired checkbox state.
func New(browseData BrowseData, initial []string, preview PreviewFunc) Model {
	return NewWithTheme(browseData, initial, preview, theme.Default())
}

// NewWithTheme builds a model using an explicit semantic theme. It exists so
// callers and tests can select the shared no-color rendering path without
// changing any selection behavior.
func NewWithTheme(browseData BrowseData, initial []string, preview PreviewFunc, visualTheme theme.Theme) Model {
	browseData = cloneBrowseData(browseData)
	initialSet := make(map[string]bool, len(initial))
	for _, name := range initial {
		initialSet[name] = true
	}
	selected := make([]bool, len(browseData.Tags))
	for i, tag := range browseData.Tags {
		selected[i] = initialSet[tag.Name]
	}
	searchInput := textinput.New()
	searchInput.Prompt = "Search: "
	searchInput.Placeholder = "Tag name or description"
	visualTheme.ApplyTextInput(&searchInput)

	clearEntry := textinput.New()
	clearEntry.Prompt = "management: "
	clearEntry.Placeholder = "exact lowercase ASCII"
	visualTheme.ApplyTextInput(&clearEntry)

	helpModel := help.New()
	helpModel.ShortSeparator = " · "
	visualTheme.ApplyHelp(&helpModel)

	spinnerModel := spinner.New(spinner.WithSpinner(spinner.Line))
	visualTheme.ApplySpinner(&spinnerModel)

	canonicalKeys := newKeyMap()
	model := Model{
		data:        browseData,
		selected:    selected,
		initial:     append([]bool(nil), selected...),
		previewFunc: preview,
		theme:       visualTheme,
		keys:        canonicalKeys,
		help:        helpModel,
		searchInput: searchInput,
		clearEntry:  clearEntry,
		spinner:     spinnerModel,
		browse:      newViewport(visualTheme, canonicalKeys),
		profiles:    newViewport(visualTheme, canonicalKeys),
		detailView:  newViewport(visualTheme, canonicalKeys),
		previewView: newViewport(visualTheme, canonicalKeys),
		confirmView: newViewport(visualTheme, canonicalKeys),
	}
	model.syncComponents()
	return model
}

func newViewport(visualTheme theme.Theme, canonicalKeys keyMap) viewport.Model {
	model := viewport.New(0, 0)
	model.MouseWheelEnabled = true
	model.Style = visualTheme.Body
	model.KeyMap = viewport.KeyMap{
		PageDown: canonicalKeys.PageDown,
		PageUp:   canonicalKeys.PageUp,
		Down:     canonicalKeys.Down,
		Up:       canonicalKeys.Up,
	}
	return model
}

func cloneBrowseData(c BrowseData) BrowseData {
	out := BrowseData{
		Tags:     make([]Tag, len(c.Tags)),
		Profiles: make([]Profile, len(c.Profiles)),
	}
	for i, tag := range c.Tags {
		out.Tags[i] = tag
		out.Tags[i].Profiles = append([]string(nil), tag.Profiles...)
		out.Tags[i].ManagedEntries = append([]string(nil), tag.ManagedEntries...)
		out.Tags[i].Dependencies = append([]string(nil), tag.Dependencies...)
		out.Tags[i].Provisioners = append([]string(nil), tag.Provisioners...)
		out.Tags[i].Components = append([]Component(nil), tag.Components...)
	}
	for i, profile := range c.Profiles {
		out.Profiles[i] = profile
		out.Profiles[i].Tags = append([]string(nil), profile.Tags...)
	}
	return out
}

// Init starts the model without side effects.
func (m Model) Init() tea.Cmd { return nil }

// Update applies navigation and draft checkbox changes while routing messages
// only to the Bubbles component active on the current screen.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if pressed, ok := message.(tea.KeyMsg); ok && key.Matches(pressed, m.keys.Cancel) {
		return m.cancel(), tea.Quit
	}
	if response, ok := message.(previewResponse); ok {
		m = m.acceptPreview(response)
		m.syncComponents()
		return m, nil
	}
	if size, ok := message.(tea.WindowSizeMsg); ok {
		m.width = theme.Clamp(size.Width)
		m.height = theme.Clamp(size.Height)
		m.sized = true
		m.syncComponents()
		return m, nil
	}

	if pressed, ok := message.(tea.KeyMsg); ok && m.screen != screenSearch && key.Matches(pressed, m.keys.Quit) {
		return m.cancel(), tea.Quit
	}

	switch m.screen {
	case screenSearch:
		return m.updateSearch(message)
	case screenProfiles:
		return m.updateProfiles(message)
	case screenDetail:
		return m.updateDetail(message)
	case screenLoading:
		return m.updateLoading(message)
	case screenPreview:
		return m.updatePreview(message)
	case screenReductionConfirmation:
		return m.updateReduction(message)
	case screenClearConfirmation:
		return m.updateClear(message)
	default:
		return m.updateBrowse(message)
	}
}

func (m Model) updateSearch(message tea.Msg) (tea.Model, tea.Cmd) {
	if pressed, ok := message.(tea.KeyMsg); ok {
		switch {
		case key.Matches(pressed, m.keys.Down):
			m.cursor = min(max(0, len(m.visibleTags())-1), m.cursor+1)
			m.syncComponents()
			return m, nil
		case key.Matches(pressed, m.keys.Up):
			m.cursor = max(0, m.cursor-1)
			m.syncComponents()
			return m, nil
		case key.Matches(pressed, m.keys.Back):
			m.searchInput.Reset()
			m.searchInput.Blur()
			m.cursor = 0
			m.screen = screenList
			m.syncComponents()
			return m, nil
		case key.Matches(pressed, m.keys.Accept):
			m.searchInput.Blur()
			m.screen = screenList
			m.syncComponents()
			return m, nil
		}
	}
	before := m.searchInput.Value()
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(message)
	if m.searchInput.Value() != before {
		m.cursor = 0
		m.syncComponents()
	}
	return m, batch(cmd)
}

func (m Model) updateBrowse(message tea.Msg) (tea.Model, tea.Cmd) {
	pressed, isKey := message.(tea.KeyMsg)
	if isKey {
		visible := m.visibleTags()
		switch {
		case key.Matches(pressed, m.keys.Down):
			m.cursor = min(max(0, len(visible)-1), m.cursor+1)
		case key.Matches(pressed, m.keys.Up):
			m.cursor = max(0, m.cursor-1)
		case key.Matches(pressed, m.keys.PageDown):
			m.cursor = min(max(0, len(visible)-1), m.cursor+max(1, m.browse.Height-1))
		case key.Matches(pressed, m.keys.PageUp):
			m.cursor = max(0, m.cursor-max(1, m.browse.Height-1))
		case key.Matches(pressed, m.keys.Home):
			m.cursor = 0
		case key.Matches(pressed, m.keys.End):
			m.cursor = max(0, len(visible)-1)
		case key.Matches(pressed, m.keys.Toggle):
			if m.cursor >= 0 && m.cursor < len(visible) {
				index := visible[m.cursor]
				m.selected[index] = !m.selected[index]
			}
		case key.Matches(pressed, m.keys.Search):
			m.screen = screenSearch
			m.cursor = 0
			cmd := m.searchInput.Focus()
			m.syncComponents()
			return m, batch(cmd)
		case key.Matches(pressed, m.keys.Profiles):
			m.screen = screenProfiles
			m.profile = 0
			m.syncComponents()
			return m, nil
		case key.Matches(pressed, m.keys.Details):
			if m.cursor >= 0 && m.cursor < len(visible) {
				m.detail = visible[m.cursor]
				m.detailView.GotoTop()
				m.screen = screenDetail
				m.syncComponents()
			}
			return m, nil
		case key.Matches(pressed, m.previewAction()):
			return m.startPreview()
		case key.Matches(pressed, m.keys.Back):
			return m.cancel(), tea.Quit
		}
		m.syncComponents()
		return m, nil
	}
	var cmd tea.Cmd
	m.browse, cmd = m.browse.Update(message)
	return m, batch(cmd)
}

func (m Model) updateProfiles(message tea.Msg) (tea.Model, tea.Cmd) {
	if pressed, ok := message.(tea.KeyMsg); ok {
		switch {
		case key.Matches(pressed, m.keys.Down):
			m.profile = min(max(0, len(m.data.Profiles)-1), m.profile+1)
		case key.Matches(pressed, m.keys.Up):
			m.profile = max(0, m.profile-1)
		case key.Matches(pressed, m.keys.PageDown):
			m.profile = min(max(0, len(m.data.Profiles)-1), m.profile+max(1, m.profiles.Height/2))
		case key.Matches(pressed, m.keys.PageUp):
			m.profile = max(0, m.profile-max(1, m.profiles.Height/2))
		case key.Matches(pressed, m.keys.Home):
			m.profile = 0
		case key.Matches(pressed, m.keys.End):
			m.profile = max(0, len(m.data.Profiles)-1)
		case key.Matches(pressed, m.keys.ProfileToggle):
			m.toggleProfile()
		case key.Matches(pressed, m.keys.Back), key.Matches(pressed, m.keys.Profiles):
			m.screen = screenList
			m.syncComponents()
			return m, nil
		}
		m.syncComponents()
		return m, nil
	}
	var cmd tea.Cmd
	m.profiles, cmd = m.profiles.Update(message)
	return m, batch(cmd)
}

func (m Model) updateDetail(message tea.Msg) (tea.Model, tea.Cmd) {
	if pressed, ok := message.(tea.KeyMsg); ok {
		switch {
		case key.Matches(pressed, m.keys.Return):
			m.detailView.GotoTop()
			m.screen = screenList
			m.syncComponents()
			return m, nil
		case key.Matches(pressed, m.keys.Home):
			m.detailView.GotoTop()
			return m, nil
		case key.Matches(pressed, m.keys.End):
			m.detailView.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.detailView, cmd = m.detailView.Update(message)
	return m, batch(cmd)
}

func (m Model) updateLoading(message tea.Msg) (tea.Model, tea.Cmd) {
	if pressed, ok := message.(tea.KeyMsg); ok && key.Matches(pressed, m.keys.Back) {
		m.pending = nil
		m.screen = screenList
		m.syncComponents()
		return m, nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(message)
	return m, batch(cmd)
}

func (m Model) updatePreview(message tea.Msg) (tea.Model, tea.Cmd) {
	if pressed, ok := message.(tea.KeyMsg); ok {
		switch {
		case key.Matches(pressed, m.keys.Back):
			m = m.abandonPreview()
			m.syncComponents()
			return m, nil
		case key.Matches(pressed, m.keys.Preview):
			switch m.confirmation() {
			case ConfirmationReduction:
				m.screen = screenReductionConfirmation
				m.confirmView.GotoTop()
				m.syncComponents()
				return m, nil
			case ConfirmationClear:
				m.screen = screenClearConfirmation
				m.clearEntry.Reset()
				cmd := m.clearEntry.Focus()
				m.confirmView.GotoTop()
				m.syncComponents()
				return m, batch(cmd)
			}
			m.finished = true
			m.quitting = true
			return m, tea.Quit
		case key.Matches(pressed, m.keys.Home):
			m.previewView.GotoTop()
			return m, nil
		case key.Matches(pressed, m.keys.End):
			m.previewView.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.previewView, cmd = m.previewView.Update(message)
	return m, batch(cmd)
}

func (m Model) updateReduction(message tea.Msg) (tea.Model, tea.Cmd) {
	if pressed, ok := message.(tea.KeyMsg); ok {
		switch {
		case key.Matches(pressed, m.keys.Acknowledge):
			return m.finishAcknowledged(), tea.Quit
		case key.Matches(pressed, m.keys.Decline):
			m = m.abandonPreview()
			m.syncComponents()
			return m, nil
		case key.Matches(pressed, m.keys.Home):
			m.confirmView.GotoTop()
			return m, nil
		case key.Matches(pressed, m.keys.End):
			m.confirmView.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.confirmView, cmd = m.confirmView.Update(message)
	return m, batch(cmd)
}

func (m Model) updateClear(message tea.Msg) (tea.Model, tea.Cmd) {
	if pressed, ok := message.(tea.KeyMsg); ok {
		switch {
		case key.Matches(pressed, m.keys.Back):
			m = m.abandonPreview()
			m.syncComponents()
			return m, nil
		case key.Matches(pressed, m.keys.Preview):
			if m.clearEntry.Value() == string(ConfirmationClear) {
				return m.finishAcknowledged(), tea.Quit
			}
			m.clearError = `Confirmation did not match; type exactly "clear".`
			m.syncComponents()
			return m, nil
		case key.Matches(pressed, m.keys.PageUp), key.Matches(pressed, m.keys.PageDown):
			var cmd tea.Cmd
			m.confirmView, cmd = m.confirmView.Update(message)
			return m, batch(cmd)
		}
	}
	before := m.clearEntry.Value()
	var cmd tea.Cmd
	m.clearEntry, cmd = m.clearEntry.Update(message)
	m.clearInput = m.clearEntry.Value()
	if before != m.clearInput {
		m.clearError = ""
		m.syncComponents()
	}
	return m, batch(cmd)
}

func batch(commands ...tea.Cmd) tea.Cmd {
	nonNil := commands[:0]
	for _, command := range commands {
		if command != nil {
			nonNil = append(nonNil, command)
		}
	}
	if len(nonNil) == 0 {
		return nil
	}
	return tea.Batch(nonNil...)
}

func (m Model) startPreview() (tea.Model, tea.Cmd) {
	tags := m.SelectedTags()
	m.nextRequest++
	request := previewRequest{
		id:          m.nextRequest,
		fingerprint: fingerprint(tags),
		tags:        append([]string(nil), tags...),
	}
	m.pending = &request
	m.accepted = Preview{}
	m.previewTags = nil
	m.previewError = ""
	m.clearInput = ""
	m.clearEntry.Reset()
	m.clearEntry.Blur()
	m.clearError = ""
	m.acknowledged = false
	m.previewView.GotoTop()
	m.confirmView.GotoTop()
	m.screen = screenLoading
	provider := m.previewFunc
	previewCommand := func() tea.Msg {
		if provider == nil {
			return previewResponse{id: request.id, fingerprint: request.fingerprint, err: errors.New("preview is unavailable")}
		}
		input := append([]string(nil), request.tags...)
		preview, err := provider(request.id, input)
		return previewResponse{id: request.id, fingerprint: request.fingerprint, preview: preview, err: err}
	}
	return m, batch(m.spinner.Tick, previewCommand)
}

func (m Model) acceptPreview(response previewResponse) Model {
	if m.pending == nil || m.canceled || response.id != m.pending.id || response.fingerprint != m.pending.fingerprint {
		return m
	}
	request := *m.pending
	m.pending = nil
	if response.err != nil {
		m.previewError = response.err.Error()
		m.screen = screenList
		return m
	}
	m.accepted = response.preview
	m.previewTags = cloneStrings(request.tags)
	m.previewError = ""
	m.previewView.GotoTop()
	m.screen = screenPreview
	return m
}

func (m Model) confirmation() Confirmation {
	if len(m.previewTags) == 0 {
		return ConfirmationClear
	}
	return m.accepted.Confirmation
}

func (m Model) finishAcknowledged() Model {
	m.acknowledged = true
	m.finished = true
	m.quitting = true
	return m
}

func (m Model) abandonPreview() Model {
	m.accepted = Preview{}
	m.previewTags = nil
	m.clearInput = ""
	m.clearEntry.Reset()
	m.clearEntry.Blur()
	m.clearError = ""
	m.acknowledged = false
	m.screen = screenList
	return m
}

func (m Model) cancel() Model {
	m.pending = nil
	m.accepted = Preview{}
	m.previewTags = nil
	m.clearInput = ""
	m.searchInput.Blur()
	m.clearEntry.Reset()
	m.clearEntry.Blur()
	m.clearError = ""
	m.acknowledged = false
	m.canceled = true
	m.finished = false
	m.quitting = true
	return m
}

func fingerprint(tags []string) string {
	hash := sha256.New()
	for _, tag := range tags {
		fmt.Fprintf(hash, "%d:%s", len(tag), tag)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (m *Model) toggleProfile() {
	if m.profile < 0 || m.profile >= len(m.data.Profiles) {
		return
	}
	indices := make([]int, 0, len(m.data.Profiles[m.profile].Tags))
	for _, name := range m.data.Profiles[m.profile].Tags {
		for i, tag := range m.data.Tags {
			if tag.Name == name {
				indices = append(indices, i)
				break
			}
		}
	}
	allSelected := len(indices) > 0
	for _, i := range indices {
		if !m.selected[i] {
			allSelected = false
			break
		}
	}
	for _, i := range indices {
		m.selected[i] = !allSelected
	}
}

func (m Model) visibleTags() []int {
	query := strings.ToLower(m.searchInput.Value())
	visible := make([]int, 0, len(m.data.Tags))
	for i, tag := range m.data.Tags {
		if query == "" || strings.Contains(strings.ToLower(tag.Name), query) || strings.Contains(strings.ToLower(tag.Description), query) {
			visible = append(visible, i)
		}
	}
	return visible
}

// SelectedTags returns a detached canonical-order snapshot of desired Tags.
func (m Model) SelectedTags() []string {
	out := make([]string, 0, len(m.data.Tags))
	for i, tag := range m.data.Tags {
		if m.selected[i] {
			out = append(out, tag.Name)
		}
	}
	return out
}

// Preview returns the immutable accepted preview value, if any.
func (m Model) Preview() Preview {
	if m.canceled {
		return Preview{}
	}
	return m.accepted
}

// Result returns a detached successful result. Before confirmation or after
// cancellation it returns the zero value.
func (m Model) Result() Result {
	if m.canceled || !m.finished {
		return Result{}
	}
	return Result{Tags: cloneStrings(m.previewTags), Preview: m.accepted, AcknowledgementAccepted: m.acknowledged}
}

// Canceled reports whether the session ended without usable intent.
func (m Model) Canceled() bool { return m.canceled }

// View renders the current selection screen.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var content string
	switch m.screen {
	case screenProfiles:
		content = m.viewProfiles()
	case screenDetail:
		content = m.viewDetail()
	case screenLoading:
		content = m.viewLoading()
	case screenPreview:
		content = m.viewPreview()
	case screenReductionConfirmation:
		content = m.viewReductionConfirmation()
	case screenClearConfirmation:
		content = m.viewClearConfirmation()
	default:
		content = m.viewList()
	}
	if m.sized {
		return m.theme.Paint(content, m.width, m.height)
	}
	return strings.TrimSuffix(content, "\n") + "\n"
}

func (m Model) viewList() string {
	title := "dots · select Tags"
	if m.compact() {
		title = "dots · Tags"
		if m.searchInput.Value() != "" {
			title = "dots · Tags*"
		}
	}
	summary := m.selectionSummary()
	if m.compact() {
		summary = fmt.Sprintf("%d/%d shown", len(m.visibleTags()), len(m.data.Tags))
	}
	compactError := m.compact() && m.previewError != ""
	lines := []string{}
	if !compactError {
		lines = append(lines, m.theme.Title.Render(title))
	}
	if !m.compact() {
		lines = append(lines, m.theme.Secondary.Render(summary))
	}
	if m.screen == screenSearch {
		lines = append(lines, m.searchInput.View())
	} else if m.searchInput.Value() != "" && !m.compact() {
		lines = append(lines, m.theme.FocusAlt.Render("Filter: "+m.searchInput.Value()))
	}

	body := m.viewportBody(m.browse)
	if m.width >= wideBreakpoint && m.screen != screenSearch {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, "  ", m.viewportBody(m.detailView))
	}
	lines = appendIfVisible(lines, body)
	if m.previewError != "" {
		if compactError {
			lines = append(lines, m.theme.Error.Render(m.compactPreviewError()))
		} else {
			lines = append(lines, m.theme.Error.Render(m.theme.Glyphs.Error+" Preview error: "+m.previewError))
		}
	} else if status := viewportStatus(m.browse); status != "" {
		lines = append(lines, m.theme.Secondary.Render(status))
	}
	if m.compact() && m.screen != screenSearch {
		action := m.keys.Toggle
		if compactError {
			action = m.keys.Retry
		}
		lines = append(lines, m.help.ShortHelpView([]key.Binding{action}))
	} else if !m.compact() {
		lines = append(lines, m.help.View(m.activeHelp()))
	}
	return strings.Join(lines, "\n")
}

func (m Model) compactPreviewError() string {
	prefix := "error:"
	cause := strings.TrimSpace(m.previewError)
	if cause == "" {
		cause = "unknown"
	}
	wrapped := theme.Wrap(cause, max(1, m.width-theme.Width(prefix)))
	if first, _, ok := strings.Cut(wrapped, "\n"); ok {
		wrapped = first
	}
	return prefix + wrapped
}

func (m Model) listContent(width int) (string, int) {
	var lines []string
	cursorLine := 0
	lastGroup := ""
	for position, i := range m.visibleTags() {
		tag := m.data.Tags[i]
		if !m.compact() && tag.Group != lastGroup {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, m.theme.Secondary.Render(tag.Group))
			lastGroup = tag.Group
		}
		cursor := " "
		if position == m.cursor {
			cursor = m.theme.Glyphs.Cursor
			cursorLine = len(lines)
		}
		checkbox := m.theme.Glyphs.Unchecked
		if m.selected[i] {
			checkbox = m.theme.Glyphs.Checked
		}
		row := fmt.Sprintf("%s %s %s", cursor, checkbox, tag.Name)
		if !m.compact() && tag.Description != "" {
			row += " — " + tag.Description
		}
		if !m.compact() && tag.State != "" {
			row += fmt.Sprintf(" (%s)", tag.State)
		}
		style := m.theme.Body
		if position == m.cursor {
			style = m.theme.Selected
		}
		wrapped := splitLines(theme.Wrap(style.Render(row), width))
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		lines = append(lines, wrapped...)
		if m.compact() {
			lines = append(lines, m.theme.Secondary.Render("state: "+valueOrNone(string(tag.State))))
		}
	}
	if len(lines) == 0 {
		return m.theme.Secondary.Render("No matching Tags."), 0
	}
	return strings.Join(lines, "\n"), cursorLine
}

func (m Model) viewPreview() string {
	title := "dots · selection preview"
	if m.compact() {
		title = "dots · preview"
	}
	summary := "Selection Reconciliation Plan · " + valueOrReady(viewportStatus(m.previewView))
	if m.compact() {
		summary = "plan"
	}
	lines := []string{m.theme.Title.Render(title), m.theme.Secondary.Render(summary)}
	lines = appendIfVisible(lines, m.viewportBody(m.previewView))
	lines = append(lines, m.help.View(m.activeHelp()))
	return strings.Join(lines, "\n")
}

func (m Model) viewReductionConfirmation() string {
	title := "dots · confirm selection reduction"
	warning := m.theme.Glyphs.Warning + " This removes selected Tags from dots management; retained external state is shown below."
	if m.compact() {
		title = "REDUCTION"
		warning = "destructive"
	}
	lines := []string{m.theme.Title.Render(title), m.theme.Warning.Render(warning)}
	lines = appendIfVisible(lines, m.viewportBody(m.confirmView))
	if status := viewportStatus(m.confirmView); status != "" {
		lines = append(lines, m.theme.Secondary.Render(status))
	}
	lines = append(lines, m.help.View(m.activeHelp()))
	return strings.Join(lines, "\n")
}

func (m Model) viewClearConfirmation() string {
	title := "dots · confirm clear selection"
	warning := m.theme.Glyphs.Warning + ` Type "clear" to remove every selected Managed Entry from dots management.`
	if m.sized && m.width < 100 {
		warning = m.theme.Glyphs.Warning + ` DESTRUCTIVE: clear all Managed Entries.`
	}
	if m.compact() {
		title = "CLEAR ALL"
		warning = "destructive"
		if m.clearError != "" {
			warning = "x error"
		}
	}
	lines := []string{m.theme.Title.Render(title), m.theme.Caution.Render(warning)}
	if m.compact() {
		lines = append(lines, m.help.ShortHelpView([]key.Binding{m.keys.Confirm}))
	}
	lines = append(lines, m.clearEntry.View())
	if m.clearError != "" && !m.compact() {
		lines = append(lines, m.theme.Error.Render(m.theme.Glyphs.Error+" error: "+m.clearError))
	}
	lines = appendIfVisible(lines, m.viewportBody(m.confirmView))
	if status := viewportStatus(m.confirmView); status != "" {
		lines = append(lines, m.theme.Secondary.Render(status))
	}
	if !m.compact() {
		lines = append(lines, m.help.View(m.activeHelp()))
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewProfiles() string {
	title := "dots · Profile presets"
	if m.compact() {
		title = "dots · presets"
	}
	summary := fmt.Sprintf("%d presets · %d Tags selected", len(m.data.Profiles), len(m.SelectedTags()))
	if m.compact() {
		summary = fmt.Sprintf("%d presets", len(m.data.Profiles))
	}
	lines := []string{m.theme.Title.Render(title), m.theme.Secondary.Render(summary)}
	lines = appendIfVisible(lines, m.viewportBody(m.profiles))
	lines = append(lines, m.help.View(m.activeHelp()))
	return strings.Join(lines, "\n")
}

func (m Model) profileContent(width int) (string, int) {
	var lines []string
	cursorLine := 0
	for i, profile := range m.data.Profiles {
		cursor := " "
		if i == m.profile {
			cursor = m.theme.Glyphs.Cursor
			cursorLine = len(lines)
		}
		row := fmt.Sprintf("%s %s %s — %s", cursor, m.profileMark(profile), profile.Name, profile.Description)
		style := m.theme.Body
		if i == m.profile {
			style = m.theme.Selected
		}
		lines = append(lines, splitLines(theme.Wrap(style.Render(row), width))...)
		lines = append(lines, splitLines(theme.Wrap(m.theme.Secondary.Render("    Tags: "+strings.Join(profile.Tags, ", ")), width))...)
	}
	if len(lines) == 0 {
		return m.theme.Secondary.Render("No Profile presets available."), 0
	}
	return strings.Join(lines, "\n"), cursorLine
}

func (m Model) profileMark(profile Profile) string {
	valid, selected := 0, 0
	for _, name := range profile.Tags {
		for i, tag := range m.data.Tags {
			if tag.Name == name {
				valid++
				if m.selected[i] {
					selected++
				}
				break
			}
		}
	}
	if valid > 0 && selected == valid {
		return m.theme.Glyphs.Checked
	}
	if selected > 0 {
		return m.theme.Glyphs.Partial
	}
	return m.theme.Glyphs.Unchecked
}

func (m Model) viewDetail() string {
	title := "dots · Tag details"
	if m.compact() {
		title = "dots · details"
	}
	lines := []string{
		m.theme.Title.Render(title),
		m.theme.Secondary.Render(valueOrReady(viewportStatus(m.detailView))),
	}
	lines = appendIfVisible(lines, m.viewportBody(m.detailView))
	lines = append(lines, m.help.View(m.activeHelp()))
	return strings.Join(lines, "\n")
}

func (m Model) viewLoading() string {
	title := "dots · Loading preview…"
	if m.compact() {
		title = "dots · preview"
	}
	loading := m.spinner.View() + " Building the pending Selection Reconciliation Plan."
	if m.compact() {
		loading = m.spinner.View() + " loading"
	}
	return strings.Join([]string{
		m.theme.Title.Render(title),
		m.theme.Action.Render(loading),
		m.help.View(m.activeHelp()),
	}, "\n")
}

func (m Model) previewContent() string {
	text := m.accepted.Text
	if m.accepted.ForwardOnly {
		text += "\n\n[Forward-only]"
	}
	return text
}

func (m Model) viewportBody(model viewport.Model) string {
	if model.Width <= 0 || model.Height <= 0 {
		return ""
	}
	return model.View()
}

func (m Model) compact() bool {
	return m.sized && m.width < 25
}

func appendIfVisible(lines []string, value string) []string {
	if value == "" {
		return lines
	}
	return append(lines, value)
}

func viewportStatus(model viewport.Model) string {
	switch {
	case !model.AtTop() && !model.AtBottom():
		return "↑ more · ↓ more"
	case !model.AtTop():
		return "↑ more"
	case !model.AtBottom():
		return "↓ more"
	default:
		return ""
	}
}

func valueOrReady(value string) string {
	if value == "" {
		return "ready"
	}
	return value
}

func (m Model) selectionSummary() string {
	visible := len(m.visibleTags())
	summary := fmt.Sprintf("%d selected · %d of %d shown", len(m.SelectedTags()), visible, len(m.data.Tags))
	if query := m.searchInput.Value(); query != "" {
		summary += fmt.Sprintf(" · filter %q", query)
	}
	return summary
}

func (m Model) viewDetailFor(i int) string {
	if i < 0 || i >= len(m.data.Tags) {
		return ""
	}
	tag := m.data.Tags[i]
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", tag.Name)
	fmt.Fprintf(&b, "Description: %s\n", valueOrNone(tag.Description))
	fmt.Fprintf(&b, "Profiles: %s\n", listOrNone(tag.Profiles))
	fmt.Fprintf(&b, "Managed Entries: %s\n", listOrNone(tag.ManagedEntries))
	fmt.Fprintf(&b, "Dependencies: %s\n", listOrNone(tag.Dependencies))
	fmt.Fprintf(&b, "Provisioners: %s\n", listOrNone(tag.Provisioners))
	fmt.Fprintf(&b, "Observed Status: %s\n", valueOrNone(string(tag.State)))
	b.WriteString("Components:\n")
	if len(tag.Components) == 0 {
		b.WriteString("  none\n")
	}
	for _, component := range tag.Components {
		fmt.Fprintf(&b, "  %s · %s · %s", component.Kind, component.Name, component.State)
		if component.Detail != "" {
			fmt.Fprintf(&b, " · %s", component.Detail)
		}
		b.WriteByte('\n')
	}
	if tag.ExternalEffectsPresent && m.initial[i] && !m.selected[i] {
		b.WriteString("Retained External State [retained]\n")
	} else if tag.ExternalEffectsPresent {
		b.WriteString("External Effects Present\n")
	} else {
		b.WriteString("External Effects Present: none\n")
	}
	draft := "unchanged"
	if m.selected[i] && !m.initial[i] {
		draft = "add"
	} else if !m.selected[i] && m.initial[i] {
		draft = "remove"
	}
	fmt.Fprintf(&b, "Draft Change: %s\n", draft)
	return b.String()
}

const wideBreakpoint = 100

func (m *Model) syncComponents() {
	width, height := m.width, m.height
	if !m.sized {
		width, height = 80, 24
	}
	width, height = theme.Clamp(width), theme.Clamp(height)
	m.help.Width = width
	if m.sized && width < 25 {
		m.searchInput.Prompt = "/ "
		m.searchInput.Placeholder = ""
		m.clearEntry.Prompt = "clear: "
		m.clearEntry.Placeholder = ""
	} else {
		m.searchInput.Prompt = "Search: "
		m.searchInput.Placeholder = "Tag name or description"
		m.clearEntry.Prompt = "management: "
		m.clearEntry.Placeholder = "exact lowercase ASCII"
	}
	m.searchInput.Width = max(1, theme.Clamp(width-theme.Width(m.searchInput.Prompt)))
	m.clearEntry.Width = max(1, theme.Clamp(width-theme.Width(m.clearEntry.Prompt)))
	m.clearInput = m.clearEntry.Value()

	queryLine := 0
	if m.screen == screenSearch || m.searchInput.Value() != "" {
		queryLine = 1
	}
	browseHeight := theme.Clamp(height - 4 - queryLine)
	if m.compact() {
		browseHeight = theme.Clamp(height - 2)
	}
	browseWidth := width
	detailWidth := width
	if width >= wideBreakpoint && m.screen != screenSearch {
		browseWidth = theme.Clamp((width - 2) / 3)
		detailWidth = theme.Clamp(width - browseWidth - 2)
	}
	m.resizeViewport(&m.browse, browseWidth, browseHeight)
	content, cursorLine := m.listContent(browseWidth)
	m.browse.SetContent(content)
	ensureVisible(&m.browse, cursorLine)

	m.resizeViewport(&m.profiles, width, theme.Clamp(height-3))
	profileContent, profileLine := m.profileContent(width)
	m.profiles.SetContent(profileContent)
	ensureVisible(&m.profiles, profileLine)

	visible := m.visibleTags()
	if (m.screen == screenList || m.screen == screenSearch) && m.cursor >= 0 && m.cursor < len(visible) {
		m.detail = visible[m.cursor]
	}
	detailHeight := theme.Clamp(height - 3)
	if width >= wideBreakpoint && m.screen != screenSearch {
		detailHeight = browseHeight
	}
	m.resizeViewport(&m.detailView, detailWidth, detailHeight)
	m.detailView.SetContent(styleAndWrap(m.viewDetailFor(m.detail), detailWidth, m.theme.Body))

	m.resizeViewport(&m.previewView, width, theme.Clamp(height-3))
	m.previewView.SetContent(styleAndWrap(m.previewContent(), width, m.theme.Body))

	confirmationReserved := 4
	if m.screen == screenClearConfirmation {
		confirmationReserved = 5
		if m.clearError != "" {
			confirmationReserved++
		}
	}
	m.resizeViewport(&m.confirmView, width, theme.Clamp(height-confirmationReserved))
	m.confirmView.SetContent(styleAndWrap(m.previewContent(), width, m.theme.Body))
}

func (m Model) resizeViewport(model *viewport.Model, width, height int) {
	model.Width = theme.Clamp(width)
	model.Height = theme.Clamp(height)
	model.Style = m.theme.Body
	model.SetYOffset(model.YOffset)
}

func ensureVisible(model *viewport.Model, line int) {
	if model.Height <= 0 {
		model.SetYOffset(0)
		return
	}
	if line < model.YOffset {
		model.SetYOffset(line)
		return
	}
	if line >= model.YOffset+model.Height {
		model.SetYOffset(line - model.Height + 1)
	}
}

func styleAndWrap(value string, width int, style lipgloss.Style) string {
	if width <= 0 || value == "" {
		return ""
	}
	var rendered []string
	for _, line := range strings.Split(strings.TrimSuffix(value, "\n"), "\n") {
		wrapped := theme.Wrap(line, width)
		if wrapped == "" {
			rendered = append(rendered, style.Render(""))
			continue
		}
		for _, part := range strings.Split(wrapped, "\n") {
			rendered = append(rendered, style.Render(part))
		}
	}
	return strings.Join(rendered, "\n")
}

func splitLines(value string) []string {
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return []string{}
	}
	return strings.Split(value, "\n")
}

func listOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func valueOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func cloneStrings(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}
