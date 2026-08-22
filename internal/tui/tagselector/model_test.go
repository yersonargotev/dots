package tagselector_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yersonargotev/dots/internal/tui/tagselector"
)

func key(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestPreviewReceivesCanonicalSnapshotAndFinishesWithExactOpaqueValue(t *testing.T) {
	wantPreview := tagselector.Preview{Text: "install zsh\nretain fzf", SemanticDigest: "sha256:opaque", ForwardOnly: true}
	var received []string
	model := tagselector.New(testBrowseData(), []string{"nvim", "zsh"}, func(tags []string) (tagselector.Preview, error) {
		received = append([]string(nil), tags...)
		tags[0] = "mutated-by-provider"
		return wantPreview, nil
	})

	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	if command == nil || !strings.Contains(model.View(), "Loading preview") {
		t.Fatalf("enter should start asynchronous preview, view:\n%s", model.View())
	}
	message := command()
	next, _ = model.Update(message)
	model = next.(tagselector.Model)
	if got, want := received, []string{"zsh", "nvim"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("preview input = %v, want canonical snapshot %v", got, want)
	}
	if got := model.Preview(); got != wantPreview {
		t.Fatalf("Preview() = %#v, want exact opaque %#v", got, wantPreview)
	}
	view := model.View()
	if !strings.Contains(view, wantPreview.Text) || !strings.Contains(view, "Forward-only") {
		t.Fatalf("preview view missing opaque text or forward-only label:\n%s", view)
	}
	if got, want := model.SelectedTags(), []string{"zsh", "nvim"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("provider mutation leaked into desired Tags: %v, want %v", got, want)
	}

	next, quit := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	if quit == nil {
		t.Fatal("enter in preview should finish with a quit command")
	}
	result := model.Result()
	if !reflect.DeepEqual(result.Tags, []string{"zsh", "nvim"}) || result.Preview != wantPreview {
		t.Fatalf("Result() = %#v, want detached Tags and exact preview", result)
	}
	result.Tags[0] = "changed"
	if got := model.Result().Tags[0]; got != "zsh" {
		t.Fatalf("Result().Tags alias model storage, got %q", got)
	}
}

func TestLoadingIgnoresRepeatedEnterSoEachRequestCallsPreviewOnce(t *testing.T) {
	var calls atomic.Int32
	model := tagselector.New(testBrowseData(), nil, func([]string) (tagselector.Preview, error) {
		calls.Add(1)
		return tagselector.Preview{Text: "ok"}, nil
	})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	next, repeated := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	if repeated != nil {
		t.Fatal("enter while loading must not create another command for the same request")
	}
	message := command()
	model = update(t, model, message)
	if got := calls.Load(); got != 1 {
		t.Fatalf("preview calls = %d, want one for one request ID", got)
	}
}

func TestPreviewErrorReturnsToDraftAndAllowsRetry(t *testing.T) {
	var calls atomic.Int32
	model := tagselector.New(testBrowseData(), []string{"zsh"}, func([]string) (tagselector.Preview, error) {
		if calls.Add(1) == 1 {
			return tagselector.Preview{}, errors.New("preview failed")
		}
		return tagselector.Preview{Text: "retry succeeded", SemanticDigest: "sha256:retry"}, nil
	})

	next, first := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	model = update(t, model, first())
	if view := model.View(); !strings.Contains(view, "Preview error: preview failed") || !strings.Contains(view, "[x] zsh") {
		t.Fatalf("preview error should return to the unchanged draft:\n%s", view)
	}
	if got := model.Preview(); got != (tagselector.Preview{}) {
		t.Fatalf("failed preview exposed usable value: %#v", got)
	}

	next, retry := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = update(t, next.(tagselector.Model), retry())
	if got := model.Preview(); got.Text != "retry succeeded" || got.SemanticDigest != "sha256:retry" {
		t.Fatalf("retry Preview() = %#v, want successful opaque preview", got)
	}
}

func TestNewerPreviewWinsWhenBCompletesBeforeA(t *testing.T) {
	aStarted := make(chan struct{})
	aRelease := make(chan struct{})
	preview := func(tags []string) (tagselector.Preview, error) {
		if reflect.DeepEqual(tags, []string{"zsh"}) {
			close(aStarted)
			<-aRelease
			return tagselector.Preview{Text: "A"}, nil
		}
		return tagselector.Preview{Text: "B", SemanticDigest: "b"}, nil
	}
	model := tagselector.New(testBrowseData(), []string{"zsh"}, preview)
	next, commandA := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	messagesA := make(chan tea.Msg, 1)
	go func() { messagesA <- commandA() }()
	<-aStarted

	model = update(t, model, tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyDown}, key(' '))
	next, commandB := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	model = update(t, model, commandB())
	if got := model.Preview().Text; got != "B" {
		t.Fatalf("accepted Preview = %q, want B", got)
	}

	close(aRelease)
	model = update(t, model, <-messagesA)
	if got := model.Preview().Text; got != "B" {
		t.Fatalf("stale A replaced accepted B, got %q", got)
	}
}

func TestStalePreviewErrorAfterCancelCannotProduceUsableResult(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	model := tagselector.New(testBrowseData(), []string{"zsh"}, func([]string) (tagselector.Preview, error) {
		close(started)
		<-release
		return tagselector.Preview{}, errors.New("late failure")
	})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(tagselector.Model)
	messages := make(chan tea.Msg, 1)
	go func() { messages <- command() }()
	<-started
	next, quit := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = next.(tagselector.Model)
	if quit == nil || !model.Canceled() {
		t.Fatal("ctrl+c while loading should cancel and quit immediately")
	}
	close(release)
	model = update(t, model, <-messages)
	if got := model.Result(); got.Tags != nil || got.Preview != (tagselector.Preview{}) {
		t.Fatalf("canceled model exposed usable result: %#v", got)
	}
}

func TestCtrlCCancelsFromEveryScreenAndClearsUsableValues(t *testing.T) {
	preparations := map[string]func(t *testing.T) tagselector.Model{
		"list": func(t *testing.T) tagselector.Model {
			return tagselector.New(detailedBrowseData(), nil, nil)
		},
		"search": func(t *testing.T) tagselector.Model {
			return update(t, tagselector.New(detailedBrowseData(), nil, nil), key('/'))
		},
		"profiles": func(t *testing.T) tagselector.Model {
			browseData := detailedBrowseData()
			browseData.Profiles = []tagselector.Profile{{Name: "core", Tags: []string{"zsh"}}}
			return update(t, tagselector.New(browseData, nil, nil), key('p'))
		},
		"detail": func(t *testing.T) tagselector.Model {
			return update(t, tagselector.New(detailedBrowseData(), nil, nil), tea.WindowSizeMsg{Width: 80}, key('d'))
		},
		"loading": func(t *testing.T) tagselector.Model {
			model := tagselector.New(detailedBrowseData(), nil, func([]string) (tagselector.Preview, error) {
				return tagselector.Preview{Text: "preview"}, nil
			})
			next, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return next.(tagselector.Model)
		},
		"preview": func(t *testing.T) tagselector.Model {
			model := tagselector.New(detailedBrowseData(), []string{"zsh"}, func([]string) (tagselector.Preview, error) {
				return tagselector.Preview{Text: "preview"}, nil
			})
			next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return update(t, next.(tagselector.Model), command())
		},
	}
	for name, prepare := range preparations {
		t.Run(name, func(t *testing.T) {
			model := prepare(t)
			next, quit := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			model = next.(tagselector.Model)
			if quit == nil || !model.Canceled() {
				t.Fatal("ctrl+c should cancel and quit")
			}
			if got := model.Result(); got.Tags != nil || got.Preview != (tagselector.Preview{}) {
				t.Fatalf("canceled model exposed %#v", got)
			}
		})
	}
}

func TestRunCancellationReturnsOnlyErrCanceled(t *testing.T) {
	result, err := tagselector.Run(bytes.NewBuffer([]byte{3}), io.Discard, testBrowseData(), []string{"zsh"}, nil)
	if !errors.Is(err, tagselector.ErrCanceled) {
		t.Fatalf("Run() error = %v, want ErrCanceled", err)
	}
	if result.Tags != nil || result.Preview != (tagselector.Preview{}) {
		t.Fatalf("Run() canceled result = %#v, want unusable zero value", result)
	}
}

func TestRunCtrlCDoesNotWaitForInFlightPreview(t *testing.T) {
	input, keys := io.Pipe()
	defer input.Close()
	defer keys.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	type outcome struct {
		result tagselector.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := tagselector.Run(input, io.Discard, testBrowseData(), []string{"zsh"}, func([]string) (tagselector.Preview, error) {
			close(started)
			<-release
			return tagselector.Preview{Text: "too late"}, nil
		})
		done <- outcome{result: result, err: err}
	}()
	if _, err := keys.Write([]byte{'\r'}); err != nil {
		t.Fatalf("write preview key: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("preview did not start")
	}
	if _, err := keys.Write([]byte{3}); err != nil {
		t.Fatalf("write ctrl+c: %v", err)
	}
	select {
	case got := <-done:
		if !errors.Is(got.err, tagselector.ErrCanceled) || got.result.Tags != nil || got.result.Preview != (tagselector.Preview{}) {
			t.Fatalf("Run() = (%#v, %v), want zero Result and ErrCanceled", got.result, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run blocked on in-flight preview after ctrl+c")
	}
}

func TestListGroupsTagsAndSearchesNameOrDescriptionCaseInsensitively(t *testing.T) {
	model := tagselector.New(testBrowseData(), []string{"zsh"}, nil)
	view := model.View()
	for _, want := range []string{"Shell", "Development", "[x] zsh", "[ ] git"} {
		if !strings.Contains(view, want) {
			t.Fatalf("list view missing %q:\n%s", want, view)
		}
	}

	model = update(t, model, key('/'), key('V'), key('E'), key('R'), key('S'), key('I'), key('O'), key('N'))
	view = model.View()
	if !strings.Contains(view, "git") || strings.Contains(view, "zsh") || strings.Contains(view, "nvim") {
		t.Fatalf("description search should show only git:\n%s", view)
	}

	model = update(t, model, tea.KeyMsg{Type: tea.KeyEsc}, key('/'), key('N'), key('V'), key('I'), key('M'))
	view = model.View()
	if !strings.Contains(view, "nvim") || strings.Contains(view, "git") {
		t.Fatalf("name search should show only nvim:\n%s", view)
	}
}

func TestAcceptedSearchFilterTargetsTheVisibleTag(t *testing.T) {
	model := tagselector.New(testBrowseData(), nil, nil)
	model = update(t, model, key('/'), key('V'), key('E'), key('R'), tea.KeyMsg{Type: tea.KeyEnter}, key(' '))
	if got, want := model.SelectedTags(), []string{"git"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("toggle after filtered search = %v, want visible Tag %v", got, want)
	}
}

func update(t *testing.T, model tagselector.Model, messages ...tea.Msg) tagselector.Model {
	t.Helper()
	for _, message := range messages {
		next, _ := model.Update(message)
		model = next.(tagselector.Model)
	}
	return model
}

func testBrowseData() tagselector.BrowseData {
	return tagselector.BrowseData{Tags: []tagselector.Tag{
		{Name: "zsh", Description: "Shell setup", Group: "Shell"},
		{Name: "git", Description: "Version control", Group: "Development"},
		{Name: "nvim", Description: "Editor", Group: "Development"},
	}}
}

func TestSelectedTagsStayInCanonicalBrowseOrderAndAreDetached(t *testing.T) {
	browseData := testBrowseData()
	model := tagselector.New(browseData, []string{"nvim", "unknown", "zsh"}, nil)

	if got, want := model.SelectedTags(), []string{"zsh", "nvim"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SelectedTags() = %v, want canonical %v", got, want)
	}

	first := model.SelectedTags()
	first[0] = "changed"
	browseData.Tags[0].Name = "also-changed"
	if got, want := model.SelectedTags(), []string{"zsh", "nvim"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SelectedTags() after caller mutation = %v, want detached %v", got, want)
	}

	model = update(t, model, key(' '), tea.KeyMsg{Type: tea.KeyDown}, key(' '))
	if got, want := model.SelectedTags(), []string{"git", "nvim"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SelectedTags() after reverse toggles = %v, want canonical browse order %v", got, want)
	}
}

func TestProfilePresetSelectsOrDeselectsAllWithoutChangingUnrelatedTags(t *testing.T) {
	browseData := testBrowseData()
	browseData.Profiles = []tagselector.Profile{
		{Name: "workstation", Description: "Everyday workstation", Tags: []string{"nvim", "zsh"}},
		{Name: "agents", Description: "Agent tools", Tags: []string{"nvim", "git"}},
	}
	model := tagselector.New(browseData, []string{"git"}, nil)
	model = update(t, model, key('p'))
	if view := model.View(); !strings.Contains(view, "[ ] workstation") || !strings.Contains(view, "Everyday workstation") {
		t.Fatalf("profiles view missing preset metadata:\n%s", view)
	}

	model = update(t, model, key(' '))
	if view := model.View(); !strings.Contains(view, "[x] workstation") {
		t.Fatalf("selected preset is not reflected in profile view:\n%s", view)
	}
	if got, want := model.SelectedTags(), []string{"zsh", "git", "nvim"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first preset toggle = %v, want all preset Tags plus unrelated %v", got, want)
	}
	model = update(t, model, tea.KeyMsg{Type: tea.KeyEnter})
	if view := model.View(); !strings.Contains(view, "[ ] workstation") {
		t.Fatalf("deselected preset is not reflected in profile view:\n%s", view)
	}
	if got, want := model.SelectedTags(), []string{"git"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second preset toggle = %v, want only unrelated %v", got, want)
	}

	model = update(t, model, tea.KeyMsg{Type: tea.KeyDown}, key(' '))
	if got, want := model.SelectedTags(), []string{"git", "nvim"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("overlapping preset toggle = %v, want deterministic select-all %v", got, want)
	}
}

func detailedBrowseData() tagselector.BrowseData {
	return tagselector.BrowseData{Tags: []tagselector.Tag{{
		Name:                   "zsh",
		Description:            "Portable shell configuration",
		Group:                  "Shell",
		Profiles:               []string{"core", "workstation"},
		ManagedEntries:         []string{"zshrc", "starship"},
		Dependencies:           []string{"starship", "fzf"},
		Provisioners:           []string{"zinit"},
		State:                  tagselector.StateDrift,
		ExternalEffectsPresent: true,
		Components: []tagselector.Component{
			{Kind: "Managed Entry", Name: "zshrc", State: tagselector.StateConflict, Detail: "local target differs"},
			{Kind: "Dependency", Name: "fzf", State: tagselector.StateMissing, Detail: "command not found"},
		},
	}}}
}

func assertDetailFields(t *testing.T, view, draft, externalEffect string) {
	t.Helper()
	for _, want := range []string{
		"Description: Portable shell configuration",
		"Profiles: core, workstation",
		"Managed Entries: zshrc, starship",
		"Dependencies: starship, fzf",
		"Provisioners: zinit",
		"Observed Status: drift",
		"Managed Entry · zshrc · conflict · local target differs",
		"Dependency · fzf · missing · command not found",
		externalEffect,
		"Draft Change: " + draft,
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail view missing %q:\n%s", want, view)
		}
	}
}

func TestWideListShowsDetailsAndDesiredCheckboxDoesNotReplaceObservedState(t *testing.T) {
	model := tagselector.New(detailedBrowseData(), []string{"zsh"}, nil)
	model = update(t, model, tea.WindowSizeMsg{Width: 100, Height: 30})
	view := model.View()
	if !strings.Contains(view, "[x] zsh") || !strings.Contains(view, "(drift)") {
		t.Fatalf("desired checkbox and observed state must both be visible:\n%s", view)
	}
	assertDetailFields(t, view, "unchanged", "External Effects Present")
	if strings.Contains(view, "Retained External State") {
		t.Fatalf("unchanged Tag misclassified current external effects as selection residue:\n%s", view)
	}

	model = update(t, model, key(' '))
	assertDetailFields(t, model.View(), "remove", "Retained External State [retained]")
}

func TestNarrowListOpensSeparateDetailWithDOrRight(t *testing.T) {
	for _, open := range []tea.KeyMsg{key('d'), {Type: tea.KeyRight}} {
		t.Run(open.String(), func(t *testing.T) {
			model := tagselector.New(detailedBrowseData(), nil, nil)
			model = update(t, model, tea.WindowSizeMsg{Width: 99, Height: 30})
			if strings.Contains(model.View(), "Managed Entries:") {
				t.Fatalf("narrow list should not inline details:\n%s", model.View())
			}
			model = update(t, model, key(' '), open)
			assertDetailFields(t, model.View(), "add", "External Effects Present")
			model = update(t, model, tea.KeyMsg{Type: tea.KeyEsc})
			if strings.Contains(model.View(), "Managed Entries:") {
				t.Fatalf("esc should return from narrow detail to list:\n%s", model.View())
			}
		})
	}
}

func TestLargeCatalogKeepsCursorVisibleWithinTerminalHeight(t *testing.T) {
	browseData := tagselector.BrowseData{}
	for i := range 30 {
		browseData.Tags = append(browseData.Tags, tagselector.Tag{
			Name: fmt.Sprintf("tag-%02d", i), Group: fmt.Sprintf("Group %d", i/5), State: tagselector.StateAligned,
		})
	}
	for _, width := range []int{80, 120} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			model := tagselector.New(browseData, nil, nil)
			model = update(t, model, tea.WindowSizeMsg{Width: width, Height: 10})
			for range 21 {
				model = update(t, model, tea.KeyMsg{Type: tea.KeyDown})
			}
			view := model.View()
			if !strings.Contains(view, "> [ ] tag-21") {
				t.Fatalf("cursor Tag is outside viewport:\n%s", view)
			}
			if strings.Contains(view, "tag-00") {
				t.Fatalf("viewport still renders the beginning of a large catalog:\n%s", view)
			}
			if lines := renderedLineCount(view); lines > 10 {
				t.Fatalf("rendered lines = %d, want at most terminal height 10\n%s", lines, view)
			}
		})
	}
}

func TestLongPreviewScrollsWithinTerminalHeight(t *testing.T) {
	var previewText strings.Builder
	for i := 1; i <= 24; i++ {
		fmt.Fprintf(&previewText, "plan line %02d\n", i)
	}
	model := tagselector.New(testBrowseData(), nil, func([]string) (tagselector.Preview, error) {
		return tagselector.Preview{Text: previewText.String(), SemanticDigest: "sha256:long"}, nil
	})
	model = update(t, model, tea.WindowSizeMsg{Width: 80, Height: 8})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = update(t, next.(tagselector.Model), command())
	if view := model.View(); !strings.Contains(view, "plan line 01") || strings.Contains(view, "plan line 24") || !strings.Contains(view, "↓ more") {
		t.Fatalf("preview should start at the first bounded page:\n%s", view)
	}
	for range 20 {
		model = update(t, model, tea.KeyMsg{Type: tea.KeyPgDown})
	}
	view := model.View()
	if !strings.Contains(view, "plan line 24") || strings.Contains(view, "plan line 01") || !strings.Contains(view, "↑ more") {
		t.Fatalf("preview did not scroll to its final page:\n%s", view)
	}
	if lines := renderedLineCount(view); lines > 8 {
		t.Fatalf("rendered preview lines = %d, want at most terminal height 8\n%s", lines, view)
	}
}

func TestLongNarrowDetailScrollsWithinTerminalHeight(t *testing.T) {
	browseData := detailedBrowseData()
	for i := range 20 {
		browseData.Tags[0].Components = append(browseData.Tags[0].Components, tagselector.Component{
			Kind: "Dependency", Name: fmt.Sprintf("component-%02d", i), State: tagselector.StateAligned,
		})
	}
	model := tagselector.New(browseData, nil, nil)
	model = update(t, model, tea.WindowSizeMsg{Width: 80, Height: 8}, key('d'))
	if view := model.View(); strings.Contains(view, "component-19") || !strings.Contains(view, "↓ more") {
		t.Fatalf("detail should start at the first bounded page:\n%s", view)
	}
	for range 20 {
		model = update(t, model, tea.KeyMsg{Type: tea.KeyPgDown})
	}
	view := model.View()
	if !strings.Contains(view, "component-19") || !strings.Contains(view, "↑ more") {
		t.Fatalf("detail did not scroll to its final page:\n%s", view)
	}
	if lines := renderedLineCount(view); lines > 8 {
		t.Fatalf("rendered detail lines = %d, want at most terminal height 8\n%s", lines, view)
	}
}

func renderedLineCount(view string) int {
	return len(strings.Split(strings.TrimSuffix(view, "\n"), "\n"))
}
