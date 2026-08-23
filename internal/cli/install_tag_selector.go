package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/catalog"
	"github.com/yersonargotev/dots/internal/codexconfig"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/deps/pkgmgr"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/selectionreconciliation"
	"github.com/yersonargotev/dots/internal/selectionretirement"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
	tagselectortui "github.com/yersonargotev/dots/internal/tui/tagselector"
	"github.com/yersonargotev/dots/internal/version"
)

type installTagSelectorRunnerFunc func(io.Reader, io.Writer, tagselectortui.BrowseData, []string, tagselectortui.PreviewFunc) (tagselectortui.Result, error)

var (
	installTagSelectorRunner   installTagSelectorRunnerFunc = tagselectortui.Run
	installTagSelectorTerminal                              = interactiveTerminal
)

type terminalFile interface {
	Fd() uintptr
}

func interactiveTerminal(in io.Reader, out io.Writer) bool {
	input, inputOK := in.(terminalFile)
	output, outputOK := out.(terminalFile)
	return inputOK && outputOK && term.IsTerminal(input.Fd()) && term.IsTerminal(output.Fd())
}

func installTagSelectorRequested(cmd *cobra.Command, yes, noTUI bool) bool {
	return !yes && !noTUI && !wantsJSON(cmd) && installTagSelectorTerminal(cmd.InOrStdin(), cmd.OutOrStdout())
}

type installTagSelectorOptions struct {
	ManifestPath       string
	RequestedStateRoot string
	DryRun             bool
	SkipDependencies   bool
	NoTUI              bool
	BackupAndReplace   bool
}

type installTagSelectorCandidate struct {
	Manifest            manifest.Manifest
	Metadata            state.Metadata
	Paths               resolvedPaths
	Effective           selection.Effective
	Dependencies        *deps.PreparedInstall
	DependencyOptions   deps.Options
	DependencyLookup    deps.Lookup
	BrewDetection       pkgmgr.HomebrewDetection
	PackageManagerSetup *pkgmgr.Report
	Plan                plan.Plan
	CapturedSources     map[install.SourceCaptureKey]install.CapturedSource
	Provisioners        provision.Plan
	Retirement          selectionretirement.Plan
	RetirementBlock     string
	RetirementOptions   selectionretirement.Options
	ManifestPath        string
	RequestedStateRoot  string
	Preview             tagselectortui.Preview
}

func (c installTagSelectorCandidate) releaseCapturedSources() error {
	return install.ReleaseCapturedSources(c.CapturedSources)
}

type installTagSelectorCandidateStore struct {
	mu          sync.Mutex
	closed      bool
	latestID    uint64
	latestToken string
	candidate   *installTagSelectorCandidate
	release     func(installTagSelectorCandidate) error
}

func newInstallTagSelectorCandidateStore() *installTagSelectorCandidateStore {
	return &installTagSelectorCandidateStore{release: func(candidate installTagSelectorCandidate) error {
		return candidate.releaseCapturedSources()
	}}
}

func (s *installTagSelectorCandidateStore) activate(requestID uint64) (string, bool, error) {
	s.mu.Lock()
	if s.closed || requestID == 0 || requestID <= s.latestID {
		s.mu.Unlock()
		return "", false, nil
	}
	token := fmt.Sprintf("selector-preview-%d", requestID)
	previous := s.candidate
	s.candidate = nil
	s.latestID = requestID
	s.latestToken = token
	s.mu.Unlock()
	if previous != nil {
		if err := s.releaseCandidate(*previous); err != nil {
			return token, true, fmt.Errorf("release superseded Tag selector candidate: %w", err)
		}
	}
	return token, true, nil
}

func (s *installTagSelectorCandidateStore) put(requestID uint64, token string, candidate installTagSelectorCandidate) (bool, error) {
	s.mu.Lock()
	if s.closed || requestID != s.latestID || token == "" || token != s.latestToken {
		s.mu.Unlock()
		if err := s.releaseCandidate(candidate); err != nil {
			return false, fmt.Errorf("release rejected Tag selector candidate: %w", err)
		}
		return false, nil
	}
	previous := s.candidate
	s.candidate = &candidate
	s.mu.Unlock()
	if previous != nil {
		if err := s.releaseCandidate(*previous); err != nil {
			return true, fmt.Errorf("release replaced Tag selector candidate: %w", err)
		}
	}
	return true, nil
}

func (s *installTagSelectorCandidateStore) take(token string) (installTagSelectorCandidate, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || token == "" || token != s.latestToken || s.candidate == nil {
		return installTagSelectorCandidate{}, false
	}
	candidate := *s.candidate
	s.candidate = nil
	return candidate, true
}

type installTagSelectorPreviewProvider struct {
	store      *installTagSelectorCandidateStore
	buildMu    sync.Mutex
	releaseMu  sync.Mutex
	releaseErr error
	closed     bool
	lateError  func(error)
	build      func([]string) (installTagSelectorCandidate, error)
}

func (p *installTagSelectorPreviewProvider) preview(requestID uint64, tags []string) (tagselectortui.Preview, error) {
	p.buildMu.Lock()
	defer p.buildMu.Unlock()

	token, ok, releaseErr := p.store.activate(requestID)
	reportedLate := p.recordReleaseError(releaseErr)
	if releaseErr != nil {
		if reportedLate {
			return tagselectortui.Preview{}, errors.New("Tag selector preview provider closed during cleanup")
		}
		return tagselectortui.Preview{}, releaseErr
	}
	if !ok {
		return tagselectortui.Preview{}, fmt.Errorf("Tag selector preview request is stale or the candidate store is closed")
	}
	candidate, err := p.build(tags)
	if err != nil {
		return tagselectortui.Preview{}, err
	}
	candidate.Preview.CandidateToken = token
	stored, releaseErr := p.store.put(requestID, token, candidate)
	reportedLate = p.recordReleaseError(releaseErr)
	if releaseErr != nil {
		if reportedLate {
			return tagselectortui.Preview{}, errors.New("Tag selector preview provider closed during cleanup")
		}
		return tagselectortui.Preview{}, releaseErr
	}
	if !stored {
		return tagselectortui.Preview{}, fmt.Errorf("Tag selector preview request was superseded")
	}
	return candidate.Preview, nil
}

func (p *installTagSelectorPreviewProvider) recordReleaseError(err error) bool {
	if err == nil {
		return false
	}
	p.releaseMu.Lock()
	if p.closed {
		report := p.lateError
		p.releaseMu.Unlock()
		if report != nil {
			report(err)
		} else {
			slog.Error("late Tag selector cleanup failed", "error", err)
		}
		return true
	}
	p.releaseErr = errors.Join(p.releaseErr, err)
	p.releaseMu.Unlock()
	return false
}

func (p *installTagSelectorPreviewProvider) close() error {
	closeErr := p.store.close()
	p.releaseMu.Lock()
	defer p.releaseMu.Unlock()
	result := errors.Join(p.releaseErr, closeErr)
	p.releaseErr = nil
	p.closed = true
	return result
}

func (s *installTagSelectorCandidateStore) releaseCandidate(candidate installTagSelectorCandidate) error {
	if s.release == nil {
		return candidate.releaseCapturedSources()
	}
	return s.release(candidate)
}

func (s *installTagSelectorCandidateStore) close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	candidate := s.candidate
	s.candidate = nil
	s.mu.Unlock()
	if candidate != nil {
		if err := s.releaseCandidate(*candidate); err != nil {
			return fmt.Errorf("release stored Tag selector candidate: %w", err)
		}
	}
	return nil
}

func runInstallTagSelector(cmd *cobra.Command, m manifest.Manifest, meta state.Metadata, paths resolvedPaths, sourceReadRoot string, legacyMigrations map[string]plan.LegacyMigration, opts installTagSelectorOptions) (resultErr error) {
	initial, err := resolveInstallTagSelectorInitial(m, meta.InstalledSelection)
	if err != nil {
		return err
	}
	browseData, err := buildInstallTagSelectorBrowseData(m, meta, tagSelectorBrowseOptions{
		OS:             installHostOS,
		Arch:           installHostArch,
		SourceReadRoot: sourceReadRoot,
		Home:           paths.Home,
		StateRoot:      paths.StateRoot,
		XDGStateHome:   paths.XDGStateHome,
		Lookup:         lookupCommand,
		FontLookup:     fontInstalled(installHostOS, paths.Home),
		AppLookup:      appInstalled(installHostOS, paths.Home),
		CommandRunner:  commandOutput,
	})
	if err != nil {
		return err
	}
	candidates := newInstallTagSelectorCandidateStore()
	provider := &installTagSelectorPreviewProvider{
		store: candidates,
		build: func(tags []string) (installTagSelectorCandidate, error) {
			return buildInstallTagSelectorCandidate(m, meta, paths, sourceReadRoot, legacyMigrations, installHostOS, tags, opts)
		},
	}
	defer func() {
		if err := provider.close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close Tag selector preview provider: %w", err))
		}
	}()
	result, err := installTagSelectorRunner(cmd.InOrStdin(), cmd.OutOrStdout(), browseData, initial, provider.preview)
	if errors.Is(err, tagselectortui.ErrCanceled) {
		fmt.Fprintln(cmd.OutOrStdout(), "Tag selection canceled; nothing was applied.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("run Tag selector: %w", err)
	}
	if result.Preview.SemanticDigest == "" || result.Preview.CandidateToken == "" {
		return fmt.Errorf("run Tag selector: completed without an accepted preview")
	}
	candidate, ok := candidates.take(result.Preview.CandidateToken)
	if !ok {
		return fmt.Errorf("run Tag selector: accepted preview does not match its immutable candidate")
	}
	defer func() {
		if err := candidate.releaseCapturedSources(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("release accepted Tag selector candidate: %w", err))
		}
	}()
	if err := provider.close(); err != nil {
		return fmt.Errorf("close Tag selector preview provider: %w", err)
	}
	if !reflect.DeepEqual(result.Preview, candidate.Preview) || !slices.Equal(result.Tags, candidate.Effective.Selection.Tags) {
		return fmt.Errorf("run Tag selector: accepted preview does not match its immutable candidate")
	}
	if candidate.Preview.Text != "" {
		fmt.Fprint(cmd.OutOrStdout(), candidate.Preview.Text)
		if !strings.HasSuffix(candidate.Preview.Text, "\n") {
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	if opts.DryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "Dry run complete; nothing was applied.")
		return nil
	}
	return applyInstallTagSelectorCandidate(cmd, candidate, opts, result.AcknowledgementAccepted)
}

type tagSelectorBrowseOptions struct {
	OS             string
	Arch           string
	SourceReadRoot string
	Home           string
	StateRoot      string
	XDGStateHome   string
	Lookup         deps.Lookup
	FontLookup     deps.FontLookup
	AppLookup      deps.AppLookup
	CommandRunner  deps.CommandRunner
}

var tagSelectorGroups = []struct {
	Profile string
	Label   string
}{
	{Profile: "core", Label: "Core"},
	{Profile: "desktop", Label: "Desktop"},
	{Profile: "agents", Label: "Agents"},
	{Profile: "web", Label: "Web"},
	{Profile: "mobile", Label: "Mobile"},
}

func buildInstallTagSelectorBrowseData(m manifest.Manifest, meta state.Metadata, opts tagSelectorBrowseOptions) (tagselectortui.BrowseData, error) {
	report, err := catalog.Build(m, catalog.Options{OS: opts.OS})
	if err != nil {
		return tagselectortui.BrowseData{}, fmt.Errorf("build Tag selector browse data from Install Catalog: %w", err)
	}

	currentTags := make(map[string]catalog.TagSummary, len(report.Tags))
	for _, tag := range report.Tags {
		currentTags[tag.Name] = tag
	}
	profileOrder := orderedTagSelectorProfiles(report.Profiles)
	profileByName := make(map[string]catalog.ProfileSummary, len(report.Profiles))
	profileTags := make(map[string][]string, len(report.Profiles))
	memberships := make(map[string][]string, len(report.Tags))
	for _, profile := range report.Profiles {
		profileByName[profile.Name] = profile
		resolved, _, normalizeErr := manifest.NormalizeTags(m, profile.Tags)
		if normalizeErr != nil {
			return tagselectortui.BrowseData{}, fmt.Errorf("normalize Tag selector Profile %q: %w", profile.Name, normalizeErr)
		}
		for _, tag := range resolved {
			if _, ok := currentTags[tag]; !ok {
				continue
			}
			profileTags[profile.Name] = appendUniqueString(profileTags[profile.Name], tag)
		}
	}
	for _, profileName := range profileOrder {
		for _, tag := range profileTags[profileName] {
			memberships[tag] = append(memberships[tag], profileName)
		}
	}

	orderedTags := make([]string, 0, len(report.Tags))
	groups := make(map[string]string, len(report.Tags))
	for _, group := range tagSelectorGroups {
		for _, tag := range profileTags[group.Profile] {
			if _, grouped := groups[tag]; grouped {
				continue
			}
			groups[tag] = group.Label
			orderedTags = append(orderedTags, tag)
		}
	}
	remaining := make([]string, 0, len(report.Tags))
	for _, tag := range report.Tags {
		if _, grouped := groups[tag.Name]; grouped {
			continue
		}
		groups[tag.Name] = "Global"
		remaining = append(remaining, tag.Name)
	}
	sort.Strings(remaining)
	orderedTags = append(orderedTags, remaining...)

	result := tagselectortui.BrowseData{Tags: []tagselectortui.Tag{}, Profiles: []tagselectortui.Profile{}}
	for _, profileName := range profileOrder {
		profile := profileByName[profileName]
		result.Profiles = append(result.Profiles, tagselectortui.Profile{
			Name:        profile.Name,
			Description: profile.Description,
			Tags:        append([]string(nil), profileTags[profileName]...),
		})
	}
	for _, tagName := range orderedTags {
		tag, buildErr := buildInstallTagSelectorTag(m, meta, currentTags[tagName], groups[tagName], memberships[tagName], opts)
		if buildErr != nil {
			return tagselectortui.BrowseData{}, buildErr
		}
		result.Tags = append(result.Tags, tag)
	}
	return result, nil
}

func orderedTagSelectorProfiles(profiles []catalog.ProfileSummary) []string {
	available := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		available[profile.Name] = true
	}
	result := make([]string, 0, len(profiles))
	seen := make(map[string]bool, len(profiles))
	for _, group := range tagSelectorGroups {
		if available[group.Profile] {
			result = append(result, group.Profile)
			seen[group.Profile] = true
		}
	}
	var remaining []string
	for _, profile := range profiles {
		if !seen[profile.Name] {
			remaining = append(remaining, profile.Name)
		}
	}
	sort.Strings(remaining)
	return append(result, remaining...)
}

func buildInstallTagSelectorTag(m manifest.Manifest, meta state.Metadata, summary catalog.TagSummary, group string, profiles []string, opts tagSelectorBrowseOptions) (tagselectortui.Tag, error) {
	effective, err := selection.ResolveIntent(m, selection.Intent{Source: selection.SourceExplicit, ExtraTags: []string{summary.Name}})
	if err != nil {
		return tagselectortui.Tag{}, fmt.Errorf("resolve Tag selector Tag %q: %w", summary.Name, err)
	}
	detailReport, err := catalog.Tag(m, summary.Name, catalog.Options{OS: opts.OS})
	if err != nil {
		return tagselectortui.Tag{}, fmt.Errorf("describe Tag selector Tag %q: %w", summary.Name, err)
	}
	detail := detailReport.Tag
	portableReport, err := catalog.Tag(m, summary.Name, catalog.Options{OS: "all"})
	if err != nil {
		return tagselectortui.Tag{}, fmt.Errorf("describe portable Tag selector Tag %q: %w", summary.Name, err)
	}
	portable := portableReport.Tag

	entryReport, err := status.Build(m, meta, status.Options{
		Selection:    &effective.Selection,
		OS:           opts.OS,
		SourceRoot:   opts.SourceReadRoot,
		Home:         opts.Home,
		XDGStateHome: opts.XDGStateHome,
	})
	if err != nil {
		return tagselectortui.Tag{}, fmt.Errorf("inspect Tag selector Tag %q: %w", summary.Name, err)
	}
	dependencyReport, err := deps.CheckWithToolProbes(m, deps.Options{
		Selection: &effective.Selection,
		OS:        opts.OS,
		Arch:      opts.Arch,
		Home:      opts.Home,
		StateRoot: opts.StateRoot,
		AppLookup: opts.AppLookup,
	}, opts.Lookup, opts.FontLookup, opts.CommandRunner)
	if err != nil {
		return tagselectortui.Tag{}, fmt.Errorf("inspect Tag selector Dependencies for %q: %w", summary.Name, err)
	}
	provisionPlan, err := provision.Build(m, provision.Options{Selection: &effective.Selection, OS: opts.OS, AppLookup: opts.AppLookup})
	if err != nil {
		return tagselectortui.Tag{}, fmt.Errorf("inspect Tag selector Provisioners for %q: %w", summary.Name, err)
	}
	provisionReport := buildTagSelectorProvisionStatus(m, provisionPlan, meta, summary.Name)

	result := tagselectortui.Tag{
		Name:           summary.Name,
		Description:    summary.Description,
		Group:          group,
		Profiles:       append([]string(nil), profiles...),
		Components:     []tagselectortui.Component{},
		ManagedEntries: []string{},
		Dependencies:   []string{},
		Provisioners:   []string{},
	}
	for _, entry := range portable.Entries {
		result.ManagedEntries = appendUniqueString(result.ManagedEntries, entry.Target)
	}
	for _, override := range portable.SourceOverrides {
		result.ManagedEntries = appendUniqueString(result.ManagedEntries, override.Target)
	}
	for _, dependency := range portable.Dependencies {
		result.Dependencies = appendUniqueString(result.Dependencies, dependency.Name)
	}
	for _, item := range portable.Provisioners {
		result.Provisioners = appendUniqueString(result.Provisioners, item.Tool)
	}
	for _, entry := range entryReport.Entries {
		componentDetail := entry.Reason
		if entry.State == status.StateUnsupported && componentDetail == "" {
			componentDetail = "alignment unsupported"
		}
		result.Components = append(result.Components, tagselectortui.Component{
			Kind: "Managed Entry", Name: entry.Target, State: tagSelectorManagedEntryState(entry.State), Detail: componentDetail,
		})
		result.ManagedEntries = appendUniqueString(result.ManagedEntries, entry.Target)
	}
	for _, dependency := range dependencyReport.Results {
		detailText := dependency.Warning
		if detailText == "" {
			detailText = dependency.Command
		}
		result.Components = append(result.Components, tagselectortui.Component{
			Kind: "Dependency", Name: dependency.Name, State: tagSelectorDependencyState(dependency), Detail: detailText,
		})
		result.Dependencies = appendUniqueString(result.Dependencies, dependency.Name)
		result.ExternalEffectsPresent = result.ExternalEffectsPresent || dependency.Present
	}
	for _, item := range provisionReport.Items {
		detailText := strings.Join(item.Missing, ", ")
		if detailText == "" {
			detailText = item.LastRunAt
		}
		result.Components = append(result.Components, tagselectortui.Component{
			Kind: "Provisioner", Name: item.Tool, State: tagSelectorProvisionerState(item.Status), Detail: detailText,
		})
		result.Provisioners = appendUniqueString(result.Provisioners, item.Tool)
		result.ExternalEffectsPresent = result.ExternalEffectsPresent || item.Status == provision.StatusStateProvisioned
	}
	applicableDependencies := make(map[string]bool, len(dependencyReport.Results))
	for _, dependency := range dependencyReport.Results {
		applicableDependencies[dependency.Name] = true
	}
	excludedDependencies := make(map[string]bool)
	for _, dependency := range portable.Dependencies {
		if applicableDependencies[dependency.Name] || excludedDependencies[dependency.Name] {
			continue
		}
		excludedDependencies[dependency.Name] = true
		result.Components = append(result.Components, tagselectortui.Component{
			Kind: "Dependency", Name: dependency.Name, State: tagselectortui.StateNotApplicable, Detail: "not applicable to " + opts.OS,
		})
	}
	for _, excluded := range detail.Excluded {
		if excluded.Type == "dependency_set" {
			continue
		}
		result.Components = append(result.Components, tagselectortui.Component{
			Kind: tagSelectorComponentKind(excluded.Type), Name: excluded.Name, State: tagselectortui.StateNotApplicable, Detail: excluded.Reason,
		})
	}
	result.State = aggregateTagSelectorState(result.Components)
	return result, nil
}

func tagSelectorComponentKind(kind string) string {
	switch kind {
	case "entry":
		return "Managed Entry"
	case "provisioner":
		return "Provisioner"
	case "source_override":
		return "Source Override"
	default:
		return "Selected Surface"
	}
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func tagSelectorManagedEntryState(value status.State) tagselectortui.State {
	switch value {
	case status.StateOK:
		return tagselectortui.StateAligned
	case status.StateMissing:
		return tagselectortui.StateMissing
	case status.StateDrifted:
		return tagselectortui.StateDrift
	case status.StateSkipped:
		return tagselectortui.StateNotApplicable
	case status.StateConflict, status.StateUnsupported:
		return tagselectortui.StateConflict
	default:
		return tagselectortui.StateConflict
	}
}

func tagSelectorDependencyState(value deps.Result) tagselectortui.State {
	if !value.Present {
		return tagselectortui.StateMissing
	}
	if value.Warning != "" {
		return tagselectortui.StateDrift
	}
	return tagselectortui.StateAligned
}

func tagSelectorProvisionerState(value provision.StatusState) tagselectortui.State {
	switch value {
	case provision.StatusStateProvisioned:
		return tagselectortui.StateAligned
	case provision.StatusStatePending, provision.StatusStateMissingDependencies:
		return tagselectortui.StateMissing
	case provision.StatusStateFailed:
		return tagselectortui.StateConflict
	default:
		return tagselectortui.StateConflict
	}
}

func buildTagSelectorProvisionStatus(m manifest.Manifest, plan provision.Plan, meta state.Metadata, tag string) provision.StatusReport {
	selectorMeta := state.Metadata{Provisioners: make([]state.ProvisionerRecord, 0, len(plan.Steps))}
	for _, step := range plan.Steps {
		record, ok := findTagSelectorProvisionerRecord(m, meta.Provisioners, tag, step)
		if !ok {
			continue
		}
		record.Profile = plan.Profile
		selectorMeta.Provisioners = append(selectorMeta.Provisioners, record)
	}
	return provision.BuildStatus(plan, selectorMeta)
}

func findTagSelectorProvisionerRecord(m manifest.Manifest, records []state.ProvisionerRecord, tag string, step provision.Step) (state.ProvisionerRecord, bool) {
	var tagged state.ProvisionerRecord
	hasMatchingTag := false
	var legacy state.ProvisionerRecord
	hasLegacy := false
	hasTagged := false
	for _, record := range records {
		if record.Tool != step.Tool || record.Executable != step.Executable || !slices.Equal(record.Args, step.Args) {
			continue
		}
		if len(record.Tags) == 0 {
			if !hasLegacy || tagSelectorProvisionerRecordIsNewer(record, legacy) {
				legacy, hasLegacy = record, true
			}
			continue
		}
		hasTagged = true
		normalizedTags := normalizeTagSelectorProvisionerRecordTags(m, record.Tags)
		if slices.Contains(normalizedTags, tag) && (!hasMatchingTag || tagSelectorProvisionerRecordIsNewer(record, tagged)) {
			tagged, hasMatchingTag = record, true
		}
	}
	if hasMatchingTag {
		return tagged, true
	}
	if hasTagged {
		return state.ProvisionerRecord{}, false
	}
	return legacy, hasLegacy
}

func normalizeTagSelectorProvisionerRecordTags(m manifest.Manifest, tags []string) []string {
	var normalized []string
	for _, tag := range tags {
		resolved, _, err := manifest.NormalizeTags(m, []string{tag})
		if err != nil {
			continue
		}
		for _, current := range resolved {
			normalized = appendUniqueString(normalized, current)
		}
	}
	return normalized
}

// tagSelectorProvisionerRecordIsNewer prefers valid persisted timestamps. A
// later metadata entry breaks absent, invalid, or equal timestamp ties.
func tagSelectorProvisionerRecordIsNewer(candidate, current state.ProvisionerRecord) bool {
	candidateTime, candidateErr := time.Parse(time.RFC3339Nano, candidate.LastRunAt)
	currentTime, currentErr := time.Parse(time.RFC3339Nano, current.LastRunAt)
	candidateOK := candidateErr == nil
	currentOK := currentErr == nil
	switch {
	case candidateOK && !currentOK:
		return true
	case !candidateOK && currentOK:
		return false
	case candidateOK && currentOK && !candidateTime.Equal(currentTime):
		return candidateTime.After(currentTime)
	default:
		return true
	}
}

func aggregateTagSelectorState(components []tagselectortui.Component) tagselectortui.State {
	if len(components) == 0 {
		return tagselectortui.StateNotApplicable
	}
	precedence := map[tagselectortui.State]int{
		tagselectortui.StateNotApplicable: 0,
		tagselectortui.StateAligned:       1,
		tagselectortui.StateMissing:       2,
		tagselectortui.StateDrift:         3,
		tagselectortui.StateConflict:      4,
	}
	result := tagselectortui.StateNotApplicable
	for _, component := range components {
		if precedence[component.State] > precedence[result] {
			result = component.State
		}
	}
	return result
}

func resolveInstallTagSelectorInitial(m manifest.Manifest, installed *state.InstalledSelection) ([]string, error) {
	if installed == nil {
		return []string{}, nil
	}
	effective, err := selection.ResolveIntent(m, selection.Intent{
		Source:     selection.SourceRecorded,
		Profiles:   installed.Profiles,
		ExtraTags:  installed.ExtraTags,
		AllowEmpty: len(installed.Profiles) == 0 && len(installed.ExtraTags) == 0,
	})
	if err != nil {
		return nil, fmt.Errorf("load Installed Selection for Tag selector: %w", err)
	}
	return append([]string(nil), effective.Selection.Tags...), nil
}

func buildInstallTagSelectorCandidate(m manifest.Manifest, meta state.Metadata, paths resolvedPaths, sourceReadRoot string, legacyMigrations map[string]plan.LegacyMigration, hostOS string, tags []string, opts installTagSelectorOptions) (installTagSelectorCandidate, error) {
	effective, err := selection.ResolveIntent(m, selection.Intent{
		Source:     selection.SourceExplicit,
		ExtraTags:  append([]string(nil), tags...),
		AllowEmpty: len(tags) == 0,
	})
	if err != nil {
		return installTagSelectorCandidate{}, fmt.Errorf("resolve Tag selector draft: %w", err)
	}
	effective = selection.CompareInstalled(m, effective, meta.InstalledSelection, hostOS)

	depTier, err := resolveInstallTier(hostOS)
	if err != nil {
		return installTagSelectorCandidate{}, err
	}
	depOptions := deps.Options{
		Selection: &effective.Selection,
		OS:        hostOS, Arch: installHostArch, Home: paths.Home, StateRoot: paths.StateRoot,
		AppLookup: appInstalled(hostOS, paths.Home), HTTPClient: depsHTTPClient, RollingReleaseURL: depsRollingReleaseURL,
	}
	brewDetection := packageManagerDetector.DetectHomebrew()
	depLookup := packageManagerLookup(brewDetection)
	var preparedDependencies *deps.PreparedInstall
	var packageManagerSetup *pkgmgr.Report
	if !opts.SkipDependencies {
		prepared, prepareErr := deps.PrepareInstall(m, depOptions, depLookup, fontInstalled(hostOS, paths.Home), depTier)
		if prepareErr != nil {
			return installTagSelectorCandidate{}, fmt.Errorf("prepare Tag selector Dependencies: %w", prepareErr)
		}
		setup := pkgmgr.HomebrewSetupNeed(hostOS, prepared.Report, brewDetection)
		if setup.Status != pkgmgr.StatusNotNeeded || setup.Detection.NeedsPATH {
			packageManagerSetup = &setup
		}
		if setup.Status == pkgmgr.StatusWouldOffer {
			// The accepted candidate must already contain the exact actions that
			// become runnable after the separately reviewed setup step succeeds.
			available := brewDetection
			available.Found = true
			prepared, prepareErr = deps.PrepareInstall(m, depOptions, packageManagerLookup(available), fontInstalled(hostOS, paths.Home), depTier)
			if prepareErr != nil {
				return installTagSelectorCandidate{}, fmt.Errorf("prepare post-setup Tag selector Dependencies: %w", prepareErr)
			}
		}
		preparedDependencies = &prepared
	}

	p, provisionPlan, err := buildInstallPlanAndProvisioners(m, meta, effective.Selection, hostOS, paths, sourceReadRoot, legacyMigrations)
	if err != nil {
		return installTagSelectorCandidate{}, fmt.Errorf("build Tag selector preview: %w", err)
	}
	p.Selection = &effective.Report
	p.SelectionReconciliation, err = buildSelectionReconciliation(m, meta, effective, p, hostOS, paths, sourceReadRoot, true)
	if err != nil {
		return installTagSelectorCandidate{}, fmt.Errorf("build Tag selector reconciliation preview: %w", err)
	}
	retirementOptions := selectionretirement.Options{SourceRoot: paths.SourceRoot, Home: paths.Home, StateRoot: paths.StateRoot, ForwardPlan: &p}
	var retirementPlan selectionretirement.Plan
	var retirementBlock string
	if p.SelectionReconciliation != nil {
		retirementPlan, err = selectionretirement.Build(*p.SelectionReconciliation, meta, retirementOptions)
		if err != nil {
			retirementBlock = err.Error()
			retirementPlan = selectionretirement.Plan{}
		}
	}
	capturedSources, err := install.CaptureManagedSources(p, install.Options{
		SourceRoot: paths.SourceRoot, Home: paths.Home, StateRoot: paths.StateRoot,
		ConflictDecisions: replaceAllConflictDecisions(p),
	})
	if err != nil {
		return installTagSelectorCandidate{}, fmt.Errorf("capture Tag selector Source of Truth inputs: %w", err)
	}

	forwardOnly := meta.InstalledSelection == nil
	var rendered bytes.Buffer
	if preparedDependencies != nil {
		renderPackageManagerSetup(&rendered, packageManagerSetup)
		renderDepsInstallPreview(&rendered, preparedDependencies.Report)
		fmt.Fprintln(&rendered)
	} else {
		fmt.Fprintln(&rendered, "Dependency provisioning skipped (--skip-deps).")
		fmt.Fprintln(&rendered)
	}
	if forwardOnly {
		fmt.Fprintln(&rendered, "Forward-only selection preview")
		fmt.Fprintln(&rendered, "No Installed Selection is recorded; no retirement is authorized.")
		fmt.Fprintln(&rendered)
	}
	renderPlan(&rendered, p)
	renderProvisionPlan(&rendered, provisionPlan)

	semantic := struct {
		Manifest             manifest.Manifest                `json:"manifest"`
		Metadata             state.Metadata                   `json:"metadata"`
		Paths                resolvedPaths                    `json:"paths"`
		SourceReadRoot       string                           `json:"source_read_root"`
		LegacyMigrations     map[string]plan.LegacyMigration  `json:"legacy_migrations"`
		ForwardOnly          bool                             `json:"forward_only"`
		Selection            selection.Report                 `json:"selection"`
		Dependencies         *deps.PreparedInstall            `json:"dependencies,omitempty"`
		PackageManagerSetup  *pkgmgr.Report                   `json:"package_manager_setup,omitempty"`
		Plan                 plan.Plan                        `json:"plan"`
		PlanAuthority        []tagSelectorPlanAuthority       `json:"plan_authority"`
		CapturedSources      []install.SourceCaptureAuthority `json:"captured_sources"`
		Provisioners         provision.Plan                   `json:"provisioners"`
		Retirement           selectionretirement.Plan         `json:"retirement"`
		RetirementBlock      string                           `json:"retirement_block,omitempty"`
		HistoricalRetirement string                           `json:"historical_retirement"`
	}{
		Manifest: m, Metadata: meta, Paths: paths, SourceReadRoot: sourceReadRoot, LegacyMigrations: legacyMigrations,
		ForwardOnly: forwardOnly, Selection: effective.Report, Dependencies: preparedDependencies, PackageManagerSetup: packageManagerSetup,
		Plan: p, PlanAuthority: tagSelectorPlanAuthorities(p), CapturedSources: install.SourceCaptureAuthorities(capturedSources),
		Provisioners: provisionPlan, Retirement: retirementPlan, RetirementBlock: retirementBlock,
		HistoricalRetirement: "not-run-selector",
	}
	encoded, err := json.Marshal(semantic)
	if err != nil {
		return installTagSelectorCandidate{}, errors.Join(fmt.Errorf("encode Tag selector preview: %w", err), install.ReleaseCapturedSources(capturedSources))
	}
	digest := sha256.Sum256(encoded)
	preview := tagselectortui.Preview{
		Text: rendered.String(), SemanticDigest: fmt.Sprintf("sha256:%x", digest), ForwardOnly: forwardOnly,
	}
	if len(effective.Selection.Tags) == 0 {
		preview.Confirmation = tagselectortui.ConfirmationClear
	} else if effective.Report.Change != nil && effective.Report.Change.AcknowledgementRequired {
		preview.Confirmation = tagselectortui.ConfirmationReduction
	}
	return installTagSelectorCandidate{
		Manifest: m, Metadata: meta, Paths: paths,
		Effective: effective, Dependencies: preparedDependencies, DependencyOptions: depOptions,
		DependencyLookup: depLookup, BrewDetection: brewDetection, PackageManagerSetup: packageManagerSetup,
		Plan: p, CapturedSources: capturedSources, Provisioners: provisionPlan, Retirement: retirementPlan,
		RetirementBlock: retirementBlock, RetirementOptions: retirementOptions,
		ManifestPath: opts.ManifestPath, RequestedStateRoot: opts.RequestedStateRoot, Preview: preview,
	}, nil
}

type tagSelectorPlanAuthority struct {
	ResolvedSource                string
	ResolvedSources               []string
	Contributions                 []plan.Contribution
	Content                       []byte
	PreviousContent               []byte
	PreviousHash                  string
	PreviousRecordFingerprint     string
	PreviousReconciliationReceipt *state.ReconciliationReceipt
	Ownership                     string
	Migration                     *plan.LegacyMigration
	LegacyParent                  string
}

func tagSelectorPlanAuthorities(p plan.Plan) []tagSelectorPlanAuthority {
	result := make([]tagSelectorPlanAuthority, 0, len(p.Actions))
	for _, action := range p.Actions {
		result = append(result, tagSelectorPlanAuthority{
			ResolvedSource: action.ResolvedSource, ResolvedSources: action.ResolvedSources, Contributions: action.Contributions,
			Content: action.Content, PreviousContent: action.PreviousContent, PreviousHash: action.PreviousHash,
			PreviousRecordFingerprint: action.PreviousRecordFingerprint, PreviousReconciliationReceipt: action.PreviousReconciliationReceipt,
			Ownership: action.Ownership, Migration: action.Migration, LegacyParent: action.LegacyParent,
		})
	}
	return result
}

func applyInstallTagSelectorCandidate(cmd *cobra.Command, candidate installTagSelectorCandidate, opts installTagSelectorOptions, acknowledgementAccepted bool) (resultErr error) {
	effective := candidate.Effective
	if effective.Report.Change != nil {
		change := *effective.Report.Change
		effective.Report.Change = &change
	}
	clearSelection := len(candidate.Effective.Selection.Tags) == 0
	proceed, _, err := guardSelectionChange(cmd, &effective, selectionChangePolicy{ClearSelection: clearSelection, AlreadyAccepted: acknowledgementAccepted})
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	if candidate.RetirementBlock != "" {
		return fmt.Errorf("accepted Tag selector reduction is blocked: %s", candidate.RetirementBlock)
	}

	conflictDecisions, proceed, err := resolveConflictDecisions(cmd, candidate.Plan, candidate.Paths, false, opts.NoTUI, opts.BackupAndReplace)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	installOptions := install.Options{
		SourceRoot: candidate.Paths.SourceRoot, Home: candidate.Paths.Home, StateRoot: candidate.Paths.StateRoot,
		ConflictDecisions: conflictDecisions,
		CapturedSources:   selectorCapturedSourcesForDecisions(candidate.CapturedSources, conflictDecisions),
	}
	adoptSnapshots, err := install.CaptureAdoptSnapshots(candidate.Plan, installOptions)
	if err != nil {
		return err
	}
	defer func() {
		if err := install.ReleaseAdoptSnapshots(adoptSnapshots); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("release Tag selector adopt snapshots: %w", err))
		}
	}()
	installOptions.AdoptSnapshots = adoptSnapshots
	if err := install.ValidateManagedEntries(candidate.Plan, installOptions); err != nil {
		return err
	}
	if err := validateInstallTagSelectorAuthority(candidate); err != nil {
		return err
	}

	dependencyEnvironment, dependenciesReport, err := applyInstallTagSelectorDependencies(cmd, candidate)
	if err != nil {
		return err
	}
	if candidate.Dependencies != nil && dependenciesReport == nil {
		return nil
	}

	beforeBackups, err := backups.Load(backups.Path(candidate.Paths.StateRoot))
	if err != nil {
		return err
	}
	metadataCommit, err := install.ApplyManagedEntries(candidate.Plan, installOptions)
	if err != nil {
		return err
	}
	createdBackups, err := createdBackupSetReports(candidate.Paths.StateRoot, beforeBackups)
	if err != nil {
		return err
	}

	provisionerReport, err := applyAcceptedTagSelectorProvisioners(cmd, candidate, dependencyEnvironment)
	if err != nil {
		return err
	}
	var retirementResult *selectionretirement.Result
	if len(candidate.Retirement.Actions) > 0 {
		result, applyErr := selectionretirement.Apply(candidate.Retirement, candidate.RetirementOptions)
		retirementResult = &result
		if applyErr != nil {
			return applyErr
		}
	}

	installedSelection := candidate.Effective.InstalledSelection(state.CaptureProvenance(candidate.Paths.SourceRoot, version.Value))
	if err := commitSelectorInstallationMetadata(metadataCommit, candidate.Metadata.InstalledSelection, installedSelection); err != nil {
		return err
	}
	renderTagSelectorFinalResult(cmd.OutOrStdout(), candidate, conflictDecisions, dependenciesReport, provisionerReport, retirementResult, len(createdBackups))
	return nil
}

func renderTagSelectorFinalResult(w io.Writer, candidate installTagSelectorCandidate, conflictDecisions map[string]install.ConflictDecision, dependencyReport *deps.InstallReport, provisionerReport provision.Report, retirement *selectionretirement.Result, backupSets int) {
	fmt.Fprintf(w, "Tag selection applied: tags=%s\n", renderSelectionValues(candidate.Effective.Selection.Tags))
	fmt.Fprintf(w, "  Managed Entries: %d planned action(s)\n", len(candidate.Plan.Actions))
	dependencyResults := 0
	if dependencyReport != nil {
		dependencyResults = len(dependencyReport.Items)
	}
	fmt.Fprintf(w, "  Dependencies: %d result(s)\n", dependencyResults)
	fmt.Fprintf(w, "  Provisioners: %d result(s)\n", len(provisionerReport.Items))
	removed, retained := 0, 0
	if retirement != nil {
		removed, retained = len(retirement.Removed), len(retirement.Retained)
	}
	fmt.Fprintf(w, "  Selection retirement: %d removed, %d retained\n", removed, retained)
	fmt.Fprintf(w, "  Backup Sets: %d created\n", backupSets)
	fmt.Fprintln(w, "  Historical retirement: not run (Tag selector path)")
	renderTagSelectorManagedEntryResults(w, candidate.Plan, conflictDecisions)
	renderTagSelectorRetainedExternalState(w, candidate.Plan.SelectionReconciliation)
	renderSelectionRetirement(w, retirement)
}

func renderTagSelectorManagedEntryResults(w io.Writer, p plan.Plan, conflictDecisions map[string]install.ConflictDecision) {
	if len(p.Actions) == 0 {
		return
	}
	fmt.Fprintln(w, "\nManaged Entry results:")
	for _, action := range p.Actions {
		fmt.Fprintf(w, "  %-10s %s\n", tagSelectorManagedEntryOutcome(action, p.SelectionReconciliation, conflictDecisions), action.Target)
	}
}

func tagSelectorManagedEntryOutcome(action plan.Action, report *selectionreconciliation.Report, conflictDecisions map[string]install.ConflictDecision) string {
	if action.Status == plan.StatusConflict {
		switch conflictDecisions[action.Target] {
		case install.DecisionReplace:
			return "replaced"
		case install.DecisionAdopt:
			return "adopted"
		default:
			return "skipped"
		}
	}
	if action.Status == plan.StatusMigrate {
		return "migrated"
	}
	if report != nil {
		for _, semantic := range report.Actions {
			if semantic.Scope != selectionreconciliation.ScopeManagedEntry || semantic.ResolvedTarget != action.Target {
				continue
			}
			switch semantic.Outcome {
			case selectionreconciliation.OutcomeCreate:
				return "created"
			case selectionreconciliation.OutcomeUpdate:
				return "updated"
			case selectionreconciliation.OutcomePreserve:
				return "preserved"
			case selectionreconciliation.OutcomeReconcile:
				return "reconciled"
			}
		}
	}
	switch action.Status {
	case plan.StatusCreate:
		return "created"
	case plan.StatusUpdate:
		return "updated"
	case plan.StatusUnchanged:
		return "preserved"
	case plan.StatusMissingSource:
		return "not-applied"
	default:
		return "unknown"
	}
}

func renderTagSelectorRetainedExternalState(w io.Writer, report *selectionreconciliation.Report) {
	if report == nil {
		return
	}
	printedHeading := false
	for _, action := range report.Actions {
		if action.Outcome != selectionreconciliation.OutcomeRetainedExternalState {
			continue
		}
		if !printedHeading {
			fmt.Fprintln(w, "\nRetained External State:")
			printedHeading = true
		}
		subject := strings.Join(action.Names, ", ")
		if action.Identity != "" {
			subject += " [" + action.Identity + "]"
		}
		fmt.Fprintf(w, "  %-12s %s\n", action.Scope, subject)
	}
}

func selectorCapturedSourcesForDecisions(captures map[install.SourceCaptureKey]install.CapturedSource, decisions map[string]install.ConflictDecision) map[install.SourceCaptureKey]install.CapturedSource {
	selected := make(map[install.SourceCaptureKey]install.CapturedSource, len(captures))
	for key, capture := range captures {
		if decisions[key.Target] == install.DecisionAdopt {
			continue
		}
		selected[key] = capture
	}
	return selected
}

func validateInstallTagSelectorAuthority(candidate installTagSelectorCandidate) error {
	if candidate.ManifestPath != "" {
		current, err := manifest.LoadFile(candidate.ManifestPath)
		if err != nil {
			return fmt.Errorf("revalidate accepted Tag selector Install Manifest: %w", err)
		}
		if !reflect.DeepEqual(*current, candidate.Manifest) {
			return fmt.Errorf("accepted Tag selector candidate is stale: Install Manifest changed before mutation")
		}
	}
	currentMetadata, err := loadInstallationMetadata(candidate.Paths, candidate.RequestedStateRoot)
	if err != nil {
		return fmt.Errorf("revalidate accepted Tag selector Installation Metadata: %w", err)
	}
	if !reflect.DeepEqual(currentMetadata, candidate.Metadata) {
		return fmt.Errorf("accepted Tag selector candidate is stale: Installation Metadata changed before mutation")
	}
	return nil
}

func applyInstallTagSelectorDependencies(cmd *cobra.Command, candidate installTagSelectorCandidate) ([]string, *deps.InstallReport, error) {
	if candidate.Dependencies == nil {
		return nil, &deps.InstallReport{}, nil
	}
	var setup *pkgmgr.Report
	if candidate.PackageManagerSetup != nil {
		copy := *candidate.PackageManagerSetup
		setup = &copy
	}
	brewDetection := candidate.BrewDetection
	brewPath := brewDetection.Path
	if setup != nil && setup.Status == pkgmgr.StatusWouldOffer {
		confirmed, err := confirmPackageManagerSetup(cmd.InOrStdin(), cmd.OutOrStdout(), *setup)
		if err != nil {
			return nil, nil, err
		}
		if !confirmed {
			setup.Status = pkgmgr.StatusDeclined
			fmt.Fprintln(cmd.OutOrStdout(), "Package Manager Setup declined; install canceled before Managed Configuration.")
			return nil, nil, nil
		}
		if err := pkgmgr.RunHomebrewSetup(cmd.Context(), packageManagerSetupRunner, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			setup.Status = pkgmgr.StatusFailed
			return nil, nil, fmt.Errorf("Homebrew Package Manager Setup failed: %w", err)
		}
		brewDetection = packageManagerDetector.DetectHomebrew()
		if !brewDetection.Found {
			setup.Status = pkgmgr.StatusUnavailable
			return nil, nil, fmt.Errorf("Homebrew Package Manager Setup completed but brew was not found on PATH, /opt/homebrew/bin/brew, or /usr/local/bin/brew")
		}
		setup.Status = pkgmgr.StatusInstalled
		brewPath = brewDetection.Path
	}

	prepared := *candidate.Dependencies
	if hasRequiredInstallablePreviewAction(prepared.Report) {
		confirmed, err := confirmDepsInstall(cmd.InOrStdin(), cmd.OutOrStdout())
		if err != nil {
			return nil, nil, err
		}
		if !confirmed {
			fmt.Fprintln(cmd.OutOrStdout(), "Dependency installation cancelled.")
			return nil, nil, nil
		}
	} else if hasInstallablePreviewAction(prepared.Report) {
		report, err := unresolvedInstallReportFromPreview(prepared.Report)
		renderDepsInstall(cmd.OutOrStdout(), report)
		return nil, &report, err
	}

	depOptions := pinResolvedUserLocal(candidate.DependencyOptions, prepared.Report)
	runner := &depsExecRunner{
		ctx: cmd.Context(), stdin: cmd.InOrStdin(), stdout: cmd.OutOrStdout(), stderr: cmd.ErrOrStderr(),
		home: candidate.Paths.Home, stateRoot: depOptions.StateRoot, brewPath: brewPath,
	}
	report, err := deps.InstallPrepared(prepared, depOptions, candidate.DependencyLookup, fontInstalled(depOptions.OS, candidate.Paths.Home), runner)
	renderDepsInstall(cmd.OutOrStdout(), report)
	return runner.Environment(), &report, err
}

func applyAcceptedTagSelectorProvisioners(cmd *cobra.Command, candidate installTagSelectorCandidate, baseEnv []string) (provision.Report, error) {
	report := provision.Report{
		Profile: candidate.Provisioners.Profile, Profiles: append([]string(nil), candidate.Provisioners.Profiles...),
		Tags: append([]string(nil), candidate.Provisioners.Tags...), Items: []provision.RunItem{},
	}
	selected, err := provision.Select(candidate.Manifest, provision.Options{Selection: &candidate.Effective.Selection, OS: candidate.DependencyOptions.OS})
	if err != nil {
		return report, err
	}
	if len(selected) != len(candidate.Provisioners.Steps) {
		return report, fmt.Errorf("accepted Tag selector Provisioner Plan changed shape: %d declarations for %d steps", len(selected), len(candidate.Provisioners.Steps))
	}
	ctx := cmd.Context()
	stdout := cmd.OutOrStdout()
	runner := provisionExecRunner{ctx: ctx, home: candidate.Paths.Home, stdin: cmd.InOrStdin(), stdout: stdout, stderr: cmd.ErrOrStderr(), baseEnv: baseEnv}
	for index, step := range candidate.Provisioners.Steps {
		declaration := selected[index]
		executable, args := provision.RenderCommand(declaration)
		if declaration.Tool != step.Tool || executable != step.Executable || !slices.Equal(args, step.Args) {
			return report, fmt.Errorf("accepted Tag selector Provisioner step %d no longer matches its declaration", index)
		}
		item := provision.RunItem{Tool: step.Tool, Executable: step.Executable, Args: append([]string(nil), step.Args...)}
		if missing := missingAcceptedTagSelectorProvisionerDependencies(declaration, candidate, runner); len(missing) > 0 {
			item.Status = provision.RunStatusMissingDeps
			item.Missing = missing
			report.Items = append(report.Items, item)
			renderProvisionReport(cmd.OutOrStdout(), report)
			if recordErr := recordProvisionerMetadata(candidate.Paths.StateRoot, candidate.Paths.SourceRoot, report); recordErr != nil {
				return report, errors.Join(fmt.Errorf("provisioner %q is missing dependencies: %v", step.Tool, missing), recordErr)
			}
			return report, fmt.Errorf("provisioner %q is missing dependencies: %v", step.Tool, missing)
		}
		if err := runner.Run(step.Executable, step.Args); err != nil {
			item.Status = provision.RunStatusFailed
			report.Items = append(report.Items, item)
			renderProvisionReport(cmd.OutOrStdout(), report)
			if recordErr := recordProvisionerMetadata(candidate.Paths.StateRoot, candidate.Paths.SourceRoot, report); recordErr != nil {
				return report, errors.Join(fmt.Errorf("provisioner %q failed: %w", step.Tool, err), recordErr)
			}
			return report, fmt.Errorf("provisioner %q failed: %w", step.Tool, err)
		}
		item.Status = provision.RunStatusProvisioned
		report.Items = append(report.Items, item)
	}
	if len(report.Items) > 0 {
		renderProvisionReport(cmd.OutOrStdout(), report)
		if err := recordProvisionerMetadata(candidate.Paths.StateRoot, candidate.Paths.SourceRoot, report); err != nil {
			return report, err
		}
	}
	if agents := selectedCodeGraphAgents(selected); len(agents) > 0 {
		if err := codexconfig.EnsureCodeGraphMode(candidate.Paths.Home, agents...); err != nil {
			return report, err
		}
	}
	return report, nil
}

func missingAcceptedTagSelectorProvisionerDependencies(declaration manifest.Provisioner, candidate installTagSelectorCandidate, runner provisionExecRunner) []string {
	probeOptions := deps.Options{OS: candidate.DependencyOptions.OS, AppLookup: candidate.DependencyOptions.AppLookup}
	fontLookup := fontInstalled(candidate.DependencyOptions.OS, candidate.Paths.Home)
	var missing []string
	for _, dependency := range declaration.Dependencies {
		if deps.DependencyPresent(dependency, probeOptions, runner.Lookup, fontLookup) {
			continue
		}
		if dependency.IsFont() {
			matches := dependency.FontMatches()
			if len(matches) > 0 {
				missing = append(missing, matches[0])
			}
			continue
		}
		missing = append(missing, dependency.Probe())
	}
	return missing
}
