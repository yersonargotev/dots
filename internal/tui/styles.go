package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/yersonargotev/dots/internal/install"
	uitheme "github.com/yersonargotev/dots/internal/tui/theme"
)

// Styles maps Conflict Resolution semantics onto the shared TUI theme.
type Styles struct {
	Theme uitheme.Theme

	decision map[install.ConflictDecision]lipgloss.Style
}

// DefaultStyles builds styles from the shared Catppuccin Mocha theme.
func DefaultStyles() Styles {
	return stylesFromTheme(uitheme.Default())
}

func stylesFromTheme(t uitheme.Theme) Styles {
	return Styles{
		Theme: t,
		decision: map[install.ConflictDecision]lipgloss.Style{
			install.DecisionSkip:    t.Secondary,
			install.DecisionReplace: t.Warning,
			install.DecisionAdopt:   t.Caution,
		},
	}
}

// Decision returns the semantic style for a decision, falling back to skip.
func (s Styles) Decision(d install.ConflictDecision) lipgloss.Style {
	if style, ok := s.decision[d]; ok {
		return style
	}
	return s.decision[install.DecisionSkip]
}
