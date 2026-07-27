package provision_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
)

// fakeRunner records every Run invocation and can be configured to fail on a
// specific 1-based call index so tests can assert the run stops after a failure.
type fakeRunner struct {
	calls   [][]string
	failOn  int
	failErr error
}

func (r *fakeRunner) Run(executable string, args []string) error {
	r.calls = append(r.calls, append([]string{executable}, args...))
	if r.failOn != 0 && len(r.calls) == r.failOn {
		return r.failErr
	}
	return nil
}

func lookupWith(present ...string) deps.Lookup {
	set := make(map[string]bool, len(present))
	for _, p := range present {
		set[p] = true
	}
	return func(command string) bool { return set[command] }
}

func fontLookupWith(present ...string) deps.FontLookup {
	set := make(map[string]bool, len(present))
	for _, p := range present {
		set[p] = true
	}
	return func(match string) bool { return set[match] }
}

func appLookupWith(present ...string) deps.AppLookup {
	set := make(map[string]bool, len(present))
	for _, p := range present {
		set[p] = true
	}
	return func(app string) bool { return set[app] }
}

func gentleAIProvisioner(agent string) manifest.Provisioner {
	return manifest.Provisioner{
		Tool: "gentle-ai",
		Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Scope: "global", Agents: []string{agent}},
		Dependencies: []manifest.Dependency{
			{Name: "gentle-ai"},
			{Name: "engram"},
		},
	}
}

func TestApplyRunsSelectedProvisionersInOrder(t *testing.T) {
	m := manifestWithProvisioners(gentleAIProvisioner("codex"), gentleAIProvisioner("claude"))
	runner := &fakeRunner{}
	look := lookupWith("gentle-ai", "engram")

	report, err := provision.Apply(m, provision.Options{Profile: "default", OS: "darwin"}, look, fontLookupWith(), runner)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	wantCalls := [][]string{
		{"gentle-ai", "install", "--scope", "global", "--agents", "codex"},
		{"gentle-ai", "install", "--scope", "global", "--agents", "claude"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner.calls = %#v, want %#v", runner.calls, wantCalls)
	}
	if len(report.Items) != 2 {
		t.Fatalf("len(report.Items) = %d, want 2", len(report.Items))
	}
	for i, item := range report.Items {
		if item.Status != provision.RunStatusProvisioned {
			t.Fatalf("report.Items[%d].Status = %q, want provisioned", i, item.Status)
		}
	}
}

func TestApplyFailsWithoutRunningWhenDependencyMissing(t *testing.T) {
	m := manifestWithProvisioners(gentleAIProvisioner("codex"))
	runner := &fakeRunner{}
	look := lookupWith("gentle-ai") // engram missing

	report, err := provision.Apply(m, provision.Options{Profile: "default", OS: "darwin"}, look, fontLookupWith(), runner)
	if err == nil {
		t.Fatal("Apply() error = nil, want missing-dependency error")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner.calls = %#v, want no invocation when a dependency is missing", runner.calls)
	}
	if len(report.Items) != 1 {
		t.Fatalf("len(report.Items) = %d, want 1", len(report.Items))
	}
	item := report.Items[0]
	if item.Status != provision.RunStatusMissingDeps {
		t.Fatalf("report.Items[0].Status = %q, want missing-dependencies", item.Status)
	}
	if !reflect.DeepEqual(item.Missing, []string{"engram"}) {
		t.Fatalf("report.Items[0].Missing = %#v, want [engram]", item.Missing)
	}
}

func TestApplyStopsAndReportsOnRunnerFailure(t *testing.T) {
	m := manifestWithProvisioners(gentleAIProvisioner("codex"), gentleAIProvisioner("claude"))
	runErr := errors.New("gentle-ai exploded")
	runner := &fakeRunner{failOn: 1, failErr: runErr}
	look := lookupWith("gentle-ai", "engram")

	report, err := provision.Apply(m, provision.Options{Profile: "default", OS: "darwin"}, look, fontLookupWith(), runner)
	if err == nil {
		t.Fatal("Apply() error = nil, want runner failure error")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner.calls count = %d, want 1 (run must stop after the failure)", len(runner.calls))
	}
	if len(report.Items) != 1 {
		t.Fatalf("len(report.Items) = %d, want 1", len(report.Items))
	}
	if report.Items[0].Status != provision.RunStatusFailed {
		t.Fatalf("report.Items[0].Status = %q, want failed", report.Items[0].Status)
	}
}

func TestApplyNoProvisionersIsNoOp(t *testing.T) {
	m := manifestWithProvisioners()
	runner := &fakeRunner{}

	report, err := provision.Apply(m, provision.Options{Profile: "default", OS: "darwin"}, lookupWith(), fontLookupWith(), runner)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner.calls = %#v, want none", runner.calls)
	}
	if len(report.Items) != 0 {
		t.Fatalf("len(report.Items) = %d, want 0", len(report.Items))
	}
}

func TestApplyUnknownProfileIsError(t *testing.T) {
	m := manifestWithProvisioners(gentleAIProvisioner("codex"))
	if _, err := provision.Apply(m, provision.Options{Profile: "ghost", OS: "darwin"}, lookupWith(), fontLookupWith(), &fakeRunner{}); err == nil {
		t.Fatal("Apply() error = nil, want unknown-profile error")
	}
}

func TestApplyAcceptsProvisionerFontFallbackDependency(t *testing.T) {
	prov := gentleAIProvisioner("codex")
	prov.Dependencies = append(prov.Dependencies, manifest.Dependency{
		Name: "Desktop Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*", FontFallbackMatches: []string{"CaskaydiaCoveNerdFont*"},
	})
	m := manifestWithProvisioners(prov)
	runner := &fakeRunner{}

	report, err := provision.Apply(m, provision.Options{Profile: "default", OS: "darwin"}, lookupWith("gentle-ai", "engram"), fontLookupWith("CaskaydiaCoveNerdFont*"), runner)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(report.Items) != 1 || report.Items[0].Status != provision.RunStatusProvisioned {
		t.Fatalf("report.Items = %#v, want provisioned item when fallback font is installed", report.Items)
	}
}

func TestApplyUsesFontLookupForProvisionerFontDependencies(t *testing.T) {
	prov := gentleAIProvisioner("codex")
	prov.Dependencies = append(prov.Dependencies, manifest.Dependency{
		Name: "CascadiaCode Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*",
	})
	m := manifestWithProvisioners(prov)
	runner := &fakeRunner{}
	var commandProbes []string

	report, err := provision.Apply(m, provision.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
		commandProbes = append(commandProbes, command)
		return command == "gentle-ai" || command == "engram"
	}, fontLookupWith("CascadiaCodeNF*"), runner)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if !reflect.DeepEqual(commandProbes, []string{"gentle-ai", "engram"}) {
		t.Fatalf("command probes = %#v, want only command dependencies", commandProbes)
	}
	if len(report.Items) != 1 || report.Items[0].Status != provision.RunStatusProvisioned {
		t.Fatalf("report.Items = %#v, want provisioned item", report.Items)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner.calls = %#v, want one provisioner invocation", runner.calls)
	}
}
