// Package tagselector implements the interactive Tag selection model used by
// installation flows. It owns draft intent only; callers inject preview work.
package tagselector

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	data          BrowseData
	selected      []bool
	initial       []bool
	cursor        int
	previewFunc   PreviewFunc
	screen        screen
	query         string
	profile       int
	detail        int
	width         int
	height        int
	detailScroll  int
	previewScroll int

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
	browseData = cloneBrowseData(browseData)
	initialSet := make(map[string]bool, len(initial))
	for _, name := range initial {
		initialSet[name] = true
	}
	selected := make([]bool, len(browseData.Tags))
	for i, tag := range browseData.Tags {
		selected[i] = initialSet[tag.Name]
	}
	return Model{data: browseData, selected: selected, initial: append([]bool(nil), selected...), previewFunc: preview}
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

// Update applies navigation and draft checkbox changes.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if response, ok := message.(previewResponse); ok {
		return m.acceptPreview(response), nil
	}
	if size, ok := message.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		return m, nil
	}
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.Type == tea.KeyCtrlC || (m.screen != screenSearch && key.String() == "q") {
		return m.cancel(), tea.Quit
	}
	if m.screen == screenSearch {
		switch key.Type {
		case tea.KeyEsc:
			m.screen = screenList
			m.query = ""
			m.cursor = 0
		case tea.KeyEnter:
			m.screen = screenList
		case tea.KeyDown:
			if m.cursor+1 < len(m.visibleTags()) {
				m.cursor++
			}
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.cursor = 0
			}
		case tea.KeyRunes:
			m.query += string(key.Runes)
			m.cursor = 0
		}
		return m, nil
	}
	if m.screen == screenLoading {
		if key.Type == tea.KeyEsc {
			m.pending = nil
			m.screen = screenList
		}
		return m, nil
	}
	if m.screen == screenPreview {
		switch key.Type {
		case tea.KeyEsc:
			m.accepted = Preview{}
			m.previewTags = nil
			m.previewScroll = 0
			m.screen = screenList
		case tea.KeyEnter:
			switch m.confirmation() {
			case ConfirmationReduction:
				m.screen = screenReductionConfirmation
				return m, nil
			case ConfirmationClear:
				m.screen = screenClearConfirmation
				return m, nil
			}
			m.finished = true
			m.quitting = true
			return m, tea.Quit
		}
		return m.scrollPreview(key), nil
	}
	if m.screen == screenReductionConfirmation {
		switch key.Type {
		case tea.KeyEnter:
			return m.finishAcknowledged(), tea.Quit
		case tea.KeyEsc:
			return m.abandonPreview(), nil
		}
		switch key.String() {
		case "y":
			return m.finishAcknowledged(), tea.Quit
		case "n":
			return m.abandonPreview(), nil
		}
		return m.scrollPreview(key), nil
	}
	if m.screen == screenClearConfirmation {
		switch key.Type {
		case tea.KeyEsc:
			return m.abandonPreview(), nil
		case tea.KeyEnter:
			if m.clearInput == string(ConfirmationClear) {
				return m.finishAcknowledged(), tea.Quit
			}
			m.clearError = `Confirmation did not match; type exactly "clear".`
		case tea.KeyBackspace, tea.KeyDelete:
			if len(m.clearInput) > 0 {
				runes := []rune(m.clearInput)
				m.clearInput = string(runes[:len(runes)-1])
			}
			m.clearError = ""
		case tea.KeyRunes:
			m.clearInput += string(key.Runes)
			m.clearError = ""
		}
		return m, nil
	}
	if m.screen == screenProfiles {
		switch key.String() {
		case "j", "down":
			if m.profile+1 < len(m.data.Profiles) {
				m.profile++
			}
		case "k", "up":
			if m.profile > 0 {
				m.profile--
			}
		case " ", "enter":
			m.toggleProfile()
		case "esc", "p":
			m.screen = screenList
		}
		return m, nil
	}
	if m.screen == screenDetail {
		switch key.String() {
		case "esc", "left", "d":
			m.detailScroll = 0
			m.screen = screenList
		case "j", "down":
			m.detailScroll = scrolledOffset(m.detailScroll, 1, len(m.detailContentLines()), m.detailBodyHeight())
		case "k", "up":
			m.detailScroll = scrolledOffset(m.detailScroll, -1, len(m.detailContentLines()), m.detailBodyHeight())
		case "pgdown":
			m.detailScroll = scrolledOffset(m.detailScroll, max(1, m.detailBodyHeight()-2), len(m.detailContentLines()), m.detailBodyHeight())
		case "pgup":
			m.detailScroll = scrolledOffset(m.detailScroll, -max(1, m.detailBodyHeight()-2), len(m.detailContentLines()), m.detailBodyHeight())
		case "home":
			m.detailScroll = 0
		case "end":
			m.detailScroll = clampScrollOffset(len(m.detailContentLines()), m.detailBodyHeight(), len(m.detailContentLines()))
		}
		return m, nil
	}
	switch key.String() {
	case "j", "down":
		if m.cursor+1 < len(m.visibleTags()) {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case " ":
		visible := m.visibleTags()
		if m.cursor >= 0 && m.cursor < len(visible) {
			i := visible[m.cursor]
			m.selected[i] = !m.selected[i]
		}
	case "/":
		m.screen = screenSearch
		m.query = ""
		m.cursor = 0
	case "p":
		m.screen = screenProfiles
		m.profile = 0
	case "d", "right":
		visible := m.visibleTags()
		if m.cursor >= 0 && m.cursor < len(visible) {
			m.detail = visible[m.cursor]
			m.detailScroll = 0
			m.screen = screenDetail
		}
	case "enter":
		return m.startPreview()
	case "esc":
		return m.cancel(), tea.Quit
	}
	return m, nil
}

func (m Model) scrollPreview(key tea.KeyMsg) Model {
	bodyHeight := m.previewBodyHeight()
	contentLines := len(m.previewContentLines())
	switch key.String() {
	case "j", "down":
		m.previewScroll = scrolledOffset(m.previewScroll, 1, contentLines, bodyHeight)
	case "k", "up":
		m.previewScroll = scrolledOffset(m.previewScroll, -1, contentLines, bodyHeight)
	case "pgdown":
		m.previewScroll = scrolledOffset(m.previewScroll, max(1, bodyHeight-2), contentLines, bodyHeight)
	case "pgup":
		m.previewScroll = scrolledOffset(m.previewScroll, -max(1, bodyHeight-2), contentLines, bodyHeight)
	case "home":
		m.previewScroll = 0
	case "end":
		m.previewScroll = clampScrollOffset(contentLines, bodyHeight, contentLines)
	}
	return m
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
	m.clearError = ""
	m.acknowledged = false
	m.previewScroll = 0
	m.screen = screenLoading
	provider := m.previewFunc
	return m, func() tea.Msg {
		if provider == nil {
			return previewResponse{id: request.id, fingerprint: request.fingerprint, err: errors.New("preview is unavailable")}
		}
		input := append([]string(nil), request.tags...)
		preview, err := provider(request.id, input)
		return previewResponse{id: request.id, fingerprint: request.fingerprint, preview: preview, err: err}
	}
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
	m.previewScroll = 0
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
	m.previewScroll = 0
	m.clearInput = ""
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
	query := strings.ToLower(m.query)
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
	if m.screen == screenLoading {
		return "dots · Loading preview…\n\nEsc returns to selection · q/ctrl+c cancel\n"
	}
	if m.screen == screenPreview {
		return m.viewPreview()
	}
	if m.screen == screenReductionConfirmation {
		return m.viewReductionConfirmation()
	}
	if m.screen == screenClearConfirmation {
		return m.viewClearConfirmation()
	}
	if m.screen == screenProfiles {
		return m.viewProfiles()
	}
	if m.screen == screenDetail {
		return m.viewDetail()
	}
	return m.viewList()
}

func (m Model) viewList() string {
	header := []string{"dots · select Tags"}
	if m.screen == screenSearch {
		header = append(header, "Search: "+m.query)
	} else if m.query != "" {
		header = append(header, "Filter: "+m.query)
	}
	if m.previewError != "" {
		header = append(header, "Preview error: "+m.previewError)
	}

	help := "/ search · space toggle · p profiles · d details · enter preview · q cancel"
	if m.screen == screenSearch {
		help = "type to search · enter accept · esc clear · ctrl+c cancel"
	}
	footer := []string{"", help}
	lines, cursorLine := m.listLines()

	var detail []string
	visible := m.visibleTags()
	if m.width >= 100 && m.cursor >= 0 && m.cursor < len(visible) {
		detail = append([]string{""}, splitRendered(m.viewDetailFor(visible[m.cursor]))...)
		if m.height > 0 && len(detail) > m.height/2 {
			detail = scrollPage(detail, 0, max(1, m.height/2))
		}
	}
	bodyHeight := availableBodyHeight(m.height, len(header)+len(footer)+len(detail))
	if bodyHeight < 3 && len(detail) > 0 {
		detail = nil
		bodyHeight = availableBodyHeight(m.height, len(header)+len(footer))
	}
	body := cursorPage(lines, cursorLine, bodyHeight)
	return renderLines(append(append(append(header, body...), footer...), detail...))
}

func (m Model) listLines() ([]string, int) {
	lines := []string{}
	cursorLine := 0
	lastGroup := ""
	for position, i := range m.visibleTags() {
		tag := m.data.Tags[i]
		if tag.Group != lastGroup {
			lines = append(lines, "", tag.Group)
			lastGroup = tag.Group
		}
		cursor := "  "
		if position == m.cursor {
			cursor = "> "
			cursorLine = len(lines)
		}
		checkbox := " "
		if m.selected[i] {
			checkbox = "x"
		}
		row := fmt.Sprintf("%s[%s] %s", cursor, checkbox, tag.Name)
		if tag.Description != "" {
			row += " — " + tag.Description
		}
		if tag.State != "" {
			row += fmt.Sprintf(" (%s)", tag.State)
		}
		lines = append(lines, row)
	}
	if len(lines) == 0 {
		return []string{"", "No matching Tags."}, 1
	}
	return lines, cursorLine
}

func (m Model) viewPreview() string {
	header := []string{"dots · selection preview", ""}
	footer := []string{"", "j/k scroll · pgup/pgdown · enter confirm · esc back · q/ctrl+c cancel"}
	body := scrollPage(m.previewContentLines(), m.previewScroll, m.previewBodyHeight())
	return renderLines(append(append(header, body...), footer...))
}

func (m Model) viewReductionConfirmation() string {
	header := []string{"dots · confirm selection reduction", ""}
	footer := []string{"", "y/enter acknowledge · n/esc back · q/ctrl+c cancel"}
	body := scrollPage(m.previewContentLines(), m.previewScroll, availableBodyHeight(m.height, len(header)+len(footer)))
	return renderLines(append(append(header, body...), footer...))
}

func (m Model) viewClearConfirmation() string {
	header := []string{"dots · confirm clear selection", ""}
	body := append(m.previewContentLines(), "", `Type "clear" to remove every selected Managed Entry from dots management: `+m.clearInput)
	if m.clearError != "" {
		body = append(body, m.clearError)
	}
	footer := []string{"", "enter acknowledge · esc back · q/ctrl+c cancel"}
	return renderLines(append(append(header, body...), footer...))
}

func (m Model) viewProfiles() string {
	header := []string{"dots · Profile presets", ""}
	lines := []string{}
	cursorLine := 0
	for i, profile := range m.data.Profiles {
		cursor := "  "
		if i == m.profile {
			cursor = "> "
			cursorLine = len(lines)
		}
		lines = append(lines,
			fmt.Sprintf("%s[%s] %s — %s", cursor, m.profileMark(profile), profile.Name, profile.Description),
			"    Tags: "+strings.Join(profile.Tags, ", "),
		)
	}
	if len(lines) == 0 {
		lines = []string{"No Profile presets available."}
	}
	footer := []string{"", "space/enter apply preset · esc return · ctrl+c cancel"}
	body := cursorPage(lines, cursorLine, availableBodyHeight(m.height, len(header)+len(footer)))
	return renderLines(append(append(header, body...), footer...))
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
		return "x"
	}
	if selected > 0 {
		return "-"
	}
	return " "
}

func (m Model) viewDetail() string {
	header := []string{"dots · Tag details", ""}
	footer := []string{"", "j/k scroll · pgup/pgdown · esc/left/d return · ctrl+c cancel"}
	body := scrollPage(m.detailContentLines(), m.detailScroll, m.detailBodyHeight())
	return renderLines(append(append(header, body...), footer...))
}

func (m Model) previewContentLines() []string {
	lines := splitRendered(m.accepted.Text)
	if m.accepted.ForwardOnly {
		lines = append(lines, "", "[Forward-only]")
	}
	return lines
}

func (m Model) detailContentLines() []string {
	return splitRendered(m.viewDetailFor(m.detail))
}

func (m Model) previewBodyHeight() int { return availableBodyHeight(m.height, 4) }

func (m Model) detailBodyHeight() int { return availableBodyHeight(m.height, 4) }

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

func availableBodyHeight(height, fixedLines int) int {
	if height <= 0 {
		return 0
	}
	return max(1, height-fixedLines)
}

func cursorPage(lines []string, cursorLine, height int) []string {
	if height <= 0 || len(lines) <= height {
		return append([]string(nil), lines...)
	}
	start := cursorLine - height/2
	start = max(0, min(start, len(lines)-height))
	page := append([]string(nil), lines[start:start+height]...)
	if start > 0 {
		page[0] = "↑ more"
	}
	if start+height < len(lines) {
		page[len(page)-1] = "↓ more"
	}
	return page
}

func scrollPage(lines []string, offset, height int) []string {
	if height <= 0 || len(lines) <= height {
		return append([]string(nil), lines...)
	}
	offset = clampScrollOffset(offset, height, len(lines))
	page := append([]string(nil), lines[offset:offset+height]...)
	if offset > 0 {
		page[0] = "↑ more"
	}
	if offset+height < len(lines) {
		page[len(page)-1] = "↓ more"
	}
	return page
}

func scrolledOffset(current, delta, total, height int) int {
	return clampScrollOffset(current+delta, height, total)
}

func clampScrollOffset(offset, height, total int) int {
	if height <= 0 || total <= height {
		return 0
	}
	return max(0, min(offset, total-height))
}

func splitRendered(value string) []string {
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return []string{}
	}
	return strings.Split(value, "\n")
}

func renderLines(lines []string) string {
	return strings.Join(lines, "\n") + "\n"
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
