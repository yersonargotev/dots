// Package ownershipevidence owns the mode-specific semantics for capturing,
// projecting, validating, and describing exact contribution evidence.
package ownershipevidence

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/textblock"
)

const (
	DiscriminatorSourceIdentity   = "source-identity"
	DiscriminatorSourceHash       = "source-hash"
	DiscriminatorOwnedJSON        = "owned-json"
	DiscriminatorOwnedJSONC       = "owned-jsonc"
	DiscriminatorOwnedTOML        = "owned-toml"
	DiscriminatorOwnedMarkedBlock = "owned-marked-block"
	DiscriminatorSeededBaseline   = "seeded-baseline"
	DiscriminatorMissing          = "missing"
)

// ErrDrift identifies a live target that no longer converges with captured
// ownership evidence. Callers should use errors.Is rather than comparing the
// returned error directly.
var ErrDrift = errors.New("ownership evidence drift")

// Mode is the concrete ownership behavior selected by an install strategy and
// ownership declaration. Its zero value represents copy with Whole-Target
// Ownership, matching the manifest default.
type Mode struct {
	strategy  string
	ownership string
}

// For selects ownership behavior. An empty strategy defaults to copy so the
// zero Mode remains useful, and empty ownership normalizes to whole.
func For(strategy, ownership string) Mode {
	if strategy == "" {
		strategy = "copy"
	}
	if ownership == "" {
		ownership = "whole"
	}
	return Mode{strategy: strategy, ownership: ownership}
}

// Ownership returns the normalized ownership name recorded in Installation
// Metadata.
func (m Mode) Ownership() string {
	return m.normalized().ownership
}

// Capture records exact evidence for one Source of Truth contribution.
// source is the manifest-relative identity and sourcePath is its already
// validated, resolved filesystem path. A non-nil seededBaseline overrides the
// source bytes for seeded evidence; an explicitly empty baseline is preserved.
func (m Mode) Capture(source, sourcePath string, selectorTags []string, seededBaseline []byte) (state.Contribution, error) {
	m = m.normalized()
	if err := m.validate(); err != nil {
		return state.Contribution{}, err
	}

	recorded := state.Contribution{
		Source:           source,
		SelectorTags:     append([]string(nil), selectorTags...),
		Ownership:        m.ownership,
		EvidenceRecorded: true,
	}
	if m.strategy == "symlink" {
		return recorded, nil
	}

	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return state.Contribution{}, fmt.Errorf("read %s contribution %s: %w", m.ownership, sourcePath, err)
	}
	recorded.Hash = state.HashBytes(raw)

	switch m.ownership {
	case "whole":
	case "json-subset":
		recorded.OwnedContent = cloneBytes(raw)
	case "jsonc-subset":
		recorded.OwnedContent, err = configsubset.CanonicalJSONC(raw)
		if err != nil {
			return state.Contribution{}, fmt.Errorf("canonicalize owned jsonc contribution %s: %w", sourcePath, err)
		}
	case "toml-subset", "marked-block":
		recorded.OwnedBytes = cloneBytes(raw)
	case "seeded":
		if seededBaseline != nil {
			recorded.SeededBaseline = cloneBytes(seededBaseline)
		} else {
			recorded.SeededBaseline = cloneBytes(raw)
		}
	}
	return recorded, nil
}

// Project derives target-wide compatibility evidence from exact contribution
// evidence. One contribution projects directly. Multiple contributions are
// supported only for copy/json-subset, where they are composed in order.
func (m Mode) Project(target string, contributions []state.Contribution) (state.Contribution, error) {
	m = m.normalized()
	if err := m.validate(); err != nil {
		return state.Contribution{}, err
	}
	if len(contributions) == 0 {
		return state.Contribution{}, fmt.Errorf("project target-wide evidence for %s: no contributions", target)
	}
	for i, contribution := range contributions {
		if !contribution.EvidenceRecorded {
			return state.Contribution{}, fmt.Errorf("project target-wide evidence for %s: contribution %d has incomplete evidence", target, i)
		}
		if contribution.Ownership != m.ownership {
			return state.Contribution{}, fmt.Errorf("project target-wide evidence for %s: contribution %d ownership %q does not match %q", target, i, contribution.Ownership, m.ownership)
		}
	}
	if len(contributions) == 1 {
		return cloneContribution(contributions[0]), nil
	}
	if m.strategy != "copy" || m.ownership != "json-subset" {
		return state.Contribution{}, fmt.Errorf("project target-wide evidence for %s: multiple contributions require copy/json-subset ownership", target)
	}

	composed := append([]byte(nil), contributions[0].OwnedContent...)
	for _, contribution := range contributions[1:] {
		var err error
		composed, err = configsubset.MergeJSON(composed, contribution.OwnedContent)
		if err != nil {
			return state.Contribution{}, fmt.Errorf("compose target-wide evidence for %s: %w", target, err)
		}
	}
	return state.Contribution{
		Ownership:        m.ownership,
		EvidenceRecorded: true,
		Hash:             state.HashBytes(composed),
		OwnedContent:     composed,
	}, nil
}

// Validate confirms that target still converges with evidence immediately
// before evidence is committed. resolvedSources is used to verify symlink
// identity. seededFinal accepts zero or one value: zero validates the recorded
// baseline, while one validates migration final content, including empty
// content. More than one final value is rejected.
func (m Mode) Validate(target string, resolvedSources []string, evidence state.Contribution, seededFinal ...[]byte) error {
	m = m.normalized()
	if err := m.validate(); err != nil {
		return err
	}
	if len(seededFinal) > 1 {
		return fmt.Errorf("validate ownership evidence for %s: seeded final content accepts at most one value", target)
	}
	if len(seededFinal) == 1 && m.ownership != "seeded" {
		return fmt.Errorf("validate ownership evidence for %s: final content requires seeded ownership", target)
	}
	if !evidence.EvidenceRecorded {
		return fmt.Errorf("validate ownership evidence for %s: evidence is incomplete", target)
	}
	if evidence.Ownership != m.ownership {
		return fmt.Errorf("validate ownership evidence for %s: ownership %q does not match %q", target, evidence.Ownership, m.ownership)
	}

	if m.strategy == "symlink" {
		return m.validateSymlink(target, resolvedSources)
	}

	live, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read converged target %s: %w", target, err)
	}
	converged := false
	switch m.ownership {
	case "whole":
		converged = state.HashBytes(live) == evidence.Hash
	case "json-subset":
		relation, analyzeErr := configsubset.AnalyzeJSON(live, evidence.OwnedContent)
		if analyzeErr != nil {
			return fmt.Errorf("validate converged json target %s: %w", target, analyzeErr)
		}
		converged = relation.Contains
	case "jsonc-subset":
		relation, analyzeErr := configsubset.AnalyzeJSONC(live, evidence.OwnedContent)
		if analyzeErr != nil {
			return fmt.Errorf("validate converged jsonc target %s: %w", target, analyzeErr)
		}
		converged = relation.Contains
	case "toml-subset":
		relation, analyzeErr := configsubset.AnalyzeTOML(live, evidence.OwnedBytes)
		if analyzeErr != nil {
			return fmt.Errorf("validate converged toml target %s: %w", target, analyzeErr)
		}
		converged = relation.Contains
	case "marked-block":
		reconciliation := textblock.ReconcileOwned(live, evidence.OwnedBytes, evidence.OwnedBytes, textblock.DotsManagedMarkers())
		converged = reconciliation.Compatible && !reconciliation.Changed
	case "seeded":
		want := evidence.SeededBaseline
		if len(seededFinal) == 1 {
			want = seededFinal[0]
		}
		converged = bytes.Equal(live, want)
	}
	if !converged {
		return fmt.Errorf("managed target %s no longer converged: %w", target, ErrDrift)
	}
	return nil
}

// Discriminator returns the stable public name for the exact evidence kind.
// Incomplete or unsupported evidence always reports missing.
func (m Mode) Discriminator(evidence state.Contribution) string {
	m = m.normalized()
	if err := m.validate(); err != nil {
		return DiscriminatorMissing
	}
	if !evidence.EvidenceRecorded || evidence.Ownership != m.ownership {
		return DiscriminatorMissing
	}
	switch m.ownership {
	case "json-subset":
		return DiscriminatorOwnedJSON
	case "jsonc-subset":
		return DiscriminatorOwnedJSONC
	case "toml-subset":
		return DiscriminatorOwnedTOML
	case "marked-block":
		return DiscriminatorOwnedMarkedBlock
	case "seeded":
		return DiscriminatorSeededBaseline
	case "whole":
		if m.strategy == "symlink" {
			return DiscriminatorSourceIdentity
		}
		if m.strategy == "copy" {
			return DiscriminatorSourceHash
		}
	}
	return DiscriminatorMissing
}

func (m Mode) normalized() Mode {
	return For(m.strategy, m.ownership)
}

func (m Mode) validate() error {
	switch m.strategy {
	case "copy":
		switch m.ownership {
		case "whole", "json-subset", "jsonc-subset", "toml-subset", "marked-block", "seeded":
			return nil
		}
	case "symlink":
		if m.ownership == "whole" {
			return nil
		}
	default:
		return fmt.Errorf("ownership evidence strategy %q is not supported", m.strategy)
	}
	return fmt.Errorf("ownership evidence mode %s/%s is not supported", m.strategy, m.ownership)
}

func (m Mode) validateSymlink(target string, resolvedSources []string) error {
	if len(resolvedSources) != 1 {
		return fmt.Errorf("managed target %s no longer converged: symlink has %d sources: %w", target, len(resolvedSources), ErrDrift)
	}
	info, err := os.Lstat(target)
	if err != nil {
		return fmt.Errorf("inspect converged symlink %s: %w", target, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("managed target %s no longer converged: expected symlink: %w", target, ErrDrift)
	}
	destination, err := os.Readlink(target)
	if err != nil {
		return fmt.Errorf("read converged symlink %s: %w", target, err)
	}
	if filepath.Clean(destination) != filepath.Clean(resolvedSources[0]) {
		return fmt.Errorf("managed target %s no longer converged: symlink destination changed: %w", target, ErrDrift)
	}
	return nil
}

func cloneContribution(contribution state.Contribution) state.Contribution {
	cloned := contribution
	cloned.SelectorTags = append([]string(nil), contribution.SelectorTags...)
	cloned.OwnedContent = append([]byte(nil), contribution.OwnedContent...)
	cloned.OwnedBytes = append([]byte(nil), contribution.OwnedBytes...)
	cloned.SeededBaseline = cloneBytes(contribution.SeededBaseline)
	return cloned
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte{}, value...)
}
