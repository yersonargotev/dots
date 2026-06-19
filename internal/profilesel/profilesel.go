// Package profilesel computes profile-scope omission math shared by the file
// entry and provisioner surfaces: given how each profile selects items, which
// items the active profile silently skips and which single profile best recovers
// them. It is pure (no I/O) and item-agnostic — callers inject a Selector that
// knows how to select their own slice (entries, provisioners, ...).
package profilesel

import (
	"sort"

	"github.com/yersonargotev/dots/internal/manifest"
)

// Hint describes items the active profile omits that some other profile would
// select on this OS, so a caller can nudge the user toward the fuller profile
// instead of silently dropping them.
type Hint struct {
	// Profile is the active profile being installed.
	Profile string
	// Count is how many of the skipped items SuggestedProfile would recover. It
	// is intentionally the suggested profile's coverage, not the union of
	// omissions across every profile, so the "run --profile S to include them"
	// nudge is always exact and never promises more than S delivers. In the
	// nested-profile model dots uses (e.g. desktop ⊇ default) the most complete
	// profile recovers everything, so Count equals the total the active profile
	// omits.
	Count int
	// SuggestedProfile is the other profile that covers the most skipped items —
	// the single most complete profile worth recommending.
	SuggestedProfile string
}

// Selector returns the set of item positions (index space) a profile selects on
// the given OS. Working in index space lets Skipped compare item identity across
// profiles — which item, not just how many — without depending on item struct
// equality. It returns an error for an unknown profile so Skipped surfaces the
// same validation the caller's own selection uses.
type Selector func(profileName, os string) (map[int]bool, error)

// Skipped reports whether the active profile omits items that another profile
// would select on this OS, and which single profile best recovers them. The
// second return is false (with a zero hint) when nothing is skipped — either the
// active profile already selects everything, or the OS filter excludes the extras
// for every profile so switching profiles would not recover them. The reported
// Count is the suggested profile's coverage of the skipped set, so the caller's
// remediation message stays accurate even when no single profile is a superset of
// every omission.
func Skipped(profiles map[string]manifest.Profile, active, os string, sel Selector) (Hint, bool, error) {
	activeSet, err := sel(active, os)
	if err != nil {
		return Hint{}, false, err
	}

	others := make([]string, 0, len(profiles))
	for name := range profiles {
		if name != active {
			others = append(others, name)
		}
	}
	// Sort so the suggested profile is deterministic on ties (first by name).
	sort.Strings(others)

	selections := make(map[string]map[int]bool, len(others))
	skipped := make(map[int]bool)
	for _, name := range others {
		set, err := sel(name, os)
		if err != nil {
			return Hint{}, false, err
		}
		selections[name] = set
		for i := range set {
			if !activeSet[i] {
				skipped[i] = true
			}
		}
	}

	if len(skipped) == 0 {
		return Hint{}, false, nil
	}

	var (
		suggested string
		best      int
	)
	for _, name := range others {
		covered := 0
		for i := range selections[name] {
			if skipped[i] {
				covered++
			}
		}
		if covered > best {
			best = covered
			suggested = name
		}
	}

	// best is the suggested profile's coverage of the skipped set, and is >= 1
	// whenever skipped is non-empty: every skipped index came from some other
	// profile's selection, so that profile covers it. Reporting best (not
	// len(skipped)) keeps "run --profile S to include them" exact.
	return Hint{Profile: active, Count: best, SuggestedProfile: suggested}, true, nil
}
