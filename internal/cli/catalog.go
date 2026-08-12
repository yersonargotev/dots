package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/catalog"
	"github.com/yersonargotev/dots/internal/manifest"
)

// newCatalogCommand exposes the portable, manifest-declared configuration
// catalog. It deliberately has no home or state flags: catalog data does not
// describe the current workstation or an Installed Selection.
func newCatalogCommand() *cobra.Command {
	var (
		file       string
		sourceRoot string
		osName     string
		includeAll bool
	)

	load := func(cmd *cobra.Command) (*catalog.Report, error) {
		m, err := loadCatalogManifest(cmd, file, sourceRoot)
		if err != nil {
			return nil, err
		}
		report, err := catalog.Build(*m, catalog.Options{OS: osName, IncludeLegacy: includeAll})
		if err != nil {
			return nil, err
		}
		return &report, nil
	}

	cmd := &cobra.Command{
		Use:          "catalog",
		Short:        "Inspect the portable configuration catalog",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := load(cmd)
			if err != nil {
				return err
			}
			return renderOrEmitCatalog(cmd, *report, renderCatalogSummary)
		},
	}

	cmd.PersistentFlags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to inspect")
	cmd.PersistentFlags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.PersistentFlags().StringVar(&osName, "os", "", "catalog OS: darwin, linux, or all (default: host OS)")
	cmd.PersistentFlags().BoolVar(&includeAll, "all", false, "include legacy profiles and tags in list views")
	_ = cmd.RegisterFlagCompletionFunc("os", completeCatalogOS)

	cmd.AddCommand(newCatalogProfilesCommand(load))
	cmd.AddCommand(newCatalogProfileCommand())
	cmd.AddCommand(newCatalogCompareCommand())
	cmd.AddCommand(newCatalogMapCommand())
	cmd.AddCommand(newCatalogWhyCommand())
	cmd.AddCommand(newCatalogTagsCommand(load))
	cmd.AddCommand(newCatalogTagCommand())
	return cmd
}

func newCatalogWhyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "why PROFILE QUERY",
		Short:        "Explain why a profile selects an item",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := loadCatalogWhy(cmd, args[0], args[1])
			if err != nil {
				return err
			}
			return renderOrEmitCatalog(cmd, report, renderCatalogWhy)
		},
	}
	cmd.ValidArgsFunction = completeCatalogWhy
	return cmd
}

func newCatalogMapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "map PROFILE",
		Short:        "Show how a profile composes its portable surface",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := loadCatalogMap(cmd, args[0])
			if err != nil {
				return err
			}
			return renderOrEmitCatalog(cmd, report, renderCatalogMap)
		},
	}
	cmd.ValidArgsFunction = completeCatalogProfiles
	return cmd
}

func newCatalogCompareCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "compare FROM TO",
		Short:        "Compare the portable surfaces of two profiles",
		Args:         cobra.ExactArgs(2),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := loadCatalogComparison(cmd, args[0], args[1])
			if err != nil {
				return err
			}
			return renderOrEmitCatalog(cmd, report, renderCatalogComparison)
		},
	}
	cmd.ValidArgsFunction = completeCatalogComparisonProfiles
	return cmd
}

type catalogLoader func(*cobra.Command) (*catalog.Report, error)

func newCatalogProfilesCommand(load catalogLoader) *cobra.Command {
	return &cobra.Command{
		Use:          "profiles",
		Short:        "List manifest profiles",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := load(cmd)
			if err != nil {
				return err
			}
			return renderOrEmitCatalog(cmd, *report, renderCatalogProfiles)
		},
	}
}

func newCatalogProfileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "profile NAME",
		Short:        "Show a manifest profile in detail",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := loadCatalogProfile(cmd, args[0])
			if err != nil {
				return err
			}
			return renderOrEmitCatalog(cmd, report, renderCatalogProfileDetail)
		},
	}
	cmd.ValidArgsFunction = completeCatalogProfiles
	return cmd
}

func newCatalogTagsCommand(load catalogLoader) *cobra.Command {
	return &cobra.Command{
		Use:          "tags",
		Short:        "List manifest tags",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := load(cmd)
			if err != nil {
				return err
			}
			return renderOrEmitCatalog(cmd, *report, renderCatalogTags)
		},
	}
}

func newCatalogTagCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "tag NAME",
		Short:        "Show a manifest tag in detail",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := loadCatalogTag(cmd, args[0])
			if err != nil {
				return err
			}
			return renderOrEmitCatalog(cmd, report, renderCatalogTagDetail)
		},
	}
	cmd.ValidArgsFunction = completeCatalogTags
	return cmd
}

func loadCatalogProfile(cmd *cobra.Command, name string) (catalog.Report, error) {
	return loadCatalogReport(cmd, func(m manifest.Manifest, opts catalog.Options) (catalog.Report, error) {
		return catalog.Profile(m, name, opts)
	})
}

func loadCatalogTag(cmd *cobra.Command, name string) (catalog.Report, error) {
	return loadCatalogReport(cmd, func(m manifest.Manifest, opts catalog.Options) (catalog.Report, error) {
		return catalog.Tag(m, name, opts)
	})
}

func loadCatalogComparison(cmd *cobra.Command, from, to string) (catalog.Report, error) {
	return loadCatalogReport(cmd, func(m manifest.Manifest, opts catalog.Options) (catalog.Report, error) {
		return catalog.CompareProfiles(m, from, to, opts)
	})
}

func loadCatalogMap(cmd *cobra.Command, name string) (catalog.Report, error) {
	return loadCatalogReport(cmd, func(m manifest.Manifest, opts catalog.Options) (catalog.Report, error) {
		return catalog.MapProfile(m, name, opts)
	})
}

func loadCatalogWhy(cmd *cobra.Command, profile, query string) (catalog.Report, error) {
	return loadCatalogReport(cmd, func(m manifest.Manifest, opts catalog.Options) (catalog.Report, error) {
		return catalog.ExplainProfileItem(m, profile, query, opts)
	})
}

func loadCatalogReport(cmd *cobra.Command, build func(manifest.Manifest, catalog.Options) (catalog.Report, error)) (catalog.Report, error) {
	file, sourceRoot, osName, err := catalogCommandOptions(cmd)
	if err != nil {
		return catalog.Report{}, err
	}
	m, err := loadCatalogManifest(cmd, file, sourceRoot)
	if err != nil {
		return catalog.Report{}, err
	}
	return build(*m, catalog.Options{OS: osName})
}

func renderOrEmitCatalog(cmd *cobra.Command, report catalog.Report, render func(*cobra.Command, catalog.Report)) error {
	if wantsJSON(cmd) {
		return emitOK(cmd, report)
	}
	render(cmd, report)
	return nil
}

func completeCatalogOS(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return matchingCatalogValues([]string{"darwin", "linux", "all"}, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeCatalogProfiles(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	report, err := loadCatalogCompletionReport(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(report.Profiles))
	for _, profile := range report.Profiles {
		values = append(values, profile.Name)
	}
	return matchingCatalogValues(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeCatalogComparisonProfiles(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	report, err := loadCatalogCompletionReport(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(report.Profiles))
	for _, profile := range report.Profiles {
		if len(args) == 0 || profile.Name != args[0] {
			values = append(values, profile.Name)
		}
	}
	return matchingCatalogValues(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeCatalogTags(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	report, err := loadCatalogCompletionReport(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(report.Tags))
	for _, tag := range report.Tags {
		values = append(values, tag.Name)
	}
	return matchingCatalogValues(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeCatalogWhy(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return completeCatalogProfiles(cmd, args, toComplete)
	}
	if len(args) > 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	report, err := loadCatalogProfile(cmd, args[0])
	if err != nil || report.Profile == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	detail := report.Profile
	values := []string{}
	for _, dependency := range detail.Dependencies {
		values = appendUniqueCatalogValue(values, dependency.Name)
	}
	for _, entry := range detail.Entries {
		values = appendUniqueCatalogValue(values, entry.Target)
		values = appendUniqueCatalogValue(values, entry.Source)
	}
	for _, provisioner := range detail.Provisioners {
		values = appendUniqueCatalogValue(values, provisioner.Tool)
		values = appendUniqueCatalogValue(values, provisioner.Identity)
	}
	return matchingCatalogValues(values, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func appendUniqueCatalogValue(values []string, value string) []string {
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

func loadCatalogCompletionReport(cmd *cobra.Command) (catalog.Report, error) {
	file, sourceRoot, osName, err := catalogCommandOptions(cmd)
	if err != nil {
		return catalog.Report{}, err
	}
	m, err := loadCatalogManifest(cmd, file, sourceRoot)
	if err != nil {
		return catalog.Report{}, err
	}
	return catalog.Build(*m, catalog.Options{OS: osName, IncludeLegacy: true})
}

func catalogCommandOptions(cmd *cobra.Command) (file, sourceRoot, osName string, err error) {
	if file, err = cmd.Flags().GetString("file"); err != nil {
		return "", "", "", err
	}
	if sourceRoot, err = cmd.Flags().GetString("source-root"); err != nil {
		return "", "", "", err
	}
	if osName, err = cmd.Flags().GetString("os"); err != nil {
		return "", "", "", err
	}
	return file, sourceRoot, osName, nil
}

// resolveCatalogSourceRoot keeps Catalog independent from the target home and
// Installation Metadata. An explicit source root is already sufficient; only
// the default Installed Repository convention needs the user's home directory.
func resolveCatalogSourceRoot(sourceRoot string) (string, error) {
	if sourceRoot != "" {
		return sourceRoot, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return defaultSourceRoot(home), nil
}

func loadCatalogManifest(cmd *cobra.Command, file, sourceRoot string) (*manifest.Manifest, error) {
	if cmd.Flags().Changed("file") {
		return loadManifestForCommand(cmd, file, "")
	}
	resolvedSourceRoot, err := resolveCatalogSourceRoot(sourceRoot)
	if err != nil {
		return nil, err
	}
	return loadManifestForCommand(cmd, file, resolvedSourceRoot)
}

func matchingCatalogValues(values []string, toComplete string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, toComplete) {
			result = append(result, value)
		}
	}
	return result
}
