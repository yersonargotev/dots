package tui

import (
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yersonargotev/dots/internal/install"
)

// ErrCanceled is returned when the user aborts conflict resolution. Callers must
// treat it as "apply nothing", preserving the conservative safety model.
var ErrCanceled = errors.New("conflict resolution canceled")

// ResolveConflicts runs the Bubble Tea conflict resolver against the given
// input/output streams and returns the per-target decisions. With no conflicts
// it returns an empty map without starting a program. A canceled session
// returns ErrCanceled so the caller applies nothing.
func ResolveConflicts(in io.Reader, out io.Writer, conflicts []Conflict, diff DiffFunc) (map[string]install.ConflictDecision, error) {
	if len(conflicts) == 0 {
		return map[string]install.ConflictDecision{}, nil
	}

	program := tea.NewProgram(New(conflicts, diff), tea.WithInput(in), tea.WithOutput(out))
	final, err := program.Run()
	if err != nil {
		return nil, err
	}

	model, ok := final.(Model)
	if !ok {
		return nil, errors.New("conflict resolver returned unexpected model")
	}
	if model.Canceled() {
		return nil, ErrCanceled
	}
	return model.Decisions(), nil
}
