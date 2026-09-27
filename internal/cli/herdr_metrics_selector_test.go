package cli

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
	tagselectortui "github.com/yersonargotev/dots/internal/tui/tagselector"
)

func TestRepositoryTagSelectorOffersHerdrMetricsAsGlobalOptIn(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	got, err := buildInstallTagSelectorBrowseData(*m, state.Metadata{}, tagSelectorBrowseOptions{
		OS: "darwin", Arch: "arm64", SourceReadRoot: root, Home: home, StateRoot: t.TempDir(),
		XDGStateHome: filepath.Join(home, ".local", "state"),
		Lookup:       func(string) bool { return false },
		FontLookup:   func(string) bool { return false },
		AppLookup:    func(string) bool { return false },
	})
	if err != nil {
		t.Fatalf("buildInstallTagSelectorBrowseData() error = %v", err)
	}

	var metrics *tagselectortui.Tag
	for i := range got.Tags {
		if got.Tags[i].Name == "herdr-metrics" {
			metrics = &got.Tags[i]
		}
	}
	if metrics == nil {
		t.Fatal("Tag selector omitted herdr-metrics")
	}
	if metrics.Group != "Global" || len(metrics.Profiles) != 0 {
		t.Fatalf("herdr-metrics group %q, Profiles %v; want Global opt-in outside every Profile", metrics.Group, metrics.Profiles)
	}
	if !slices.Contains(metrics.ManagedEntries, "~/.config/herdr/status.sh") || !slices.Contains(metrics.Dependencies, "macmon") {
		t.Fatalf("herdr-metrics entries %v, dependencies %v; want status script and macmon", metrics.ManagedEntries, metrics.Dependencies)
	}
}
