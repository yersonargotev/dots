package tagselector

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type stubProgramRunner struct {
	model tea.Model
	err   error
}

func (s stubProgramRunner) Run() (tea.Model, error) { return s.model, s.err }

type unexpectedFinalModel struct{}

func (unexpectedFinalModel) Init() tea.Cmd { return nil }
func (model unexpectedFinalModel) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return model, nil
}
func (unexpectedFinalModel) View() string { return "" }

func TestRunProgramErrorReturnsZeroResultAndSameError(t *testing.T) {
	wantErr := errors.New("program failed")
	result, err := runProgram(stubProgramRunner{err: wantErr})
	if err != wantErr {
		t.Fatalf("runProgram() error = %v, want exact %v", err, wantErr)
	}
	if !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("runProgram() result = %#v, want zero Result", result)
	}
}

func TestRunProgramUnexpectedFinalModelReturnsZeroResult(t *testing.T) {
	result, err := runProgram(stubProgramRunner{model: unexpectedFinalModel{}})
	if err == nil || !strings.Contains(err.Error(), "unexpected model") {
		t.Fatalf("runProgram() error = %v, want unexpected model error", err)
	}
	if !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("runProgram() result = %#v, want zero Result", result)
	}
}

func TestRunProgramUnconfirmedFinalModelReturnsZeroResult(t *testing.T) {
	result, err := runProgram(stubProgramRunner{model: New(BrowseData{}, nil, nil)})
	if err == nil || !strings.Contains(err.Error(), "without confirmation") {
		t.Fatalf("runProgram() error = %v, want unconfirmed termination error", err)
	}
	if !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("runProgram() result = %#v, want zero Result", result)
	}
}

func TestRunProgramCanceledFinalModelReturnsZeroResultAndErrCanceled(t *testing.T) {
	canceled := New(BrowseData{}, nil, nil).cancel()
	result, err := runProgram(stubProgramRunner{model: canceled})
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("runProgram() error = %v, want ErrCanceled", err)
	}
	if !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("runProgram() result = %#v, want zero Result", result)
	}
}

func TestRunProgramPreservesExplicitConfirmedEmptySelection(t *testing.T) {
	wantPreview := Preview{Text: "remove everything", SemanticDigest: "sha256:clear", CandidateToken: "candidate:clear", Confirmation: ConfirmationClear}
	confirmed := New(BrowseData{}, nil, nil)
	confirmed.finished = true
	confirmed.acknowledged = true
	confirmed.previewTags = make([]string, 0)
	confirmed.accepted = wantPreview

	result, err := runProgram(stubProgramRunner{model: confirmed})
	if err != nil {
		t.Fatalf("runProgram() error = %v", err)
	}
	if result.Tags == nil || len(result.Tags) != 0 {
		t.Fatalf("runProgram() Tags = %#v, want explicit non-nil empty selection", result.Tags)
	}
	if result.Preview != wantPreview || !result.AcknowledgementAccepted {
		t.Fatalf("runProgram() result = %#v, want exact confirmed clear", result)
	}
}
