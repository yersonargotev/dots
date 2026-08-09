package cli

import (
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	inst "github.com/yersonargotev/dots/internal/installed"
	"github.com/yersonargotev/dots/internal/selectionmigration"
	"github.com/yersonargotev/dots/internal/state"
)

type installedReport struct {
	inst.Report
	SelectionMigration *selectionmigration.Candidate `json:"selection_migration,omitempty"`
}

func (r installedReport) HasFindings() bool { return false }

func newInstalledCommand() *cobra.Command {
	var (
		file       string
		sourceRoot string
		home       string
		stateRoot  string
	)

	cmd := &cobra.Command{
		Use:   "installed",
		Short: "Show what dots has installed from Installation Metadata",
		Long:  "installed reads Installation Metadata and the Install Manifest to list installed Managed Entries, represented tags, inferred or recorded profiles, Provisioners, and Source of Truth provenance. It is read-only and never evaluates or mutates managed targets.",
		Args:  cobra.NoArgs,
		// Domain errors (bad metadata, missing manifest) are user-facing messages,
		// not command misuse.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths(home, sourceRoot, stateRoot)
			if err != nil {
				return err
			}
			m, err := loadManifestForCommand(cmd, file, paths.SourceRoot)
			if err != nil {
				return err
			}
			meta, err := loadInstallationMetadata(paths, stateRoot)
			if err != nil {
				return err
			}
			report, err := inst.Build(*m, meta, inst.Options{
				StatePath:  state.Path(paths.StateRoot),
				SourceRoot: paths.SourceRoot,
				Home:       paths.Home,
				OS:         runtime.GOOS,
			})
			if err != nil {
				return err
			}
			analysis, err := selectionmigration.Analyze(*m, meta, selectionmigration.Options{
				OS: runtime.GOOS, Home: paths.Home, SourceRoot: paths.SourceRoot, StatePath: state.Path(paths.StateRoot), XDGStateHome: paths.XDGStateHome,
			})
			if err != nil {
				return err
			}
			fullReport := installedReport{Report: report}
			if analysis.Required {
				fullReport.SelectionMigration = analysis.Candidate
			}
			return renderOrEmit(cmd, fullReport, func() error {
				renderInstalled(cmd.OutOrStdout(), report)
				renderSelectionMigration(cmd.OutOrStdout(), fullReport.SelectionMigration)
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to use for tag/profile inference")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "home directory for resolving manifest targets (default: the current user's home); use a sandbox path for validation")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata (default ~/.local/state/dots)")
	return cmd
}

func renderSelectionMigration(w io.Writer, candidate *selectionmigration.Candidate) {
	if candidate == nil {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Selection migration candidate (non-authoritative)")
	fmt.Fprintf(w, "  Profiles: %s\n", renderListOrNone(candidate.Profiles))
	fmt.Fprintf(w, "  Extra Tags: %s\n", renderListOrNone(candidate.ExtraTags))
	fmt.Fprintf(w, "  Effective Tags: %s\n", renderListOrNone(candidate.EffectiveTags))
	fmt.Fprintf(w, "  Confidence: %s\n", candidate.Confidence)
	fmt.Fprintf(w, "  Ambiguity Reasons: %s\n", renderListOrNone(candidate.AmbiguityReasonStrings()))
	if candidate.RecommendedCommand != "" {
		fmt.Fprintf(w, "  Recommended Command: %s\n", candidate.RecommendedCommand)
	}
}

func renderInstalled(w io.Writer, report inst.Report) {
	fmt.Fprintln(w, "Installed inventory")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Metadata: %s", report.Metadata.Path)
	if report.Metadata.Version != 0 {
		fmt.Fprintf(w, " (version %d)", report.Metadata.Version)
	}
	fmt.Fprintln(w)
	if !report.Provenance.Empty() {
		fmt.Fprintf(w, "Provenance: %s\n", renderProvenance(report.Provenance, true))
	} else {
		fmt.Fprintln(w, "Provenance: unknown (metadata predates provenance capture)")
	}

	fmt.Fprintln(w)
	if report.InstalledSelection == nil {
		fmt.Fprintln(w, "Installed Selection: none recorded (historical inventory below is non-authoritative)")
	} else {
		selection := report.InstalledSelection
		fmt.Fprintln(w, "Installed Selection (authoritative)")
		fmt.Fprintf(w, "  Profiles: %s\n", renderListOrNone(selection.Profiles))
		fmt.Fprintf(w, "  Extra Tags: %s\n", renderListOrNone(selection.ExtraTags))
		fmt.Fprintf(w, "  Resolved Tags: %s\n", renderListOrNone(selection.ResolvedTags))
		fmt.Fprintf(w, "  Source of Truth: %s\n", renderProvenance(selection.Provenance, false))
		fmt.Fprintf(w, "  Recorded At: %s\n", renderValueOrUnknown(selection.Provenance.RecordedAt))
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Historical inventory (non-authoritative)")
	if len(report.ManagedEntries) == 0 {
		fmt.Fprintln(w, "Managed Entries: none recorded")
	} else {
		fmt.Fprintf(w, "Managed Entries (%d)\n", len(report.ManagedEntries))
		for _, entry := range report.ManagedEntries {
			fmt.Fprintf(w, "  %-8s %s -> %s\n", entry.Strategy, entry.Source, entry.Target)
			fmt.Fprintf(w, "    tags: %s (%s)\n", renderListOrUnknown(entry.Tags), entry.TagsSource)
			fmt.Fprintf(w, "    profiles: %s (%s)\n", renderListOrUnknown(entry.Profiles), entry.ProfilesSource)
			if entry.InstalledAt != "" {
				fmt.Fprintf(w, "    installed: %s\n", entry.InstalledAt)
			}
			if !entry.ManifestMatched {
				fmt.Fprintln(w, "    manifest: no current match")
			}
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Tags represented: %s\n", renderListOrNone(report.Tags))

	fmt.Fprintln(w)
	if len(report.Profiles) == 0 {
		fmt.Fprintln(w, "Profiles: none inferred or recorded")
	} else {
		fmt.Fprintln(w, "Profiles")
		for _, profile := range report.Profiles {
			fmt.Fprintf(w, "  %-18s %-8s %s\n", profile.Name, profile.State, profile.Source)
			fmt.Fprintf(w, "    tags: %s", renderListOrNone(profile.CoveredTags))
			if len(profile.MissingTags) > 0 {
				fmt.Fprintf(w, " (missing: %s)", strings.Join(profile.MissingTags, ", "))
			}
			fmt.Fprintln(w)
			fmt.Fprintf(w, "    managed entries: %d/%d\n", profile.CoveredEntries, profile.TotalEntries)
			fmt.Fprintf(w, "    provisioners: %d/%d recorded, %d provisioned\n", profile.RecordedProvisioners, profile.TotalProvisioners, profile.ProvisionedProvisioners)
		}
	}

	fmt.Fprintln(w)
	if len(report.Provisioners) == 0 {
		fmt.Fprintln(w, "Provisioners: none recorded")
	} else {
		fmt.Fprintf(w, "Provisioners (%d)\n", len(report.Provisioners))
		for _, prov := range report.Provisioners {
			fmt.Fprintf(w, "  %-22s %s %s\n", prov.Status, prov.Executable, strings.Join(prov.Args, " "))
			fmt.Fprintf(w, "    tags: %s (%s)\n", renderListOrUnknown(prov.Tags), prov.TagsSource)
			fmt.Fprintf(w, "    profiles: %s (%s)\n", renderListOrUnknown(prov.Profiles), prov.ProfilesSource)
			if prov.LastRunAt != "" {
				fmt.Fprintf(w, "    last run: %s\n", prov.LastRunAt)
			}
			if len(prov.Missing) > 0 {
				fmt.Fprintf(w, "    missing: %s\n", strings.Join(prov.Missing, ", "))
			}
			if !prov.ManifestMatched {
				fmt.Fprintln(w, "    manifest: no current match")
			}
		}
	}

	if len(report.Notes) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Notes")
		for _, note := range report.Notes {
			fmt.Fprintf(w, "  - %s\n", note)
		}
	}
}

func renderListOrUnknown(values []string) string {
	if len(values) == 0 {
		return "unknown"
	}
	return strings.Join(values, ", ")
}

func renderListOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func renderProvenance(provenance state.Provenance, includeRecordedAt bool) string {
	parts := []string{}
	if provenance.SourceRoot != "" {
		parts = append(parts, "source="+provenance.SourceRoot)
	}
	if provenance.SourceRevision != "" {
		parts = append(parts, "commit="+provenance.SourceRevision)
	}
	if provenance.DotsVersion != "" {
		parts = append(parts, "dots="+provenance.DotsVersion)
	}
	if includeRecordedAt && provenance.RecordedAt != "" {
		parts = append(parts, "recorded="+provenance.RecordedAt)
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, ", ")
}

func renderValueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
