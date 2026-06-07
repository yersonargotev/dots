package tui

import (
	catppuccin "github.com/catppuccin/go"
	"github.com/charmbracelet/lipgloss"
	"github.com/yersonargotev/dots/internal/install"
)

// Styles holds the lipgloss styles for the conflict resolver, themed with
// Catppuccin Mocha. Keeping them in one place makes the palette swappable and
// keeps View() focused on layout rather than color literals.
type Styles struct {
	Title     lipgloss.Style
	Subtitle  lipgloss.Style
	Cursor    lipgloss.Style
	Target    lipgloss.Style
	Source    lipgloss.Style
	Help      lipgloss.Style
	DiffTitle lipgloss.Style
	DiffBody  lipgloss.Style

	decision map[install.ConflictDecision]lipgloss.Style
}

// color returns a lipgloss color from a Catppuccin Mocha swatch hex.
func mocha(c catppuccin.Color) lipgloss.Color {
	return lipgloss.Color(c.Hex)
}

// DefaultStyles builds the Catppuccin Mocha themed styles.
func DefaultStyles() Styles {
	m := catppuccin.Mocha
	return Styles{
		Title:     lipgloss.NewStyle().Bold(true).Foreground(mocha(m.Mauve())),
		Subtitle:  lipgloss.NewStyle().Foreground(mocha(m.Subtext0())),
		Cursor:    lipgloss.NewStyle().Bold(true).Foreground(mocha(m.Sky())),
		Target:    lipgloss.NewStyle().Foreground(mocha(m.Text())),
		Source:    lipgloss.NewStyle().Foreground(mocha(m.Subtext1())),
		Help:      lipgloss.NewStyle().Foreground(mocha(m.Overlay1())),
		DiffTitle: lipgloss.NewStyle().Bold(true).Foreground(mocha(m.Yellow())),
		DiffBody: lipgloss.NewStyle().
			Foreground(mocha(m.Text())).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mocha(m.Surface2())).
			Padding(0, 1),
		decision: map[install.ConflictDecision]lipgloss.Style{
			install.DecisionSkip:    lipgloss.NewStyle().Foreground(mocha(m.Overlay0())),
			install.DecisionReplace: lipgloss.NewStyle().Bold(true).Foreground(mocha(m.Peach())),
			install.DecisionAdopt:   lipgloss.NewStyle().Bold(true).Foreground(mocha(m.Green())),
		},
	}
}

// Decision returns the style for a given decision, falling back to skip.
func (s Styles) Decision(d install.ConflictDecision) lipgloss.Style {
	if style, ok := s.decision[d]; ok {
		return style
	}
	return s.decision[install.DecisionSkip]
}
