package tagselector

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

func TestViewportsUseCanonicalNavigationBindings(t *testing.T) {
	model := New(BrowseData{}, nil, nil)
	viewports := map[string]struct {
		up       key.Binding
		down     key.Binding
		pageUp   key.Binding
		pageDown key.Binding
	}{
		"browse":   {model.browse.KeyMap.Up, model.browse.KeyMap.Down, model.browse.KeyMap.PageUp, model.browse.KeyMap.PageDown},
		"profiles": {model.profiles.KeyMap.Up, model.profiles.KeyMap.Down, model.profiles.KeyMap.PageUp, model.profiles.KeyMap.PageDown},
		"detail":   {model.detailView.KeyMap.Up, model.detailView.KeyMap.Down, model.detailView.KeyMap.PageUp, model.detailView.KeyMap.PageDown},
		"preview":  {model.previewView.KeyMap.Up, model.previewView.KeyMap.Down, model.previewView.KeyMap.PageUp, model.previewView.KeyMap.PageDown},
		"confirm":  {model.confirmView.KeyMap.Up, model.confirmView.KeyMap.Down, model.confirmView.KeyMap.PageUp, model.confirmView.KeyMap.PageDown},
	}
	for name, bindings := range viewports {
		t.Run(name, func(t *testing.T) {
			assertSameBinding(t, bindings.up, model.keys.Up)
			assertSameBinding(t, bindings.down, model.keys.Down)
			assertSameBinding(t, bindings.pageUp, model.keys.PageUp)
			assertSameBinding(t, bindings.pageDown, model.keys.PageDown)
		})
	}
}

func assertSameBinding(t *testing.T, got, want key.Binding) {
	t.Helper()
	if !reflect.DeepEqual(got.Keys(), want.Keys()) || got.Help() != want.Help() {
		t.Fatalf("binding = keys %v help %#v, want canonical keys %v help %#v", got.Keys(), got.Help(), want.Keys(), want.Help())
	}
}
