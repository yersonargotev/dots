package tagselector_test

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yersonargotev/dots/internal/tui/tagselector"
	"github.com/yersonargotev/dots/internal/tui/theme"
)

func TestSearchTextInputHandlesUnicodePasteBackspaceAndLiteralQ(t *testing.T) {
	browseData := tagselector.BrowseData{Tags: []tagselector.Tag{
		{Name: "café", Description: "Unicode editor", Group: "Development"},
		{Name: "shell", Description: "Terminal", Group: "Shell"},
	}}
	model := tagselector.NewWithTheme(browseData, nil, nil, theme.NoColor())
	model = update(t, model, key('/'), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("CAFÉq")})
	if model.Canceled() {
		t.Fatal("q typed into the focused search input canceled the selector")
	}
	model = update(t, model, tea.KeyMsg{Type: tea.KeyBackspace}, tea.KeyMsg{Type: tea.KeyEnter})
	view := model.View()
	if !strings.Contains(view, `Filter: CAFÉ`) || !strings.Contains(view, "café") || strings.Contains(view, "shell") {
		t.Fatalf("Unicode paste/backspace did not produce the accepted filter:\n%s", view)
	}

	model = update(t, model, key(' '))
	if got, want := model.SelectedTags(), []string{"café"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("toggle after Unicode filtering = %v, want %v", got, want)
	}
}

func TestSearchKeepsUpDownNavigationWithinFilteredTags(t *testing.T) {
	browseData := tagselector.BrowseData{Tags: []tagselector.Tag{
		{Name: "alpha", Description: "shared result"},
		{Name: "beta", Description: "shared result"},
	}}
	model := tagselector.NewWithTheme(browseData, nil, nil, theme.NoColor())
	model = update(t, model,
		key('/'),
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("shared")},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
		key(' '),
	)
	if got, want := model.SelectedTags(), []string{"beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("down in focused search selected %v, want %v", got, want)
	}
	model = update(t, model, key('/'), tea.KeyMsg{Type: tea.KeyUp}, tea.KeyMsg{Type: tea.KeyEnter}, key(' '))
	if got, want := model.SelectedTags(), []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("up in focused search selected %v, want %v", got, want)
	}
}

func TestClearTextInputHandlesPastedUnicodeAndStillRequiresExactASCII(t *testing.T) {
	wantPreview := tagselector.Preview{Text: "remove everything", Confirmation: tagselector.ConfirmationClear}
	model := tagselector.NewWithTheme(testBrowseData(), []string{"zsh"}, func(uint64, []string) (tagselector.Preview, error) {
		return wantPreview, nil
	}, theme.NoColor())
	model = update(t, model, key(' '))
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = update(t, next.(tagselector.Model), previewCommand(t, command)(), tea.KeyMsg{Type: tea.KeyEnter})

	model = update(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("clear🙂")})
	next, quit := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	if quit != nil || !reflect.DeepEqual(model.Result(), tagselector.Result{}) {
		t.Fatal("a Unicode-suffixed clear phrase was accepted")
	}
	model = update(t, model, tea.KeyMsg{Type: tea.KeyBackspace})
	next, quit = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	if quit == nil {
		t.Fatal("exact lowercase ASCII clear was not accepted after Unicode backspace")
	}
	if got := model.Result(); got.Tags == nil || len(got.Tags) != 0 || got.Preview != wantPreview || !got.AcknowledgementAccepted {
		t.Fatalf("accepted clear Result() = %#v", got)
	}
}

func TestPreviewCommandBatchesSpinnerAndProviderAndSpinnerIsPendingOnly(t *testing.T) {
	model := tagselector.NewWithTheme(testBrowseData(), []string{"zsh"}, func(uint64, []string) (tagselector.Preview, error) {
		return tagselector.Preview{Text: "stable preview"}, nil
	}, theme.NoColor())
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	message := command()
	commands, ok := message.(tea.BatchMsg)
	if !ok || len(commands) != 2 {
		t.Fatalf("preview command = %T with %d batched commands, want spinner and provider", message, len(commands))
	}
	if view := model.View(); !strings.Contains(view, "Building the pending Selection Reconciliation Plan") {
		t.Fatalf("pending view does not show specific spinner copy:\n%s", view)
	}
	model = update(t, model, commands[len(commands)-1]())
	if view := model.View(); strings.Contains(view, "Building the pending Selection Reconciliation Plan") || !strings.Contains(view, "stable preview") {
		t.Fatalf("spinner remained visible outside pending preview:\n%s", view)
	}
}

func TestNoColorRenderingRetainsSemanticLabelsWithoutANSI(t *testing.T) {
	model := tagselector.NewWithTheme(detailedBrowseData(), []string{"zsh"}, nil, theme.NoColor())
	model = update(t, model, tea.WindowSizeMsg{Width: 100, Height: 24})
	view := model.View()
	if strings.Contains(view, "\x1b[") {
		t.Fatalf("no-color view contains ANSI escapes: %q", view)
	}
	for _, want := range []string{"> [x] zsh", "drift", "Observed Status: drift", "Draft Change: unchanged"} {
		if !strings.Contains(view, want) {
			t.Fatalf("no-color view missing semantic cue %q:\n%s", want, view)
		}
	}
}

func TestEveryScreenSurvivesZeroTinyStandardAndWideResizes(t *testing.T) {
	preview := tagselector.Preview{Text: strings.Repeat("plan line\n", 20), Confirmation: tagselector.ConfirmationReduction}
	preparations := map[string]func(t *testing.T) tagselector.Model{
		"browse": func(t *testing.T) tagselector.Model {
			return tagselector.NewWithTheme(detailedBrowseData(), []string{"zsh"}, nil, theme.NoColor())
		},
		"search": func(t *testing.T) tagselector.Model {
			return update(t, tagselector.NewWithTheme(detailedBrowseData(), nil, nil, theme.NoColor()), key('/'))
		},
		"profiles": func(t *testing.T) tagselector.Model {
			data := detailedBrowseData()
			data.Profiles = []tagselector.Profile{{Name: "core", Tags: []string{"zsh"}}}
			return update(t, tagselector.NewWithTheme(data, nil, nil, theme.NoColor()), key('p'))
		},
		"detail": func(t *testing.T) tagselector.Model {
			return update(t, tagselector.NewWithTheme(detailedBrowseData(), nil, nil, theme.NoColor()), key('d'))
		},
		"loading": func(t *testing.T) tagselector.Model {
			model := tagselector.NewWithTheme(testBrowseData(), []string{"zsh"}, func(uint64, []string) (tagselector.Preview, error) {
				return preview, nil
			}, theme.NoColor())
			next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return next.(tagselector.Model)
		},
		"preview": func(t *testing.T) tagselector.Model {
			return reviewedNoColor(t, tagselector.Preview{Text: strings.Repeat("plan line\n", 20)})
		},
		"reduction": func(t *testing.T) tagselector.Model {
			return update(t, reviewedNoColor(t, preview), tea.KeyMsg{Type: tea.KeyEnter})
		},
		"clear": func(t *testing.T) tagselector.Model {
			model := tagselector.NewWithTheme(testBrowseData(), []string{"zsh"}, func(uint64, []string) (tagselector.Preview, error) {
				return tagselector.Preview{Text: strings.Repeat("remove\n", 20), Confirmation: tagselector.ConfirmationClear}, nil
			}, theme.NoColor())
			model = update(t, model, key(' '))
			next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return update(t, next.(tagselector.Model), previewCommand(t, command)(), tea.KeyMsg{Type: tea.KeyEnter})
		},
	}
	sizes := []tea.WindowSizeMsg{{Width: 0, Height: 0}, {Width: 12, Height: 4}, {Width: 80, Height: 24}, {Width: 140, Height: 32}}
	for name, prepare := range preparations {
		t.Run(name, func(t *testing.T) {
			model := prepare(t)
			for _, size := range sizes {
				model = update(t, model, size)
				assertBoundedView(t, model.View(), size.Width, size.Height)
			}
			next, quit := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			model = next.(tagselector.Model)
			if quit == nil || !model.Canceled() || !reflect.DeepEqual(model.Result(), tagselector.Result{}) {
				t.Fatal("screen was not operable after repeated resize")
			}
		})
	}
}

func TestClearAndReductionKeepConsequenceAndActionVisibleAtTinyAndStandardSizes(t *testing.T) {
	clearModel := tagselector.NewWithTheme(testBrowseData(), []string{"zsh"}, func(uint64, []string) (tagselector.Preview, error) {
		return tagselector.Preview{Text: strings.Repeat("remove\n", 10), Confirmation: tagselector.ConfirmationClear}, nil
	}, theme.NoColor())
	clearModel = update(t, clearModel, key(' '))
	next, command := clearModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	clearModel = update(t, next.(tagselector.Model), previewCommand(t, command)(), tea.KeyMsg{Type: tea.KeyEnter})
	for _, test := range []struct {
		size tea.WindowSizeMsg
		want []string
	}{
		{size: tea.WindowSizeMsg{Width: 12, Height: 4}, want: []string{"CLEAR ALL", "destructive", "clear:", "enter"}},
		{size: tea.WindowSizeMsg{Width: 80, Height: 24}, want: []string{"DESTRUCTIVE", "Managed Entries", "management:", "enter"}},
	} {
		model := update(t, clearModel, test.size)
		view := model.View()
		for _, want := range test.want {
			if !strings.Contains(view, want) {
				t.Fatalf("clear confirmation at %dx%d hid %q:\n%s", test.size.Width, test.size.Height, want, view)
			}
		}
	}

	reduction := update(t, reviewedNoColor(t, tagselector.Preview{Text: strings.Repeat("remove\n", 10), Confirmation: tagselector.ConfirmationReduction}), tea.KeyMsg{Type: tea.KeyEnter}, tea.WindowSizeMsg{Width: 12, Height: 4})
	for _, want := range []string{"REDUCTION", "destructive", "y/enter"} {
		if view := reduction.View(); !strings.Contains(view, want) {
			t.Fatalf("tiny reduction confirmation hid %q:\n%s", want, view)
		}
	}
}

func reviewedNoColor(t *testing.T, preview tagselector.Preview) tagselector.Model {
	t.Helper()
	model := tagselector.NewWithTheme(testBrowseData(), []string{"zsh"}, func(uint64, []string) (tagselector.Preview, error) {
		return preview, nil
	}, theme.NoColor())
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return update(t, next.(tagselector.Model), previewCommand(t, command)())
}

func assertBoundedView(t *testing.T, view string, width, height int) {
	t.Helper()
	if width <= 0 || height <= 0 {
		if view != "" {
			t.Fatalf("zero-area view = %q, want empty", view)
		}
		return
	}
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("view has %d lines, terminal height is %d:\n%s", len(lines), height, view)
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("view line width = %d, terminal width is %d: %q", got, width, line)
		}
	}
}
