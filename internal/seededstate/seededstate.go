// Package seededstate reconciles opaque runtime state seeded by dots.
package seededstate

import "bytes"

// Classification describes how live seeded state relates to its baselines.
type Classification string

const (
	// AlignedCurrent means live state already matches the current baseline.
	AlignedCurrent Classification = "aligned-current"
	// AdvanceBaseline means live state remains at the previous baseline and can
	// safely advance to the current one.
	AdvanceBaseline Classification = "advance-baseline"
	// LocalEvolution means live state contains trusted local changes and must be
	// left untouched.
	LocalEvolution Classification = "local-evolution"
)

// Result is the outcome of reconciling seeded runtime state. Content is set
// only when Changed is true, and is a copy of the current baseline.
type Result struct {
	Classification Classification
	Changed        bool
	Content        []byte
}

// Reconcile compares opaque live state with the recorded previous and current
// Source of Truth baselines. It never mutates its inputs. A baseline advances
// only when the live state still exactly matches the previous baseline.
func Reconcile(live, previous, current []byte) Result {
	if bytes.Equal(live, current) {
		return Result{Classification: AlignedCurrent}
	}
	if bytes.Equal(live, previous) && !bytes.Equal(previous, current) {
		return Result{
			Classification: AdvanceBaseline,
			Changed:        true,
			Content:        bytes.Clone(current),
		}
	}
	return Result{Classification: LocalEvolution}
}
