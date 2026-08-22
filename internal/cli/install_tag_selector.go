package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/catalog"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
	tagselectortui "github.com/yersonargotev/dots/internal/tui/tagselector"
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

func runInstallTagSelectorPreview(cmd *cobra.Command, m manifest.Manifest, meta state.Metadata, paths resolvedPaths, sourceReadRoot string, legacyMigrations map[string]plan.LegacyMigration) error {
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
	preview := func(tags []string) (tagselectortui.Preview, error) {
		return buildInstallTagSelectorPreview(m, meta, paths, sourceReadRoot, legacyMigrations, installHostOS, tags)
	}
	result, err := installTagSelectorRunner(cmd.InOrStdin(), cmd.OutOrStdout(), browseData, initial, preview)
	if errors.Is(err, tagselectortui.ErrCanceled) {
		fmt.Fprintln(cmd.OutOrStdout(), "Tag selection canceled; nothing was applied.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("run Tag selector: %w", err)
	}
	if result.Preview.SemanticDigest == "" {
		return fmt.Errorf("run Tag selector: completed without an accepted preview")
	}
	if result.Preview.Text != "" {
		fmt.Fprint(cmd.OutOrStdout(), result.Preview.Text)
		if !strings.HasSuffix(result.Preview.Text, "\n") {
			fmt.Fprintln(cmd.OutOrStdout())
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Selection preview only; nothing was applied.")
	return nil
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

func buildInstallTagSelectorPreview(m manifest.Manifest, meta state.Metadata, paths resolvedPaths, sourceReadRoot string, legacyMigrations map[string]plan.LegacyMigration, hostOS string, tags []string) (tagselectortui.Preview, error) {
	effective, err := selection.ResolveIntent(m, selection.Intent{
		Source:     selection.SourceExplicit,
		ExtraTags:  append([]string(nil), tags...),
		AllowEmpty: len(tags) == 0,
	})
	if err != nil {
		return tagselectortui.Preview{}, fmt.Errorf("resolve Tag selector draft: %w", err)
	}
	effective = selection.CompareInstalled(m, effective, meta.InstalledSelection, hostOS)
	p, provisionPlan, err := buildInstallPlanAndProvisioners(m, meta, effective.Selection, hostOS, paths, sourceReadRoot, legacyMigrations)
	if err != nil {
		return tagselectortui.Preview{}, fmt.Errorf("build Tag selector preview: %w", err)
	}
	p.Selection = &effective.Report
	p.SelectionReconciliation, err = buildSelectionReconciliation(m, meta, effective, p, hostOS, paths, sourceReadRoot, true)
	if err != nil {
		return tagselectortui.Preview{}, fmt.Errorf("build Tag selector reconciliation preview: %w", err)
	}

	forwardOnly := meta.InstalledSelection == nil
	var rendered bytes.Buffer
	if forwardOnly {
		fmt.Fprintln(&rendered, "Forward-only selection preview")
		fmt.Fprintln(&rendered, "No Installed Selection is recorded; no retirement is authorized.")
		fmt.Fprintln(&rendered)
	}
	renderPlan(&rendered, p)
	renderProvisionPlan(&rendered, provisionPlan)

	semantic := struct {
		ForwardOnly  bool             `json:"forward_only"`
		Selection    selection.Report `json:"selection"`
		Plan         plan.Plan        `json:"plan"`
		Provisioners provision.Plan   `json:"provisioners"`
	}{
		ForwardOnly: forwardOnly, Selection: effective.Report, Plan: p, Provisioners: provisionPlan,
	}
	encoded, err := json.Marshal(semantic)
	if err != nil {
		return tagselectortui.Preview{}, fmt.Errorf("encode Tag selector preview: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return tagselectortui.Preview{
		Text: rendered.String(), SemanticDigest: fmt.Sprintf("sha256:%x", digest), ForwardOnly: forwardOnly,
	}, nil
}
