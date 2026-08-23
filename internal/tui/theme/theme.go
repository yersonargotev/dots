// Package theme provides the shared semantic styling used by dots' terminal
// interfaces. It deliberately has no dependency on either TUI package, so both
// the conflict resolver and tag selector can consume it without an import cycle.
package theme

import (
	catppuccin "github.com/catppuccin/go"
	"github.com/charmbracelet/lipgloss"
)

// Glyphs are portable, color-independent state cues. Consumers should keep the
// accompanying state label (for example, "error" or "skip") visible as well.
type Glyphs struct {
	Cursor    string
	Checked   string
	Unchecked string
	Partial   string
	Success   string
	Warning   string
	Error     string
	Inactive  string
}

// Theme maps semantic roles to Lip Gloss styles. Geometry is intentionally
// separate from secondary text: InactiveGeometry is the only role that uses
// Overlay0, whose contrast is insufficient for essential normal text.
type Theme struct {
	Canvas           lipgloss.Style
	Title            lipgloss.Style
	Body             lipgloss.Style
	Secondary        lipgloss.Style
	Selected         lipgloss.Style
	Action           lipgloss.Style
	Success          lipgloss.Style
	Warning          lipgloss.Style
	Caution          lipgloss.Style
	Error            lipgloss.Style
	Focus            lipgloss.Style
	FocusAlt         lipgloss.Style
	InactiveGeometry lipgloss.Style

	Glyphs Glyphs
	color  bool
}

// Default returns the shared Catppuccin Mocha theme.
func Default() Theme {
	m := catppuccin.Mocha
	base := color(m.Base())
	style := func(foreground catppuccin.Color) lipgloss.Style {
		return lipgloss.NewStyle().
			Foreground(color(foreground)).
			Background(base).
			ColorWhitespace(true)
	}

	return Theme{
		Canvas:           lipgloss.NewStyle().Background(base).ColorWhitespace(true),
		Title:            style(m.Text()).Bold(true),
		Body:             style(m.Text()),
		Secondary:        style(m.Subtext0()),
		Selected:         style(m.Blue()).Bold(true),
		Action:           style(m.Blue()),
		Success:          style(m.Green()),
		Warning:          style(m.Yellow()),
		Caution:          style(m.Peach()),
		Error:            style(m.Red()),
		Focus:            style(m.Rosewater()).Bold(true),
		FocusAlt:         style(m.Lavender()),
		InactiveGeometry: style(m.Overlay0()),
		Glyphs:           defaultGlyphs(),
		color:            true,
	}
}

// NoColor returns the same semantic and glyph contract without terminal
// styling. Rendering through this theme emits no ANSI escape sequences.
func NoColor() Theme {
	plain := lipgloss.NewStyle()
	return Theme{
		Canvas:           plain,
		Title:            plain,
		Body:             plain,
		Secondary:        plain,
		Selected:         plain,
		Action:           plain,
		Success:          plain,
		Warning:          plain,
		Caution:          plain,
		Error:            plain,
		Focus:            plain,
		FocusAlt:         plain,
		InactiveGeometry: plain,
		Glyphs:           defaultGlyphs(),
	}
}

func color(c catppuccin.Color) lipgloss.Color {
	return lipgloss.Color(c.Hex)
}

func defaultGlyphs() Glyphs {
	return Glyphs{
		Cursor:    ">",
		Checked:   "[x]",
		Unchecked: "[ ]",
		Partial:   "[-]",
		Success:   "+",
		Warning:   "!",
		Error:     "x",
		Inactive:  "-",
	}
}
