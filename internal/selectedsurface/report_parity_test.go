package selectedsurface_test

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/yersonargotev/dots/internal/catalog"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/doctor"
	"github.com/yersonargotev/dots/internal/installed"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selectedsurface"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
)

func TestRepositoryReportsAgreeWithSelectedSurface(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.LoadFile(filepath.Join(repositoryRoot, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	profileNames := make([]string, 0, len(m.Profiles))
	for name := range m.Profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)

	for _, profileName := range profileNames {
		profile := m.Profiles[profileName]
		profileSelection, err := manifest.ResolveReadOnlySelection(*m, []string{profileName}, nil)
		if err != nil {
			t.Fatalf("resolve Profile %q: %v", profileName, err)
		}
		tagSelection, err := manifest.ResolveReadOnlySelection(*m, nil, profile.Tags)
		if err != nil {
			t.Fatalf("resolve Tags for Profile %q: %v", profileName, err)
		}

		for _, osName := range []string{"darwin", "linux"} {
			t.Run(profileName+"/"+osName, func(t *testing.T) {
				home := t.TempDir()
				xdgStateHome := filepath.Join(home, ".local", "state")
				profileSurface := selectedsurface.Evaluate(*m, profileSelection.Tags, osName)
				tagSurface := selectedsurface.Evaluate(*m, tagSelection.Tags, osName)
				if !reflect.DeepEqual(profileSurface, tagSurface) {
					t.Fatal("Profile and equivalent explicit Tags produced different Selected Surfaces")
				}

				profileDeps, err := deps.Check(*m, deps.Options{Selection: &profileSelection, OS: osName}, absent, absent)
				if err != nil {
					t.Fatal(err)
				}
				tagDeps, err := deps.Check(*m, deps.Options{Selection: &tagSelection, OS: osName}, absent, absent)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(profileDeps.Results, tagDeps.Results) || len(profileDeps.Results) != len(profileSurface.Dependencies) {
					t.Fatalf("Dependency report disagrees with Selected Surface: results=%d dependencies=%d", len(profileDeps.Results), len(profileSurface.Dependencies))
				}

				profilePlan := buildPlan(t, *m, profileSelection, osName, repositoryRoot, home, xdgStateHome)
				tagPlan := buildPlan(t, *m, tagSelection, osName, repositoryRoot, home, xdgStateHome)
				if !reflect.DeepEqual(profilePlan.Actions, tagPlan.Actions) {
					t.Fatal("Install Plan differs for Profile and equivalent explicit Tags")
				}

				profileProvisioners, err := provision.Build(*m, provision.Options{Selection: &profileSelection, OS: osName})
				if err != nil {
					t.Fatal(err)
				}
				tagProvisioners, err := provision.Build(*m, provision.Options{Selection: &tagSelection, OS: osName})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(profileProvisioners.Steps, tagProvisioners.Steps) || len(profileProvisioners.Steps) != len(profileSurface.Provisioners) {
					t.Fatalf("Provisioner Plan disagrees with Selected Surface: steps=%d provisioners=%d", len(profileProvisioners.Steps), len(profileSurface.Provisioners))
				}

				profileStatus := buildStatus(t, *m, profileSelection, osName, repositoryRoot, home, xdgStateHome)
				tagStatus := buildStatus(t, *m, tagSelection, osName, repositoryRoot, home, xdgStateHome)
				if !reflect.DeepEqual(profileStatus.Entries, tagStatus.Entries) {
					t.Fatal("status differs for Profile and equivalent explicit Tags")
				}

				profileDoctor := buildDoctor(t, *m, profileSelection, osName, repositoryRoot, home, xdgStateHome)
				tagDoctor := buildDoctor(t, *m, tagSelection, osName, repositoryRoot, home, xdgStateHome)
				if !reflect.DeepEqual(profileDoctor.Dependencies.Results, tagDoctor.Dependencies.Results) ||
					!reflect.DeepEqual(profileDoctor.Configuration.Entries, tagDoctor.Configuration.Entries) ||
					!reflect.DeepEqual(profileDoctor.Provisioners.Items, tagDoctor.Provisioners.Items) ||
					!reflect.DeepEqual(profileDoctor.SecretScan, tagDoctor.SecretScan) {
					t.Fatal("doctor differs for Profile and equivalent explicit Tags")
				}

				assertCatalogSurface(t, *m, profileName, osName, profileSurface)
				assertInstalledCoverage(t, *m, profileName, profile.Tags, osName, repositoryRoot, home, xdgStateHome, profileSurface)
			})
		}
	}
}

func absent(string) bool { return false }

func buildPlan(t *testing.T, m manifest.Manifest, selected manifest.Selection, osName, sourceRoot, home, xdgStateHome string) plan.Plan {
	t.Helper()
	result, err := plan.Build(m, plan.Options{Selection: &selected, OS: osName, SourceRoot: sourceRoot, Home: home, XDGStateHome: xdgStateHome})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func buildStatus(t *testing.T, m manifest.Manifest, selected manifest.Selection, osName, sourceRoot, home, xdgStateHome string) status.Report {
	t.Helper()
	result, err := status.Build(m, state.Metadata{}, status.Options{Selection: &selected, OS: osName, SourceRoot: sourceRoot, Home: home, XDGStateHome: xdgStateHome})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func buildDoctor(t *testing.T, m manifest.Manifest, selected manifest.Selection, osName, sourceRoot, home, xdgStateHome string) doctor.Report {
	t.Helper()
	result, err := doctor.Build(m, state.Metadata{}, doctor.Options{Selection: &selected, OS: osName, SourceRoot: sourceRoot, Home: home, XDGStateHome: xdgStateHome}, absent, absent)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertCatalogSurface(t *testing.T, m manifest.Manifest, profileName, osName string, surface selectedsurface.Surface) {
	t.Helper()
	report, err := catalog.Profile(m, profileName, catalog.Options{OS: osName})
	if err != nil {
		t.Fatal(err)
	}
	if report.Profile == nil {
		t.Fatal("catalog Profile detail is nil")
	}
	applicableOverrides := 0
	for _, override := range report.Profile.SourceOverrides {
		if override.Applicable {
			applicableOverrides++
		}
	}
	if len(report.Profile.Entries) != len(surface.Entries) ||
		len(report.Profile.Dependencies) != len(surface.DependencyOrigins) ||
		len(report.Profile.DependencySets) != len(surface.DependencySets) ||
		len(report.Profile.Provisioners) != len(surface.Provisioners) ||
		applicableOverrides != len(surface.SourceOverrides) {
		t.Fatalf("Install Catalog disagrees with Selected Surface: detail=%+v surface=%+v", report.Profile, surface)
	}
}

func assertInstalledCoverage(t *testing.T, m manifest.Manifest, profileName string, tags []string, osName, sourceRoot, home, xdgStateHome string, surface selectedsurface.Surface) {
	t.Helper()
	meta := state.Metadata{Entries: []state.Record{{Source: "not-in-manifest", Target: "~/.not-in-manifest", Strategy: "copy", Profiles: []string{profileName}, Tags: append([]string(nil), tags...)}}}
	report, err := installed.Build(m, meta, installed.Options{SourceRoot: sourceRoot, Home: home, XDGStateHome: xdgStateHome, OS: osName})
	if err != nil {
		t.Fatal(err)
	}
	for _, coverage := range report.Profiles {
		if coverage.Name != profileName {
			continue
		}
		if coverage.TotalEntries != len(surface.Entries) || coverage.TotalProvisioners != len(surface.Provisioners) {
			t.Fatalf("installed coverage disagrees with Selected Surface: coverage=%+v entries=%d provisioners=%d", coverage, len(surface.Entries), len(surface.Provisioners))
		}
		return
	}
	t.Fatalf("installed report omitted Profile %q coverage", profileName)
}
