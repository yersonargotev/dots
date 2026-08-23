package tagselector_test

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/yersonargotev/dots/internal/tui/tagselector"
	"github.com/yersonargotev/dots/internal/tui/theme"
)

func TestTeaTestSuccessfulSelectionFlow(t *testing.T) {
	wantPreview := tagselector.Preview{
		Text:           "install zsh",
		SemanticDigest: "sha256:teatest",
		CandidateToken: "candidate:teatest",
		ForwardOnly:    true,
	}
	model := tagselector.NewWithTheme(testBrowseData(), []string{"zsh"}, func(uint64, []string) (tagselector.Preview, error) {
		return wantPreview, nil
	}, theme.NoColor())
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	teatest.WaitFor(t, tm.Output(), func(output []byte) bool {
		return bytes.Contains(output, []byte("selection preview")) && bytes.Contains(output, []byte("install zsh"))
	}, teatest.WithDuration(2*time.Second))
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	final := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second)).(tagselector.Model)
	if final.Canceled() {
		t.Fatal("successful teatest flow was canceled")
	}
	if got := final.Result(); !reflect.DeepEqual(got.Tags, []string{"zsh"}) || got.Preview != wantPreview || got.AcknowledgementAccepted {
		t.Fatalf("successful teatest Result() = %#v", got)
	}
}

func TestTeaTestCanceledSelectionFlow(t *testing.T) {
	model := tagselector.NewWithTheme(testBrowseData(), []string{"zsh"}, nil, theme.NoColor())
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

	final := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second)).(tagselector.Model)
	if !final.Canceled() {
		t.Fatal("canceled teatest flow did not report cancellation")
	}
	if got := final.Result(); !reflect.DeepEqual(got, tagselector.Result{}) {
		t.Fatalf("canceled teatest exposed Result() = %#v", got)
	}
}
