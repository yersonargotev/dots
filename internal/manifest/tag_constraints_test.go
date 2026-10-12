package manifest_test

import (
	"github.com/yersonargotev/dots/internal/manifest"
	"strings"
	"testing"
)

func TestVariantConstraintsApplyToProfilesAndExplicitTags(t *testing.T) {
	m := manifest.Manifest{Tags: map[string]manifest.Tag{
		"official": {Kind: "surface", Status: "current"},
		"custom":   {Kind: "surface", Status: "current", ConflictsWith: []string{"official"}, SupportedPlatforms: []string{"darwin_arm64", "linux_amd64"}},
	}, Profiles: map[string]manifest.Profile{"core": {Tags: []string{"official"}}}}
	for _, tags := range [][]string{{"official", "custom"}, {"custom", "official"}} {
		if _, err := manifest.ResolveReadOnlySelection(m, nil, tags); err == nil || !strings.Contains(err.Error(), "cannot be selected together") {
			t.Fatalf("tags %v: %v", tags, err)
		}
	}
	if _, err := manifest.ResolveSelection(m, []string{"core"}, []string{"custom"}); err == nil {
		t.Fatal("profile conflict accepted")
	}
	for _, platform := range []struct {
		os, arch string
		want     bool
	}{{"darwin", "arm64", true}, {"linux", "amd64", true}, {"darwin", "amd64", false}, {"linux", "arm64", false}} {
		err := manifest.ValidateSelection(m, manifest.Selection{Tags: []string{"custom"}}, platform.os, platform.arch)
		if (err == nil) != platform.want {
			t.Errorf("%s/%s: %v", platform.os, platform.arch, err)
		}
	}
	// Pre-resolved callers must not bypass mutual exclusion.
	if err := manifest.ValidateSelection(m, manifest.Selection{Tags: []string{"official", "custom"}}, "darwin", "arm64"); err == nil {
		t.Fatal("pre-resolved conflict accepted")
	}
	if err := manifest.ValidateSelection(m, manifest.Selection{Tags: []string{"official"}}, "linux", "arm64"); err != nil {
		t.Fatal(err)
	}
}

func TestTagConstraintDeclarationValidation(t *testing.T) {
	for _, bad := range []manifest.Tag{
		{ConflictsWith: []string{"missing"}}, {ConflictsWith: []string{"herdr"}},
		{SupportedPlatforms: []string{"windows_amd64"}}, {SupportedPlatforms: []string{"linux_amd64", "linux_amd64"}},
	} {
		m := manifestWithHerdrSpec(manifest.ProvisionerSpec{Plugin: "owner/repo", Ref: herdrTestCommit})
		bad.Kind = "surface"
		bad.Status = "current"
		m.Tags = map[string]manifest.Tag{"herdr": bad}
		if err := m.Validate(); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
