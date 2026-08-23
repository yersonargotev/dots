package theme

import (
	"math"
	"strings"
	"testing"

	catppuccin "github.com/catppuccin/go"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestDefaultSemanticPaletteAndContrast(t *testing.T) {
	theme := Default()
	m := catppuccin.Mocha

	roles := []struct {
		name  string
		style lipgloss.Style
		want  catppuccin.Color
	}{
		{name: "title", style: theme.Title, want: m.Text()},
		{name: "body", style: theme.Body, want: m.Text()},
		{name: "secondary", style: theme.Secondary, want: m.Subtext0()},
		{name: "selected", style: theme.Selected, want: m.Blue()},
		{name: "action", style: theme.Action, want: m.Blue()},
		{name: "success", style: theme.Success, want: m.Green()},
		{name: "warning", style: theme.Warning, want: m.Yellow()},
		{name: "caution", style: theme.Caution, want: m.Peach()},
		{name: "error", style: theme.Error, want: m.Red()},
		{name: "focus", style: theme.Focus, want: m.Rosewater()},
		{name: "alternate focus", style: theme.FocusAlt, want: m.Lavender()},
	}

	base := lipgloss.Color(m.Base().Hex)
	if got := theme.Canvas.GetBackground(); got != base {
		t.Fatalf("canvas background = %v, want Mocha Base %v", got, base)
	}
	for _, role := range roles {
		t.Run(role.name, func(t *testing.T) {
			if got, want := role.style.GetForeground(), lipgloss.Color(role.want.Hex); got != want {
				t.Errorf("foreground = %v, want %v", got, want)
			}
			if got := role.style.GetBackground(); got != base {
				t.Errorf("background = %v, want Mocha Base %v", got, base)
			}
			if got := contrast(role.want, m.Base()); got < 4.5 {
				t.Errorf("contrast = %.2f:1, want >= 4.5:1", got)
			}
		})
	}

	if got, want := theme.InactiveGeometry.GetForeground(), lipgloss.Color(m.Overlay0().Hex); got != want {
		t.Errorf("inactive geometry foreground = %v, want %v", got, want)
	}
	if got := contrast(m.Overlay0(), m.Base()); got >= 4.5 {
		t.Fatalf("test assumption failed: Overlay0 contrast = %.2f, want below essential-text threshold", got)
	}
}

func TestPaintUsesBaseForCompleteBoundedArea(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)

	got := Default().Paint("abc\n界", 5, 3)
	plain := stripANSI(got)
	if want := "abc  \n界   \n     "; plain != want {
		t.Fatalf("plain painted output = %q, want %q", plain, want)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("painted height = %d, want 3", len(lines))
	}
	baseSequence := "\x1b[48;2;30;30;46m"
	for i, line := range lines {
		if Width(line) != 5 {
			t.Errorf("line %d width = %d, want 5", i, Width(line))
		}
		if !strings.Contains(line, baseSequence) {
			t.Errorf("line %d does not paint its content/padding with Mocha Base: %q", i, line)
		}
	}
}

func TestDisplayWidthWrapAndDimensionClamping(t *testing.T) {
	styled := "\x1b[31m界ab\x1b[0m"
	if got := Width(styled); got != 4 {
		t.Fatalf("Width(styled wide text) = %d, want 4", got)
	}
	wrapped := Wrap(styled, 2)
	if got, want := stripANSI(wrapped), "界\nab"; got != want {
		t.Fatalf("Wrap(styled wide text) = %q, want %q", got, want)
	}
	for i, line := range strings.Split(wrapped, "\n") {
		if got := Width(line); got > 2 {
			t.Errorf("wrapped line %d width = %d, want <= 2", i, got)
		}
	}

	if got := Wrap("content", 0); got != "" {
		t.Errorf("Wrap at zero = %q, want empty", got)
	}
	if got := Default().Paint("content", -1, 3); got != "" {
		t.Errorf("Paint at negative width = %q, want empty", got)
	}
	if got := Default().Paint("content", 3, 0); got != "" {
		t.Errorf("Paint at zero height = %q, want empty", got)
	}
	if width, height := InnerSize(1, 2, 8, -1); width != 0 || height != 2 {
		t.Errorf("InnerSize tiny dimensions = (%d, %d), want (0, 2)", width, height)
	}
}

func TestNoColorNeutralizesBubblesStylesAndPreservesCues(t *testing.T) {
	theme := NoColor()

	helpModel := help.New()
	theme.ApplyHelp(&helpModel)
	if helpModel.ShortSeparator != " | " || helpModel.Ellipsis != "..." {
		t.Fatalf("no-color help retained non-ASCII defaults: separator %q, ellipsis %q", helpModel.ShortSeparator, helpModel.Ellipsis)
	}
	helpOutput := helpModel.ShortHelpView([]key.Binding{key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "apply"),
	)})

	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "filter"
	theme.ApplyTextInput(&input)
	inputOutput := input.View()

	spinnerModel := spinner.New(spinner.WithSpinner(spinner.Dot))
	theme.ApplySpinner(&spinnerModel)
	spinnerOutput := spinnerModel.View()
	for _, frame := range spinnerModel.Spinner.Frames {
		for _, r := range frame {
			if r > 127 {
				t.Fatalf("no-color spinner retained non-ASCII frame %q", frame)
			}
		}
	}

	painted := theme.Paint("\x1b[31m"+theme.Glyphs.Cursor+" "+theme.Glyphs.Checked+" selected\x1b[0m", 20, 1)
	all := helpOutput + inputOutput + spinnerOutput + painted
	if strings.Contains(all, "\x1b") {
		t.Fatalf("no-color output contains ANSI escapes: %q", all)
	}
	for _, cue := range []string{"enter", "apply", ">", "[x]", "selected"} {
		if !strings.Contains(all, cue) {
			t.Errorf("no-color output lost cue %q: %q", cue, all)
		}
	}

	styles := []lipgloss.Style{
		helpModel.Styles.Ellipsis,
		helpModel.Styles.ShortKey,
		helpModel.Styles.ShortDesc,
		helpModel.Styles.ShortSeparator,
		helpModel.Styles.FullKey,
		helpModel.Styles.FullDesc,
		helpModel.Styles.FullSeparator,
		input.PromptStyle,
		input.TextStyle,
		input.PlaceholderStyle,
		input.CompletionStyle,
		input.Cursor.Style,
		input.Cursor.TextStyle,
		input.CursorStyle,
		spinnerModel.Style,
	}
	for i, style := range styles {
		if rendered := style.Render("test"); strings.Contains(rendered, "\x1b") {
			t.Errorf("component style %d retained an ANSI-producing default: %q", i, rendered)
		}
	}
}

func contrast(foreground, background catppuccin.Color) float64 {
	lighter, darker := luminance(foreground), luminance(background)
	if lighter < darker {
		lighter, darker = darker, lighter
	}
	return (lighter + 0.05) / (darker + 0.05)
}

func luminance(c catppuccin.Color) float64 {
	channels := [3]float64{}
	for i, value := range c.RGB {
		channel := float64(value) / 255
		if channel <= 0.04045 {
			channels[i] = channel / 12.92
		} else {
			channels[i] = math.Pow((channel+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*channels[0] + 0.7152*channels[1] + 0.0722*channels[2]
}

func stripANSI(value string) string {
	// Width/Wrap use the same parser; this small test helper only strips SGR
	// sequences emitted by the deterministic Lip Gloss styles above.
	for {
		start := strings.Index(value, "\x1b[")
		if start < 0 {
			return value
		}
		rest := value[start+2:]
		end := strings.IndexByte(rest, 'm')
		if end < 0 {
			return value
		}
		value = value[:start] + rest[end+1:]
	}
}
