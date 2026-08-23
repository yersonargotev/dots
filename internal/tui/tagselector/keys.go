package tagselector

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Cancel        key.Binding
	Quit          key.Binding
	Up            key.Binding
	Down          key.Binding
	PageUp        key.Binding
	PageDown      key.Binding
	Home          key.Binding
	End           key.Binding
	Toggle        key.Binding
	Search        key.Binding
	Profiles      key.Binding
	Details       key.Binding
	Preview       key.Binding
	Retry         key.Binding
	Confirm       key.Binding
	Accept        key.Binding
	Back          key.Binding
	Acknowledge   key.Binding
	Decline       key.Binding
	Return        key.Binding
	ProfileToggle key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Cancel:        binding([]string{"ctrl+c"}, "ctrl+c", "cancel"),
		Quit:          binding([]string{"q"}, "q", "cancel"),
		Up:            binding([]string{"up", "k"}, "up/k", "up"),
		Down:          binding([]string{"down", "j"}, "down/j", "down"),
		PageUp:        binding([]string{"pgup"}, "pgup", "page up"),
		PageDown:      binding([]string{"pgdown"}, "pgdown", "page down"),
		Home:          binding([]string{"home"}, "home", "first"),
		End:           binding([]string{"end"}, "end", "last"),
		Toggle:        binding([]string{" "}, "space", "toggle"),
		Search:        binding([]string{"/"}, "/", "search"),
		Profiles:      binding([]string{"p"}, "p", "profiles"),
		Details:       binding([]string{"d", "right"}, "d/right", "details"),
		Preview:       binding([]string{"enter"}, "enter", "preview"),
		Retry:         binding([]string{"enter"}, "enter", "retry"),
		Accept:        binding([]string{"enter"}, "enter", "filter"),
		Back:          binding([]string{"esc"}, "esc", "clear/back"),
		Acknowledge:   binding([]string{"y", "enter"}, "y/enter", "ok"),
		Decline:       binding([]string{"n", "esc"}, "n/esc", "back"),
		Return:        binding([]string{"esc", "left", "d"}, "esc/←/d", "back"),
		Confirm:       binding([]string{"enter"}, "enter", "ok"),
		ProfileToggle: binding([]string{" ", "enter"}, "spc/↵", "apply"),
	}
}

func binding(keys []string, helpKey, description string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(helpKey, description))
}

type screenHelp []key.Binding

func (h screenHelp) ShortHelp() []key.Binding  { return h }
func (h screenHelp) FullHelp() [][]key.Binding { return [][]key.Binding{h} }

func (m Model) activeHelp() screenHelp {
	k := m.keys
	switch m.screen {
	case screenSearch:
		return screenHelp{k.Accept, k.Up, k.Down, k.Back, k.Cancel}
	case screenProfiles:
		return screenHelp{k.ProfileToggle, k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End, k.Back, k.Quit, k.Cancel}
	case screenDetail:
		return screenHelp{k.Return, k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End, k.Quit, k.Cancel}
	case screenLoading:
		return screenHelp{k.Back, k.Quit, k.Cancel}
	case screenPreview:
		return screenHelp{k.Confirm, k.Back, k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End, k.Quit, k.Cancel}
	case screenReductionConfirmation:
		return screenHelp{k.Acknowledge, k.Decline, k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End, k.Quit, k.Cancel}
	case screenClearConfirmation:
		return screenHelp{k.Confirm, k.Back, k.PageUp, k.PageDown, k.Quit, k.Cancel}
	default:
		if m.previewError != "" {
			return screenHelp{k.Retry, k.Toggle, k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End, k.Search, k.Profiles, k.Details, k.Quit, k.Cancel}
		}
		return screenHelp{k.Toggle, m.previewAction(), k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End, k.Search, k.Profiles, k.Details, k.Quit, k.Cancel}
	}
}

func (m Model) previewAction() key.Binding {
	if m.previewError != "" {
		return m.keys.Retry
	}
	return m.keys.Preview
}
