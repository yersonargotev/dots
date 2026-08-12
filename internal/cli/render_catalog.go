package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/catalog"
)

func renderCatalogSummary(cmd *cobra.Command, report catalog.Report) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Catalog (OS: %s; metadata: %s)\n", report.OS, report.MetadataOrigin)
	renderCatalogProfilesTo(w, report.Profiles)
	renderCatalogTagsTo(w, report.Tags)
	renderCatalogHidden(w, report.Hidden)
}

func renderCatalogProfiles(cmd *cobra.Command, report catalog.Report) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Profiles (OS: %s; metadata: %s)\n", report.OS, report.MetadataOrigin)
	renderCatalogProfilesTo(w, report.Profiles)
	if report.Hidden.Profiles > 0 {
		fmt.Fprintf(w, "Hidden legacy profiles: %d (use --all to include)\n", report.Hidden.Profiles)
	}
}

func renderCatalogTags(cmd *cobra.Command, report catalog.Report) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Tags (OS: %s; metadata: %s)\n", report.OS, report.MetadataOrigin)
	renderCatalogTagsTo(w, report.Tags)
	if report.Hidden.Tags > 0 {
		fmt.Fprintf(w, "Hidden legacy tags: %d (use --all to include)\n", report.Hidden.Tags)
	}
}

func renderCatalogProfilesTo(w io.Writer, profiles []catalog.ProfileSummary) {
	fmt.Fprintln(w, "Profiles:")
	if len(profiles) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, profile := range profiles {
		fmt.Fprintf(w, "  - %s (%s)", profile.Name, profile.Status)
		if profile.Description != "" {
			fmt.Fprintf(w, ": %s", profile.Description)
		}
		fmt.Fprintf(w, " [tags: %s]\n", catalogList(profile.Tags))
	}
}

func renderCatalogTagsTo(w io.Writer, tags []catalog.TagSummary) {
	fmt.Fprintln(w, "Tags:")
	if len(tags) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, tag := range tags {
		fmt.Fprintf(w, "  - %s (%s, %s, %s)", tag.Name, tag.Kind, tag.Status, tag.Origin)
		if tag.Description != "" {
			fmt.Fprintf(w, ": %s", tag.Description)
		}
		if tag.ReplacedBy != "" {
			fmt.Fprintf(w, " [replaced by: %s]", tag.ReplacedBy)
		}
		fmt.Fprintln(w)
	}
}

func renderCatalogHidden(w io.Writer, hidden catalog.Hidden) {
	if hidden.Profiles == 0 && hidden.Tags == 0 {
		return
	}
	fmt.Fprintln(w, "Hidden legacy items:")
	if hidden.Profiles > 0 {
		fmt.Fprintf(w, "  profiles: %d\n", hidden.Profiles)
	}
	if hidden.Tags > 0 {
		fmt.Fprintf(w, "  tags: %d\n", hidden.Tags)
	}
	fmt.Fprintln(w, "  use --all to include")
}

func renderCatalogProfileDetail(cmd *cobra.Command, report catalog.Report) {
	renderCatalogDetail(cmd.OutOrStdout(), "Profile", report, report.Profile)
}

func renderCatalogTagDetail(cmd *cobra.Command, report catalog.Report) {
	renderCatalogDetail(cmd.OutOrStdout(), "Tag", report, report.Tag)
}

func renderCatalogComparison(cmd *cobra.Command, report catalog.Report) {
	comparison := report.Comparison
	if comparison == nil {
		return
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Profile comparison: %s -> %s\n", comparison.From, comparison.To)
	fmt.Fprintf(w, "OS: %s\n", report.OS)
	fmt.Fprintf(w, "Metadata origin: %s\n", report.MetadataOrigin)
	renderCatalogComparisonSurface(w, "Added", "+", comparison.Added)
	renderCatalogComparisonSurface(w, "Removed", "-", comparison.Removed)
	fmt.Fprintln(w, "Shared:")
	fmt.Fprintf(w, "  %d tags, %d dependencies, %d entries, %d source overrides, %d provisioners, %d behaviors\n",
		comparison.Shared.ResolvedTags,
		comparison.Shared.Dependencies,
		comparison.Shared.Entries,
		comparison.Shared.SourceOverrides,
		comparison.Shared.Provisioners,
		comparison.Shared.Behaviors,
	)
}

func renderCatalogMap(cmd *cobra.Command, report catalog.Report) {
	profileMap := report.Map
	if profileMap == nil {
		return
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Profile map %q (OS: %s)\n", profileMap.Profile, report.OS)
	if profileMap.Description != "" {
		fmt.Fprintf(w, "%s\n", profileMap.Description)
	}
	fmt.Fprintf(w, "%s\n", profileMap.Profile)
	for i, tag := range profileMap.Tags {
		branch := "├─"
		continuation := "│  "
		if i == len(profileMap.Tags)-1 {
			branch = "└─"
			continuation = "   "
		}
		fmt.Fprintf(w, "%s %s", branch, tag.Name)
		if tag.Description != "" {
			fmt.Fprintf(w, " — %s", tag.Description)
		}
		fmt.Fprintf(w, "\n%s└─ %d %s · %d %s · %d %s\n", continuation,
			tag.Surface.Entries, countLabel(tag.Surface.Entries, "entry", "entries"),
			tag.Surface.Dependencies, countLabel(tag.Surface.Dependencies, "dependency", "dependencies"),
			tag.Surface.Provisioners, countLabel(tag.Surface.Provisioners, "provisioner", "provisioners"),
		)
	}
	fmt.Fprintf(w, "Total: %d %s · %d unique %s · %d %s\n",
		profileMap.Total.Entries, countLabel(profileMap.Total.Entries, "entry", "entries"),
		profileMap.Total.Dependencies, countLabel(profileMap.Total.Dependencies, "dependency", "dependencies"),
		profileMap.Total.Provisioners, countLabel(profileMap.Total.Provisioners, "provisioner", "provisioners"),
	)
}

func renderCatalogWhy(cmd *cobra.Command, report catalog.Report) {
	why := report.Why
	if why == nil {
		return
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Why profile %q selects %q (OS: %s)\n", why.Profile, why.Query, report.OS)
	fmt.Fprintf(w, "%s\n", why.Profile)
	for matchIndex, match := range why.Matches {
		matchBranch := "├─"
		continuation := "│  "
		if matchIndex == len(why.Matches)-1 {
			matchBranch = "└─"
			continuation = "   "
		}
		fmt.Fprintf(w, "%s %s %s\n", matchBranch, match.Type, match.Identity)
		fmt.Fprintf(w, "%s├─ selected by tags: %s\n", continuation, catalogList(match.ContributingTags))
		switch match.Type {
		case "dependency":
			renderCatalogWhyDependency(w, continuation, match.Dependency)
		case "entry":
			renderCatalogWhyEntry(w, continuation, match.Entry)
		case "provisioner":
			renderCatalogWhyProvisioner(w, continuation, match.Identity, match.Provisioner)
		}
	}
}

func renderCatalogWhyDependency(w io.Writer, prefix string, dependency *catalog.WhyDependency) {
	if dependency == nil {
		return
	}
	for index, declaration := range dependency.Declarations {
		branch := "├─"
		if index == len(dependency.Declarations)-1 {
			branch = "└─"
		}
		fmt.Fprintf(w, "%s%s declared by %s (%s)\n", prefix, branch, renderCatalogOrigin(declaration.Origin), declaration.Requirement)
	}
}

func renderCatalogWhyEntry(w io.Writer, prefix string, entry *catalog.WhyEntry) {
	if entry == nil {
		return
	}
	if entry.SourceOverrideTag != "" {
		fmt.Fprintf(w, "%s└─ %s -> %s (%s; source override: %s)\n", prefix, entry.Entry.Source, entry.Entry.Target, entry.Entry.Strategy, entry.SourceOverrideTag)
		return
	}
	fmt.Fprintf(w, "%s└─ %s -> %s (%s)\n", prefix, entry.Entry.Source, entry.Entry.Target, entry.Entry.Strategy)
}

func renderCatalogWhyProvisioner(w io.Writer, prefix, identity string, provisioner *catalog.Provisioner) {
	if provisioner == nil {
		return
	}
	fmt.Fprintf(w, "%s└─ %s %s: %s\n", prefix, provisioner.Tool, provisioner.Operation, identity)
}

func countLabel(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func renderCatalogComparisonSurface(w io.Writer, heading, marker string, surface catalog.ComparisonSurface) {
	fmt.Fprintf(w, "%s:\n", heading)
	empty := len(surface.ResolvedTags) == 0 && len(surface.Dependencies) == 0 && len(surface.Entries) == 0 && len(surface.SourceOverrides) == 0 && len(surface.Provisioners) == 0 && len(surface.Behaviors) == 0
	if empty {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, tag := range surface.ResolvedTags {
		fmt.Fprintf(w, "  %s tag %s\n", marker, tag)
	}
	for _, dependency := range surface.Dependencies {
		fmt.Fprintf(w, "  %s dependency %s (%s)\n", marker, dependency.Name, dependency.Requirement)
	}
	for _, entry := range surface.Entries {
		fmt.Fprintf(w, "  %s entry %s -> %s (%s)\n", marker, entry.Source, entry.Target, entry.Strategy)
	}
	for _, override := range surface.SourceOverrides {
		fmt.Fprintf(w, "  %s source override %s: %s -> %s\n", marker, override.Tag, override.Source, override.Target)
	}
	for _, provisioner := range surface.Provisioners {
		fmt.Fprintf(w, "  %s provisioner %s (%s", marker, provisioner.Tool, provisioner.Operation)
		if provisioner.Identity != "" {
			fmt.Fprintf(w, ": %s", provisioner.Identity)
		}
		fmt.Fprintln(w, ")")
	}
	for _, behavior := range surface.Behaviors {
		fmt.Fprintf(w, "  %s behavior %s\n", marker, behavior.Action)
	}
}

func renderCatalogDetail(w io.Writer, label string, report catalog.Report, detail *catalog.Detail) {
	if detail == nil {
		return
	}
	fmt.Fprintf(w, "%s %q\n", label, detail.Name)
	fmt.Fprintf(w, "OS: %s\n", report.OS)
	fmt.Fprintf(w, "Metadata origin: %s\n", report.MetadataOrigin)
	if detail.Description != "" {
		fmt.Fprintf(w, "Description: %s\n", detail.Description)
	}
	fmt.Fprintf(w, "Status: %s\n", detail.Status)
	if detail.Kind != "" {
		fmt.Fprintf(w, "Kind: %s\n", detail.Kind)
	}
	if detail.ReplacedBy != "" {
		fmt.Fprintf(w, "Replaced by: %s\n", detail.ReplacedBy)
	}
	fmt.Fprintf(w, "Resolved tags: %s\n", catalogList(detail.ResolvedTags))
	renderCatalogDependencySets(w, detail.DependencySets)
	renderCatalogDependencies(w, "Dependencies", detail.Dependencies)
	renderCatalogEntries(w, detail.Entries)
	renderCatalogSourceOverrides(w, detail.SourceOverrides)
	renderCatalogProvisioners(w, detail.Provisioners)
	renderCatalogBehaviors(w, detail.Behaviors)
	renderCatalogExcluded(w, detail.Excluded)
}

func renderCatalogDependencies(w io.Writer, heading string, dependencies []catalog.Dependency) {
	fmt.Fprintf(w, "%s:\n", heading)
	if len(dependencies) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, dep := range dependencies {
		fmt.Fprintf(w, "  - %s (%s; origin: %s)\n", dep.Name, dep.Requirement, renderCatalogOrigin(dep.Origin))
		if len(dep.Probes) > 0 {
			fmt.Fprintf(w, "    probes: %s\n", catalogList(dep.Probes))
		}
	}
}

func renderCatalogDependencySets(w io.Writer, sets []catalog.DependencySet) {
	fmt.Fprintln(w, "Dependency sets:")
	if len(sets) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, set := range sets {
		fmt.Fprintf(w, "  - tags: %s; OS: %s\n", catalogList(set.Tags), catalogList(set.OS))
		for _, dep := range set.Dependencies {
			fmt.Fprintf(w, "    - %s (%s)\n", dep.Name, dep.Requirement)
		}
	}
}

func renderCatalogEntries(w io.Writer, entries []catalog.Entry) {
	fmt.Fprintln(w, "Entries:")
	if len(entries) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, entry := range entries {
		fmt.Fprintf(w, "  - %s -> %s (%s; tags: %s; OS: %s)", entry.Source, entry.Target, entry.Strategy, catalogList(entry.Tags), catalogList(entry.OS))
		if entry.TargetRoot != "" {
			fmt.Fprintf(w, " [target root: %s]", entry.TargetRoot)
		}
		if entry.Ownership != "" {
			fmt.Fprintf(w, " [ownership: %s]", entry.Ownership)
		}
		fmt.Fprintln(w)
		for _, dep := range entry.Dependencies {
			fmt.Fprintf(w, "    dependency: %s (%s)\n", dep.Name, dep.Requirement)
		}
	}
}

func renderCatalogSourceOverrides(w io.Writer, overrides []catalog.SourceOverride) {
	fmt.Fprintln(w, "Source overrides:")
	if len(overrides) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, override := range overrides {
		state := "not applicable"
		if override.Applicable {
			state = "applicable"
		}
		fmt.Fprintf(w, "  - %s: %s -> %s (entry: %s; OS: %s; %s)\n", override.Tag, override.Source, override.Target, override.Entry, catalogList(override.OS), state)
	}
}

func renderCatalogProvisioners(w io.Writer, provisioners []catalog.Provisioner) {
	fmt.Fprintln(w, "Provisioners:")
	if len(provisioners) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, provisioner := range provisioners {
		fmt.Fprintf(w, "  - %s (%s", provisioner.Tool, provisioner.Operation)
		if provisioner.Identity != "" {
			fmt.Fprintf(w, ": %s", provisioner.Identity)
		}
		fmt.Fprintf(w, "; tags: %s; OS: %s)\n", catalogList(provisioner.Tags), catalogList(provisioner.OS))
		if provisioner.Scope != "" {
			fmt.Fprintf(w, "    scope: %s\n", provisioner.Scope)
		}
		if len(provisioner.Agents) > 0 {
			fmt.Fprintf(w, "    agents: %s\n", catalogList(provisioner.Agents))
		}
		if len(provisioner.Skills) > 0 {
			fmt.Fprintf(w, "    skills: %s\n", catalogList(provisioner.Skills))
		}
		if len(provisioner.Command) > 0 {
			fmt.Fprintf(w, "    command: %s\n", strings.Join(provisioner.Command, " "))
		}
		if len(provisioner.EnvironmentNames) > 0 {
			fmt.Fprintf(w, "    environment names: %s\n", catalogList(provisioner.EnvironmentNames))
		}
		for _, dep := range provisioner.Dependencies {
			fmt.Fprintf(w, "    dependency: %s (%s)\n", dep.Name, dep.Requirement)
		}
	}
}

func renderCatalogBehaviors(w io.Writer, behaviors []catalog.Behavior) {
	fmt.Fprintln(w, "Behaviors:")
	if len(behaviors) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, behavior := range behaviors {
		fmt.Fprintf(w, "  - %s: %s\n", behavior.Action, behavior.Description)
	}
}

func renderCatalogExcluded(w io.Writer, excluded []catalog.ExcludedSurface) {
	fmt.Fprintln(w, "Excluded surfaces:")
	if len(excluded) == 0 {
		fmt.Fprintln(w, "  none")
		return
	}
	for _, item := range excluded {
		fmt.Fprintf(w, "  - %s %s (OS: %s): %s\n", item.Type, item.Name, catalogList(item.OS), item.Reason)
	}
}

func renderCatalogOrigin(origin catalog.Origin) string {
	parts := []string{origin.Type}
	if origin.Name != "" {
		parts = append(parts, origin.Name)
	}
	if len(origin.Tags) > 0 {
		parts = append(parts, "tags="+catalogList(origin.Tags))
	}
	return strings.Join(parts, " ")
}

func catalogList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
