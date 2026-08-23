package theme

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
)

// ApplyHelp replaces every style in a help model. This prevents Bubbles'
// adaptive default colors from leaking into either TUI, including no-color
// rendering.
func (t Theme) ApplyHelp(model *help.Model) {
	if model == nil {
		return
	}
	model.Styles = help.Styles{
		Ellipsis:       t.InactiveGeometry,
		ShortKey:       t.Action,
		ShortDesc:      t.Secondary,
		ShortSeparator: t.InactiveGeometry,
		FullKey:        t.Action,
		FullDesc:       t.Secondary,
		FullSeparator:  t.InactiveGeometry,
	}
	if !t.color {
		model.ShortSeparator = " | "
		model.FullSeparator = "    "
		model.Ellipsis = "..."
	}
}

// ApplyTextInput replaces every style in a text input, including both current
// and deprecated cursor fields retained by Bubbles v1.
func (t Theme) ApplyTextInput(model *textinput.Model) {
	if model == nil {
		return
	}
	model.PromptStyle = t.Focus
	model.TextStyle = t.Body
	model.PlaceholderStyle = t.Secondary
	model.CompletionStyle = t.Secondary
	model.Cursor.Style = t.FocusAlt
	model.Cursor.TextStyle = t.Body
	model.CursorStyle = t.FocusAlt
}

// ApplySpinner replaces the spinner's otherwise unstyled/default style.
func (t Theme) ApplySpinner(model *spinner.Model) {
	if model == nil {
		return
	}
	model.Style = t.Action
	if !t.color {
		model.Spinner = spinner.Line
	}
}
