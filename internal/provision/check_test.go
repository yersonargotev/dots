package provision_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
)

func TestCheckReportsReadiness(t *testing.T) {
	m := manifestWithProvisioners(marketplaceProvisioner("example/tools"))

	t.Run("all dependencies present", func(t *testing.T) {
		report, err := provision.Check(m, provision.Options{Profile: "default", OS: "darwin"}, lookupWith("claude", "npx"), fontLookupWith())
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if report.Profile != "default" {
			t.Fatalf("Check.Profile = %q, want default", report.Profile)
		}
		if len(report.Items) != 1 {
			t.Fatalf("len(Check.Items) = %d, want 1", len(report.Items))
		}
		item := report.Items[0]
		if item.Tool != "claude" || item.Executable != "claude" {
			t.Fatalf("readiness tool/executable = %q/%q, want claude", item.Tool, item.Executable)
		}
		wantArgs := []string{"plugin", "marketplace", "add", "example/tools"}
		if !reflect.DeepEqual(item.Args, wantArgs) {
			t.Fatalf("readiness Args = %#v, want %#v", item.Args, wantArgs)
		}
		if len(item.Missing) != 0 {
			t.Fatalf("readiness Missing = %#v, want none", item.Missing)
		}
	})

	t.Run("missing dependency is reported without executing", func(t *testing.T) {
		report, err := provision.Check(m, provision.Options{Profile: "default", OS: "darwin"}, lookupWith("claude"), fontLookupWith())
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if !reflect.DeepEqual(report.Items[0].Missing, []string{"npx"}) {
			t.Fatalf("readiness Missing = %#v, want [npx]", report.Items[0].Missing)
		}
	})

	t.Run("unknown profile is an error", func(t *testing.T) {
		if _, err := provision.Check(m, provision.Options{Profile: "ghost", OS: "darwin"}, lookupWith(), fontLookupWith()); err == nil {
			t.Fatal("Check() error = nil, want unknown-profile error")
		}
	})
}

func TestCheckAcceptsProvisionerFontFallbackDependency(t *testing.T) {
	prov := marketplaceProvisioner("example/tools")
	prov.Dependencies = append(prov.Dependencies, manifest.Dependency{
		Name: "Desktop Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*", FontFallbackMatches: []string{"CaskaydiaCoveNerdFont*"},
	})
	m := manifestWithProvisioners(prov)

	report, err := provision.Check(m, provision.Options{Profile: "default", OS: "darwin"}, lookupWith("claude", "npx"), fontLookupWith("CaskaydiaCoveNerdFont*"))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(report.Items) != 1 || len(report.Items[0].Missing) != 0 {
		t.Fatalf("readiness = %#v, want no missing deps when fallback font is installed", report.Items)
	}
}

func TestCheckAcceptsProvisionerDarwinAppDependency(t *testing.T) {
	prov := marketplaceProvisioner("example/tools")
	prov.Dependencies = append(prov.Dependencies, manifest.Dependency{
		Name: "ghostty", Command: "ghostty", DarwinApp: " Ghostty.app ",
	})
	m := manifestWithProvisioners(prov)

	report, err := provision.Check(m, provision.Options{
		Profile: "default", OS: "darwin", AppLookup: appLookupWith("Ghostty.app"),
	}, lookupWith("claude", "npx"), fontLookupWith())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(report.Items) != 1 || len(report.Items[0].Missing) != 0 {
		t.Fatalf("readiness = %#v, want no missing deps when Darwin app is installed", report.Items)
	}
}

func TestCheckUsesFontLookupForProvisionerFontDependencies(t *testing.T) {
	prov := marketplaceProvisioner("example/tools")
	prov.Dependencies = append(prov.Dependencies, manifest.Dependency{
		Name: "CascadiaCode Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*",
	})
	m := manifestWithProvisioners(prov)
	var commandProbes []string

	report, err := provision.Check(m, provision.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
		commandProbes = append(commandProbes, command)
		return command == "claude" || command == "npx"
	}, fontLookupWith("CascadiaCodeNF*"))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !reflect.DeepEqual(commandProbes, []string{"claude", "npx"}) {
		t.Fatalf("command probes = %#v, want only command dependencies", commandProbes)
	}
	if len(report.Items) != 1 {
		t.Fatalf("len(Check.Items) = %d, want 1", len(report.Items))
	}
	if len(report.Items[0].Missing) != 0 {
		t.Fatalf("readiness Missing = %#v, want none when font is installed", report.Items[0].Missing)
	}
}
