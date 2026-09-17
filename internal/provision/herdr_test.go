package provision_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
)

const (
	herdrMetadataCommit = "c696c36256eddc6ee1983ab9f202848b84460e06"
	herdrGitCommit      = "83ce41a11c5cc3ab2de1452ab303f6dfb976a937"
	herdrTabbyCommit    = "34c01f9791dd3228acae7ca378adb38e09d9fb6c"
)

func herdrProvisioner(plugin, ref string) manifest.Provisioner {
	return manifest.Provisioner{
		Tool: "herdr", Tags: []string{"core"}, OS: []string{"darwin"},
		Spec: manifest.ProvisionerSpec{Plugin: plugin, Ref: ref},
		Dependencies: []manifest.Dependency{
			{Name: "herdr"},
			{Name: "git"},
			{Name: "python3"},
			{Name: "node"},
		},
	}
}

func TestRenderHerdrPluginInstallCommand(t *testing.T) {
	gotExec, gotArgs := provision.RenderCommand(manifest.Provisioner{
		Tool: "herdr",
		Spec: manifest.ProvisionerSpec{
			Plugin: "  szrenwei/herdr-space-tab-metadata  ",
			Ref:    "  " + herdrMetadataCommit + "  ",
		},
	})

	if gotExec != "herdr" {
		t.Fatalf("RenderCommand() executable = %q, want herdr", gotExec)
	}
	wantArgs := []string{"plugin", "install", "szrenwei/herdr-space-tab-metadata", "--ref", herdrMetadataCommit, "--yes"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("RenderCommand() args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestHerdrPlanSelectsThreeDarwinPluginsAndNoLinuxPlugins(t *testing.T) {
	plugins := []manifest.Provisioner{
		herdrProvisioner("szrenwei/herdr-space-tab-metadata", herdrMetadataCommit),
		herdrProvisioner("hasuwini77/herdr-tab-git", herdrGitCommit),
		herdrProvisioner("yersonargotev/tabby", herdrTabbyCommit),
	}
	m := manifestWithProvisioners(plugins...)

	darwin, err := provision.Build(m, provision.Options{Profile: "default", OS: "darwin"})
	if err != nil {
		t.Fatalf("Build(darwin) error = %v", err)
	}
	if len(darwin.Steps) != 3 {
		t.Fatalf("len(Build(darwin).Steps) = %d, want 3", len(darwin.Steps))
	}
	for i, step := range darwin.Steps {
		if step.Tool != "herdr" || step.Executable != "herdr" {
			t.Fatalf("darwin.Steps[%d] tool/executable = %q/%q, want herdr/herdr", i, step.Tool, step.Executable)
		}
		if !reflect.DeepEqual(step.Targets, []string{"~/.config/herdr/plugins"}) {
			t.Fatalf("darwin.Steps[%d].Targets = %#v", i, step.Targets)
		}
	}

	linux, err := provision.Build(m, provision.Options{Profile: "default", OS: "linux"})
	if err != nil {
		t.Fatalf("Build(linux) error = %v", err)
	}
	if len(linux.Steps) != 0 {
		t.Fatalf("Build(linux).Steps = %#v, want none", linux.Steps)
	}
}

func TestHerdrReadinessReportsDependenciesWithoutRunning(t *testing.T) {
	m := manifestWithProvisioners(herdrProvisioner("hasuwini77/herdr-tab-git", herdrGitCommit))

	ready, err := provision.Check(m, provision.Options{Profile: "default", OS: "darwin"}, lookupWith("herdr", "git", "python3", "node"), fontLookupWith())
	if err != nil {
		t.Fatalf("Check(ready) error = %v", err)
	}
	if len(ready.Items) != 1 || len(ready.Items[0].Missing) != 0 {
		t.Fatalf("Check(ready).Items = %#v", ready.Items)
	}

	missing, err := provision.Check(m, provision.Options{Profile: "default", OS: "darwin"}, lookupWith("herdr", "git"), fontLookupWith())
	if err != nil {
		t.Fatalf("Check(missing) error = %v", err)
	}
	if !reflect.DeepEqual(missing.Items[0].Missing, []string{"python3", "node"}) {
		t.Fatalf("Check(missing).Missing = %#v, want [python3 node]", missing.Items[0].Missing)
	}
}

func TestHerdrApplyUsesOneCommandPerEntryAndPropagatesFailure(t *testing.T) {
	first := herdrProvisioner("szrenwei/herdr-space-tab-metadata", herdrMetadataCommit)
	second := herdrProvisioner("hasuwini77/herdr-tab-git", herdrGitCommit)
	m := manifestWithProvisioners(first, second)
	look := lookupWith("herdr", "git", "python3", "node")
	runErr := errors.New("herdr install failed")
	runner := &fakeRunner{failOn: 2, failErr: runErr}

	report, err := provision.Apply(m, provision.Options{Profile: "default", OS: "darwin"}, look, fontLookupWith(), runner)
	if !errors.Is(err, runErr) {
		t.Fatalf("Apply() error = %v, want wrapping runner error", err)
	}
	wantCalls := [][]string{
		{"herdr", "plugin", "install", "szrenwei/herdr-space-tab-metadata", "--ref", herdrMetadataCommit, "--yes"},
		{"herdr", "plugin", "install", "hasuwini77/herdr-tab-git", "--ref", herdrGitCommit, "--yes"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("runner.calls = %#v, want %#v", runner.calls, wantCalls)
	}
	if len(report.Items) != 2 || report.Items[0].Status != provision.RunStatusProvisioned || report.Items[1].Status != provision.RunStatusFailed {
		t.Fatalf("Apply() report = %#v", report)
	}
}

func TestHerdrApplyRepeatsPinnedInstallForRefresh(t *testing.T) {
	m := manifestWithProvisioners(herdrProvisioner("yersonargotev/tabby", herdrTabbyCommit))
	runner := &fakeRunner{}
	look := lookupWith("herdr", "git", "python3", "node")

	for run := 0; run < 2; run++ {
		if _, err := provision.Apply(m, provision.Options{Profile: "default", OS: "darwin"}, look, fontLookupWith(), runner); err != nil {
			t.Fatalf("Apply() run %d error = %v", run+1, err)
		}
	}
	want := []string{"herdr", "plugin", "install", "yersonargotev/tabby", "--ref", herdrTabbyCommit, "--yes"}
	if len(runner.calls) != 2 || !reflect.DeepEqual(runner.calls[0], want) || !reflect.DeepEqual(runner.calls[1], want) {
		t.Fatalf("repeated runner.calls = %#v, want two identical refresh invocations", runner.calls)
	}
}
