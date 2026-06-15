package provision_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
)

func TestCheckReportsReadiness(t *testing.T) {
	m := manifestWithProvisioners(gentleAIProvisioner("codex"))

	t.Run("all dependencies present", func(t *testing.T) {
		report, err := provision.Check(m, provision.Options{Profile: "default", OS: "darwin"}, lookupWith("gentle-ai", "engram"), fontLookupWith())
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
		if item.Tool != "gentle-ai" || item.Executable != "gentle-ai" {
			t.Fatalf("readiness tool/executable = %q/%q, want gentle-ai", item.Tool, item.Executable)
		}
		wantArgs := []string{"install", "--scope", "global", "--agents", "codex"}
		if !reflect.DeepEqual(item.Args, wantArgs) {
			t.Fatalf("readiness Args = %#v, want %#v", item.Args, wantArgs)
		}
		if len(item.Missing) != 0 {
			t.Fatalf("readiness Missing = %#v, want none", item.Missing)
		}
	})

	t.Run("missing dependency is reported without executing", func(t *testing.T) {
		report, err := provision.Check(m, provision.Options{Profile: "default", OS: "darwin"}, lookupWith("gentle-ai"), fontLookupWith())
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if !reflect.DeepEqual(report.Items[0].Missing, []string{"engram"}) {
			t.Fatalf("readiness Missing = %#v, want [engram]", report.Items[0].Missing)
		}
	})

	t.Run("unknown profile is an error", func(t *testing.T) {
		if _, err := provision.Check(m, provision.Options{Profile: "ghost", OS: "darwin"}, lookupWith(), fontLookupWith()); err == nil {
			t.Fatal("Check() error = nil, want unknown-profile error")
		}
	})
}

func TestCheckUsesFontLookupForProvisionerFontDependencies(t *testing.T) {
	prov := gentleAIProvisioner("codex")
	prov.Dependencies = append(prov.Dependencies, manifest.Dependency{
		Name: "CascadiaCode Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*",
	})
	m := manifestWithProvisioners(prov)
	var commandProbes []string

	report, err := provision.Check(m, provision.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
		commandProbes = append(commandProbes, command)
		return command == "gentle-ai" || command == "engram"
	}, fontLookupWith("CascadiaCodeNF*"))
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !reflect.DeepEqual(commandProbes, []string{"gentle-ai", "engram"}) {
		t.Fatalf("command probes = %#v, want only command dependencies", commandProbes)
	}
	if len(report.Items) != 1 {
		t.Fatalf("len(Check.Items) = %d, want 1", len(report.Items))
	}
	if len(report.Items[0].Missing) != 0 {
		t.Fatalf("readiness Missing = %#v, want none when font is installed", report.Items[0].Missing)
	}
}
