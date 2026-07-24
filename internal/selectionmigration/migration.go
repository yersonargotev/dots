// Package selectionmigration analyzes pre-v3 Installation Metadata and proposes
// an explicit Installed Selection without mutating metadata or the filesystem.
package selectionmigration

import (
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/yersonargotev/dots/internal/installed"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
)

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

const (
	ReasonConflictingRecordedProfiles AmbiguityReason = "conflicting_recorded_profiles"
	ReasonConflictingRecordedTags     AmbiguityReason = "conflicting_recorded_tags"
	ReasonMissingRecordedProfiles     AmbiguityReason = "missing_recorded_profiles"
	ReasonMissingRecordedTags         AmbiguityReason = "missing_recorded_tags"
	ReasonNoHistoricalEvidence        AmbiguityReason = "no_historical_evidence"
	ReasonNoCompleteProfileCoverage   AmbiguityReason = "no_complete_profile_coverage"
	ReasonMultipleCompleteProfiles    AmbiguityReason = "multiple_complete_profiles"
	ReasonUnknownRecordedProfile      AmbiguityReason = "unknown_recorded_profile"
	ReasonUnmatchedHistoricalRecord   AmbiguityReason = "unmatched_historical_record"
	ReasonPartialProfileCoverage      AmbiguityReason = "partial_profile_coverage"
	ReasonUnusableSelection           AmbiguityReason = "unusable_selection"
	ReasonMissingTargetSourceEvidence AmbiguityReason = "missing_target_source_evidence"
	ReasonTargetSourceMismatch        AmbiguityReason = "target_source_mismatch"
)

type Confidence string

type AmbiguityReason string

type Candidate struct {
	Profiles           []string          `json:"profiles"`
	ExtraTags          []string          `json:"extra_tags"`
	EffectiveTags      []string          `json:"effective_tags"`
	Confidence         Confidence        `json:"confidence"`
	AmbiguityReasons   []AmbiguityReason `json:"ambiguity_reasons"`
	RecommendedCommand string            `json:"recommended_command,omitempty"`
}

type Analysis struct {
	Required  bool       `json:"required"`
	Candidate *Candidate `json:"candidate,omitempty"`
}

type Options struct {
	OS         string
	Home       string
	SourceRoot string
	StatePath  string
}

func (c Candidate) Unambiguous() bool {
	return len(c.AmbiguityReasons) == 0 && (len(c.Profiles) > 0 || len(c.ExtraTags) > 0)
}

func (c Candidate) AmbiguityReasonStrings() []string {
	out := make([]string, len(c.AmbiguityReasons))
	for i, reason := range c.AmbiguityReasons {
		out[i] = string(reason)
	}
	return out
}

func (c Candidate) Effective(m manifest.Manifest) (manifest.Selection, error) {
	return manifest.ResolveReadOnlySelection(m, c.Profiles, c.ExtraTags)
}

func Analyze(m manifest.Manifest, meta state.Metadata, opts Options) (Analysis, error) {
	if meta.InstalledSelection != nil || (meta.Version != 1 && meta.Version != 2) {
		return Analysis{Required: false}, nil
	}

	report, err := installed.Build(m, meta, installed.Options{
		OS: opts.OS, Home: opts.Home, StatePath: opts.StatePath,
	})
	if err != nil {
		return Analysis{}, err
	}

	reasons := reasonSet{}
	// Only current-manifest matches may contribute represented Tags. The raw
	// historical union in installed.Report intentionally remains diagnostic,
	// not authoritative migration intent.
	represented := representedMatchedTags(report)
	tagSets := recordedTagSets(meta)
	candidateTags := represented
	missingRecordedTags := len(tagSets) > 0 && hasMissingRecordedTags(meta)
	if len(tagSets) == 1 && !missingRecordedTags {
		candidateTags = newStringSet(tagSets[0])
	}
	addOverrideEvidence(m, meta, opts.OS, opts.Home, candidateTags)
	profileSets := recordedProfileSets(meta)
	profiles := []string(nil)
	confidence := ConfidenceLow

	if len(profileSets) == 1 {
		profiles = append([]string(nil), profileSets[0]...)
		confidence = ConfidenceHigh
		if hasMissingRecordedProfiles(meta) {
			reasons.add(ReasonMissingRecordedProfiles)
		}
	} else {
		if len(profileSets) > 1 {
			reasons.add(ReasonConflictingRecordedProfiles)
		}
		complete := completeProfiles(report)
		switch len(complete) {
		case 1:
			profiles = []string{complete[0]}
			confidence = ConfidenceMedium
		case 0:
			if len(meta.Entries) == 0 && len(meta.Provisioners) == 0 {
				reasons.add(ReasonNoHistoricalEvidence)
			} else {
				reasons.add(ReasonNoCompleteProfileCoverage)
			}
		default:
			reasons.add(ReasonMultipleCompleteProfiles)
		}
	}
	if len(tagSets) > 1 {
		reasons.add(ReasonConflictingRecordedTags)
	}
	if missingRecordedTags {
		reasons.add(ReasonMissingRecordedTags)
	}

	for _, p := range profiles {
		if _, ok := m.Profiles[p]; !ok {
			reasons.add(ReasonUnknownRecordedProfile)
		}
	}
	for _, entry := range report.ManagedEntries {
		if !entry.ManifestMatched {
			reasons.add(ReasonUnmatchedHistoricalRecord)
		}
	}
	for _, prov := range report.Provisioners {
		if !prov.ManifestMatched {
			reasons.add(ReasonUnmatchedHistoricalRecord)
		}
	}
	for _, coverage := range report.Profiles {
		if coverage.State != installed.CoveragePartial {
			continue
		}
		if len(profiles) == 0 || containsString(profiles, coverage.Name) {
			reasons.add(ReasonPartialProfileCoverage)
		}
	}

	extra := candidateTags.values()
	covered := map[string]bool{}
	for _, p := range profiles {
		if profile, ok := m.Profiles[p]; ok {
			for _, tag := range profile.Tags {
				covered[tag] = true
			}
		}
	}
	extra = filter(extra, func(tag string) bool { return !covered[tag] })

	candidate := &Candidate{Profiles: profiles, ExtraTags: extra, Confidence: confidence}
	selection, resolveErr := candidate.Effective(m)
	if resolveErr != nil {
		reasons.add(ReasonUnusableSelection)
	} else {
		candidate.EffectiveTags = append([]string{}, selection.Tags...)
		statusReport, statusErr := status.Build(m, meta, status.Options{
			Selection: &selection, OS: opts.OS, Home: opts.Home, SourceRoot: opts.SourceRoot,
		})
		if statusErr != nil {
			if errors.Is(statusErr, os.ErrNotExist) {
				reasons.add(ReasonMissingTargetSourceEvidence)
			} else {
				return Analysis{}, statusErr
			}
		} else {
			evaluated := 0
			for _, entry := range statusReport.Entries {
				if entry.State == status.StateSkipped {
					continue
				}
				evaluated++
				if entry.State == status.StateMissing {
					reasons.add(ReasonMissingTargetSourceEvidence)
				} else if entry.State != status.StateOK {
					reasons.add(ReasonTargetSourceMismatch)
				}
			}
			if evaluated == 0 && (len(meta.Entries) > 0 || len(meta.Provisioners) > 0) {
				reasons.add(ReasonMissingTargetSourceEvidence)
			}
		}
	}

	candidate.AmbiguityReasons = reasons.values()
	if len(candidate.AmbiguityReasons) > 0 {
		candidate.Confidence = ConfidenceLow
	}
	if candidate.Unambiguous() {
		candidate.RecommendedCommand = command(candidate)
	}
	return Analysis{Required: true, Candidate: candidate}, nil
}

func recordedProfileSets(meta state.Metadata) [][]string {
	values := make([][]string, 0, len(meta.Entries)+len(meta.Provisioners))
	for _, rec := range meta.Entries {
		values = append(values, rec.Profiles)
	}
	for _, rec := range meta.Provisioners {
		profiles := append([]string(nil), rec.Profiles...)
		profiles = append(profiles, strings.Split(rec.Profile, ",")...)
		values = append(values, profiles)
	}
	return distinctSets(values)
}

func recordedTagSets(meta state.Metadata) [][]string {
	values := make([][]string, 0, len(meta.Entries)+len(meta.Provisioners))
	for _, rec := range meta.Entries {
		values = append(values, rec.Tags)
	}
	for _, rec := range meta.Provisioners {
		values = append(values, rec.Tags)
	}
	return distinctSets(values)
}

func distinctSets(values [][]string) [][]string {
	unique := map[string][]string{}
	for _, values := range values {
		set := newStringSet(values).values()
		if len(set) == 0 {
			continue
		}
		unique[strings.Join(set, "\x00")] = set
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([][]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, unique[key])
	}
	return out
}

func hasMissingRecordedProfiles(meta state.Metadata) bool {
	for _, rec := range meta.Entries {
		if len(newStringSet(rec.Profiles)) == 0 {
			return true
		}
	}
	for _, rec := range meta.Provisioners {
		values := append([]string(nil), rec.Profiles...)
		values = append(values, strings.Split(rec.Profile, ",")...)
		if len(newStringSet(values)) == 0 {
			return true
		}
	}
	return false
}

func hasMissingRecordedTags(meta state.Metadata) bool {
	for _, rec := range meta.Entries {
		if len(newStringSet(rec.Tags)) == 0 {
			return true
		}
	}
	for _, rec := range meta.Provisioners {
		if len(newStringSet(rec.Tags)) == 0 {
			return true
		}
	}
	return false
}

func completeProfiles(report installed.Report) []string {
	var out []string
	for _, coverage := range report.Profiles {
		if coverage.State == installed.CoverageComplete {
			out = append(out, coverage.Name)
		}
	}
	sort.Strings(out)
	return out
}

func representedMatchedTags(report installed.Report) stringSet {
	out := stringSet{}
	for _, entry := range report.ManagedEntries {
		if entry.ManifestMatched {
			for _, tag := range entry.Tags {
				out[tag] = true
			}
		}
	}
	for _, prov := range report.Provisioners {
		if prov.ManifestMatched {
			for _, tag := range prov.Tags {
				out[tag] = true
			}
		}
	}
	return out
}

func addOverrideEvidence(m manifest.Manifest, meta state.Metadata, osName, home string, represented map[string]bool) {
	for _, rec := range meta.Entries {
		for _, entry := range m.Entries {
			if !manifest.MatchesOS(entry.OS, osName) || rec.Strategy != entry.Strategy {
				continue
			}
			target, err := plan.ResolveTarget(entry.Target, home)
			if err != nil || target != rec.Target {
				continue
			}
			for tag, source := range entry.SourceOverrides {
				if rec.Source == source {
					represented[tag] = true
				}
			}
		}
	}
}

type reasonSet map[AmbiguityReason]bool

func (s reasonSet) add(v AmbiguityReason) { s[v] = true }
func (s reasonSet) values() []AmbiguityReason {
	out := make([]AmbiguityReason, 0, len(s))
	for v := range s {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

type stringSet map[string]bool

func newStringSet(values []string) stringSet {
	out := stringSet{}
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out[v] = true
		}
	}
	return out
}
func (s stringSet) values() []string {
	out := make([]string, 0, len(s))
	for v := range s {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func filter(in []string, keep func(string) bool) []string {
	out := []string{}
	for _, v := range in {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}
func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func command(c *Candidate) string {
	parts := []string{"dots", "install"}
	for _, p := range c.Profiles {
		parts = append(parts, "--profile", p)
	}
	for _, t := range c.ExtraTags {
		parts = append(parts, "--tag", t)
	}
	return strings.Join(parts, " ")
}
