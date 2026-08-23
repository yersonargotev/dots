package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yersonargotev/dots/internal/install"
)

type fakeConflictProgram struct {
	final tea.Model
	err   error
}

func (f fakeConflictProgram) Run() (tea.Model, error) {
	return f.final, f.err
}

func TestResolveConflictProgramRejectsUnconfirmedDraft(t *testing.T) {
	m := New(sampleConflicts(), nil)
	m = update(t, m, key('r'))
	if got := m.Decisions()["/home/u/.gitconfig"]; got != install.DecisionReplace {
		t.Fatalf("Decisions() draft = %q, want replace", got)
	}

	decisions, err := resolveConflictProgram(fakeConflictProgram{final: m})
	if decisions != nil || !errors.Is(err, ErrCanceled) {
		t.Fatalf("unconfirmed finalization = (%v, %v), want (nil, ErrCanceled)", decisions, err)
	}
}

func TestDiffQuitKeysCancelAndExposeNoAuthority(t *testing.T) {
	for _, cancelKey := range []tea.KeyMsg{{Type: tea.KeyEsc}, key('q')} {
		t.Run(cancelKey.String(), func(t *testing.T) {
			m := New(sampleConflicts(), func(Conflict) string { return "diff" })
			m = update(t, m, key('r'), key('d'), cancelKey)
			if !m.Canceled() || m.confirmed {
				t.Fatalf("%s in diff = canceled %v, confirmed %v; want true, false", cancelKey.String(), m.Canceled(), m.confirmed)
			}
			if decisions := m.Decisions(); len(decisions) != 0 {
				t.Fatalf("%s in diff exposed decisions: %v", cancelKey.String(), decisions)
			}

			decisions, err := resolveConflictProgram(fakeConflictProgram{final: m})
			if decisions != nil || !errors.Is(err, ErrCanceled) {
				t.Fatalf("%s wrapper result = (%v, %v), want (nil, ErrCanceled)", cancelKey.String(), decisions, err)
			}
		})
	}
}

func TestResolveConflictProgramReturnsConfirmedDecisions(t *testing.T) {
	m := New(sampleConflicts(), nil)
	m = update(t, m, key('r'), tea.KeyMsg{Type: tea.KeyEnter})

	decisions, err := resolveConflictProgram(fakeConflictProgram{final: m})
	if err != nil {
		t.Fatalf("confirmed finalization error = %v", err)
	}
	if got := decisions["/home/u/.gitconfig"]; got != install.DecisionReplace {
		t.Fatalf("confirmed decision = %q, want replace", got)
	}
}

func TestResolveConflictProgramPropagatesRunError(t *testing.T) {
	want := errors.New("program failed")
	decisions, err := resolveConflictProgram(fakeConflictProgram{err: want})
	if decisions != nil || err != want {
		t.Fatalf("run error result = (%v, %v), want (nil, same error)", decisions, err)
	}
}

func TestResolveConflictProgramRejectsUnexpectedModel(t *testing.T) {
	decisions, err := resolveConflictProgram(fakeConflictProgram{})
	if decisions != nil || err == nil || !strings.Contains(err.Error(), "unexpected model") {
		t.Fatalf("unexpected model result = (%v, %v), want nil/error", decisions, err)
	}
}

func TestResolveConflictsWithNoConflictsReturnsEmptyMap(t *testing.T) {
	decisions, err := ResolveConflicts(strings.NewReader(""), &strings.Builder{}, nil, nil)
	if err != nil || decisions == nil || len(decisions) != 0 {
		t.Fatalf("ResolveConflicts(no conflicts) = (%v, %v), want non-nil empty map", decisions, err)
	}
}
