package theme

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Clamp converts a possibly negative terminal dimension to a usable value.
func Clamp(value int) int {
	return max(0, value)
}

// InnerSize subtracts reserved shell space and clamps zero/tiny terminals.
func InnerSize(width, height, reservedWidth, reservedHeight int) (int, int) {
	return Clamp(width - Clamp(reservedWidth)), Clamp(height - Clamp(reservedHeight))
}

// Width returns the maximum display-cell width of an ANSI-styled string.
func Width(value string) int {
	width := 0
	for line := range strings.SplitSeq(value, "\n") {
		width = max(width, ansi.StringWidth(line))
	}
	return width
}

// Wrap hard-wraps ANSI-styled text by display cells and grapheme clusters.
// A non-positive width has no drawable columns and therefore returns no text.
func Wrap(value string, width int) string {
	if width <= 0 || value == "" {
		return ""
	}
	return ansi.Hardwrap(value, width, false)
}

// Paint bounds content to the requested terminal rectangle, pads every line to
// its full width, and paints the complete owned area with the theme's canvas.
// Zero/tiny dimensions are clamped and never passed into renderer arithmetic.
func (t Theme) Paint(content string, width, height int) string {
	width, height = Clamp(width), Clamp(height)
	if width == 0 || height == 0 {
		return ""
	}
	if !t.color {
		content = ansi.Strip(content)
	}

	wrapped := strings.Split(Wrap(content, width), "\n")
	if len(wrapped) > height {
		wrapped = wrapped[:height]
	}
	for len(wrapped) < height {
		wrapped = append(wrapped, "")
	}

	for i, line := range wrapped {
		line = ansi.Truncate(line, width, "")
		padding := width - ansi.StringWidth(line)
		if padding > 0 {
			line += strings.Repeat(" ", padding)
		}
		wrapped[i] = t.Canvas.Inline(true).Render(line)
	}
	return strings.Join(wrapped, "\n")
}
