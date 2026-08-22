package tagselector

import (
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// Run presents the selector on the provided streams. Cancellation always
// returns a zero Result with ErrCanceled.
func Run(in io.Reader, out io.Writer, browseData BrowseData, initial []string, preview PreviewFunc) (Result, error) {
	program := tea.NewProgram(
		New(browseData, initial, preview),
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	final, err := program.Run()
	if err != nil {
		return Result{}, err
	}
	model, ok := final.(Model)
	if !ok {
		return Result{}, errors.New("tag selector returned unexpected model")
	}
	if model.Canceled() {
		return Result{}, ErrCanceled
	}
	if !model.finished {
		return Result{}, errors.New("tag selector ended without confirmation")
	}
	return model.Result(), nil
}
