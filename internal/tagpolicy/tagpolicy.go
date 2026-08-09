// Package tagpolicy owns the allowed Tag registry classifications. Tags are
// declarative selection only and never trigger built-in Go behavior.
package tagpolicy

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
