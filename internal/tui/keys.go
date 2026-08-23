package tui

import (
	bubbleskey "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
)

type conflictKeyMap struct {
	globalCancel bubbleskey.Binding
	list         listKeyMap
	diff         diffKeyMap
}

type listKeyMap struct {
	up        bubbleskey.Binding
	down      bubbleskey.Binding
	first     bubbleskey.Binding
	last      bubbleskey.Binding
	skip      bubbleskey.Binding
	replace   bubbleskey.Binding
	adopt     bubbleskey.Binding
	diff      bubbleskey.Binding
	apply     bubbleskey.Binding
	cancel    bubbleskey.Binding
	cancelAll bubbleskey.Binding
}

func (k listKeyMap) ShortHelp() []bubbleskey.Binding {
	return []bubbleskey.Binding{k.up, k.down, k.first, k.last, k.skip, k.replace, k.adopt, k.diff, k.apply, k.cancel, k.cancelAll}
}

func (k listKeyMap) FullHelp() [][]bubbleskey.Binding {
	return [][]bubbleskey.Binding{
		{k.up, k.down, k.first, k.last},
		{k.skip, k.replace, k.adopt, k.diff},
		{k.apply, k.cancel, k.cancelAll},
	}
}

type diffKeyMap struct {
	up        bubbleskey.Binding
	down      bubbleskey.Binding
	pageUp    bubbleskey.Binding
	pageDown  bubbleskey.Binding
	first     bubbleskey.Binding
	last      bubbleskey.Binding
	close     bubbleskey.Binding
	cancel    bubbleskey.Binding
	cancelAll bubbleskey.Binding
}

func (k diffKeyMap) ShortHelp() []bubbleskey.Binding {
	return []bubbleskey.Binding{k.up, k.down, k.pageUp, k.pageDown, k.first, k.last, k.close, k.cancel, k.cancelAll}
}

func (k diffKeyMap) FullHelp() [][]bubbleskey.Binding {
	return [][]bubbleskey.Binding{
		{k.up, k.down, k.pageUp, k.pageDown},
		{k.first, k.last, k.close, k.cancel, k.cancelAll},
	}
}

func newConflictKeyMap(diffEnabled bool) conflictKeyMap {
	diffBinding := bubbleskey.NewBinding(
		bubbleskey.WithKeys("d"),
		bubbleskey.WithHelp("d", "diff"),
	)
	diffBinding.SetEnabled(diffEnabled)
	globalCancel := bubbleskey.NewBinding(
		bubbleskey.WithKeys("ctrl+c"),
		bubbleskey.WithHelp("ctrl+c", "cancel"),
	)
	return conflictKeyMap{
		globalCancel: globalCancel,
		list: listKeyMap{
			up:        binding([]string{"up", "k"}, "up/k", "up"),
			down:      binding([]string{"down", "j"}, "down/j", "down"),
			first:     binding([]string{"home", "g"}, "home/g", "first"),
			last:      binding([]string{"end", "G"}, "end/G", "last"),
			skip:      binding([]string{"s"}, "s", "skip"),
			replace:   binding([]string{"r"}, "r", "replace"),
			adopt:     binding([]string{"a"}, "a", "adopt"),
			diff:      diffBinding,
			apply:     binding([]string{"enter"}, "enter", "apply"),
			cancel:    binding([]string{"q", "esc"}, "q/esc", "cancel"),
			cancelAll: globalCancel,
		},
		diff: diffKeyMap{
			up:        binding([]string{"up", "k"}, "up/k", "line up"),
			down:      binding([]string{"down", "j"}, "down/j", "line down"),
			pageUp:    binding([]string{"pgup", "b"}, "pgup/b", "page up"),
			pageDown:  binding([]string{"pgdown", "f", " "}, "pgdn/f", "page down"),
			first:     binding([]string{"home", "g"}, "home/g", "top"),
			last:      binding([]string{"end", "G"}, "end/G", "bottom"),
			close:     binding([]string{"d"}, "d", "close"),
			cancel:    binding([]string{"q", "esc"}, "q/esc", "cancel"),
			cancelAll: globalCancel,
		},
	}
}

func binding(keys []string, helpKey, description string) bubbleskey.Binding {
	return bubbleskey.NewBinding(bubbleskey.WithKeys(keys...), bubbleskey.WithHelp(helpKey, description))
}

func disabledViewportKeyMap() viewport.KeyMap {
	disabled := bubbleskey.NewBinding(bubbleskey.WithDisabled())
	return viewport.KeyMap{
		PageDown:     disabled,
		PageUp:       disabled,
		HalfPageUp:   disabled,
		HalfPageDown: disabled,
		Down:         disabled,
		Up:           disabled,
		Left:         disabled,
		Right:        disabled,
	}
}

func diffViewportKeyMap(keys diffKeyMap) viewport.KeyMap {
	disabled := bubbleskey.NewBinding(bubbleskey.WithDisabled())
	return viewport.KeyMap{
		PageDown:     keys.pageDown,
		PageUp:       keys.pageUp,
		HalfPageUp:   disabled,
		HalfPageDown: disabled,
		Down:         keys.down,
		Up:           keys.up,
		Left:         disabled,
		Right:        disabled,
	}
}
