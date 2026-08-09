// Package tagpolicy owns the closed set of tag-triggered behavior. Install
// Manifests may select these tags, but they cannot define shell commands or
// other executable behavior.
package tagpolicy

// Action is a reviewed, built-in behavior that may be selected by one or more
// behavior tags.
type Action string

const (
	ActionRetireGentleAIState     Action = "retire-gentle-ai-state"
	ActionRemoveCodexDelegation   Action = "remove-codex-delegation"
	ActionConvergeCodexDelegation Action = "converge-codex-delegation"
)

type behaviorPolicy struct {
	kind   string
	action Action
}

var behaviorByTag = map[string]behaviorPolicy{
	"agents":                         {kind: "surface", action: ActionRetireGentleAIState},
	"codex-delegation":               {kind: "surface", action: ActionConvergeCodexDelegation},
	"without-codex-delegation":       {kind: "cleanup", action: ActionRemoveCodexDelegation},
	"codex-spark-delegation":         {kind: "compatibility", action: ActionConvergeCodexDelegation},
	"without-codex-spark-delegation": {kind: "compatibility", action: ActionRemoveCodexDelegation},
}

// IsBehaviorTag reports whether tag has an allowlisted effect beyond ordinary
// manifest surface selection.
func IsBehaviorTag(tag string) bool {
	_, ok := behaviorByTag[tag]
	return ok
}

// ExpectedKind returns the only registry kind permitted for a behavior tag.
func ExpectedKind(tag string) (string, bool) {
	policy, ok := behaviorByTag[tag]
	return policy.kind, ok
}

// IsAllowedKind reports whether kind is a supported registry classification.
func IsAllowedKind(kind string) bool {
	switch kind {
	case "surface", "cleanup", "compatibility":
		return true
	default:
		return false
	}
}

// IsAllowedStatus reports whether status is a supported lifecycle state.
func IsAllowedStatus(status string) bool {
	switch status {
	case "current", "legacy":
		return true
	default:
		return false
	}
}

// Actions returns the deterministic set of built-in actions selected by tags.
// Removal wins over convergence when both delegation modes are selected.
func Actions(tags []string) []Action {
	present := make(map[Action]bool)
	for _, tag := range tags {
		if policy, ok := behaviorByTag[tag]; ok {
			present[policy.action] = true
		}
	}
	actions := make([]Action, 0, 2)
	if present[ActionRetireGentleAIState] {
		actions = append(actions, ActionRetireGentleAIState)
	}
	if present[ActionRemoveCodexDelegation] {
		actions = append(actions, ActionRemoveCodexDelegation)
	} else if present[ActionConvergeCodexDelegation] {
		actions = append(actions, ActionConvergeCodexDelegation)
	}
	return actions
}
