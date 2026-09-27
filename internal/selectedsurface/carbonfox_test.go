package selectedsurface_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestCarbonfoxPrecedencePreservesUnrelatedOverrides(t *testing.T) {
	m := manifest.Manifest{Entries: []manifest.Entry{{
		Source: "base", Target: "~/.config/app", Tags: []string{"app"},
		SourceOverrides: map[string]string{"adaptive-theme": "adaptive", "theme-carbonfox": "carbonfox", "other": "other"},
	}}}
	for _, tt := range []struct {
		tags []string
		want string
	}{
		{[]string{"app"}, "base"},
		{[]string{"app", "adaptive-theme"}, "adaptive"},
		{[]string{"app", "theme-carbonfox", "adaptive-theme"}, "carbonfox"},
		{[]string{"app", "adaptive-theme", "theme-carbonfox"}, "carbonfox"},
		{[]string{"app", "theme-carbonfox", "other", "adaptive-theme"}, "other"},
		{[]string{"app", "other", "theme-carbonfox", "adaptive-theme"}, "carbonfox"},
		{[]string{"app", "adaptive-theme", "other"}, "other"},
		{[]string{"app", "other", "adaptive-theme"}, "adaptive"},
	} {
		t.Run(tt.want+":"+joinTags(tt.tags), func(t *testing.T) {
			surface := selectedsurface.Evaluate(m, tt.tags, "linux")
			if len(surface.Entries) != 1 || surface.Entries[0].Source != tt.want {
				t.Fatalf("entries = %#v, want one %s source", surface.Entries, tt.want)
			}
			diagnostic := selectedsurface.EvaluateEntries(m, tt.tags, "linux")
			if diagnostic[0].Source != tt.want {
				t.Fatalf("diagnostic source = %s, want %s", diagnostic[0].Source, tt.want)
			}
		})
	}
	delete(m.Entries[0].SourceOverrides, "theme-carbonfox")
	if got := selectedsurface.Evaluate(m, []string{"app", "theme-carbonfox", "adaptive-theme"}, "linux").Entries[0].Source; got != "adaptive" {
		t.Fatalf("unrelated entry lost adaptive override: %s", got)
	}
}

func joinTags(tags []string) string {
	result := ""
	for _, tag := range tags {
		result += "/" + tag
	}
	return result
}

func TestRequiredProvisionerTagsComposeWithoutAddingDependencies(t *testing.T) {
	m := manifest.Manifest{Provisioners: []manifest.Provisioner{{
		Tool: "bat", Tags: []string{"bat", "zsh"}, RequiredTags: []string{"theme-carbonfox"},
		Dependencies: []manifest.Dependency{{Name: "bat"}},
	}}}
	for _, tt := range []struct {
		tags []string
		want int
	}{
		{nil, 0}, {[]string{"bat"}, 0}, {[]string{"zsh"}, 0},
		{[]string{"theme-carbonfox"}, 0},
		{[]string{"bat", "theme-carbonfox"}, 1},
		{[]string{"theme-carbonfox", "zsh"}, 1},
		{[]string{"bat", "zsh", "theme-carbonfox"}, 1},
	} {
		surface := selectedsurface.Evaluate(m, tt.tags, "linux")
		if len(surface.Provisioners) != tt.want || len(surface.Dependencies) != tt.want {
			t.Fatalf("tags %v: provisioners/dependencies = %d/%d, want %d", tt.tags, len(surface.Provisioners), len(surface.Dependencies), tt.want)
		}
	}
	surface := selectedsurface.Evaluate(m, []string{"bat", "theme-carbonfox"}, "linux")
	surface.Provisioners[0].RequiredTags[0] = "mutated"
	if !reflect.DeepEqual(m.Provisioners[0].RequiredTags, []string{"theme-carbonfox"}) {
		t.Fatal("returned required tags alias manifest")
	}
	m.Provisioners[0].RequiredTags = []string{"theme-carbonfox", "second"}
	if len(selectedsurface.Evaluate(m, []string{"bat", "theme-carbonfox"}, "linux").Provisioners) != 0 {
		t.Fatal("required tags were treated as alternatives")
	}
	m.Provisioners[0].RequiredTags = nil
	if len(selectedsurface.Evaluate(m, []string{"bat"}, "linux").Provisioners) != 1 {
		t.Fatal("omitted required tags changed prior selection behavior")
	}
}
