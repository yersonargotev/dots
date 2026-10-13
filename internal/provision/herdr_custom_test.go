package provision_test

import (
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
	"reflect"
	"strings"
	"testing"
)

func TestCustomHerdrUsesOwnExecutableAndOriginalPluginArguments(t *testing.T) {
	p := herdrProvisioner("yersonargotev/herdr-tab-git", herdrGitCommit)
	p.Tool = "herdr-custom"
	executable, args := provision.RenderCommand(p)
	if executable != "herdr-custom" || !reflect.DeepEqual(args, []string{"plugin", "install", "yersonargotev/herdr-tab-git", "--ref", herdrGitCommit, "--yes"}) {
		t.Fatalf("%q %v", executable, args)
	}
	m := manifestWithProvisioners(p)
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	plan, err := provision.Build(m, provision.Options{Profile: "default", OS: "darwin", Arch: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || len(plan.Steps[0].Targets) == 0 || plan.Steps[0].Targets[0] != "~/.config/herdr" {
		t.Fatalf("%+v", plan)
	}
	m.Tags = map[string]manifest.Tag{"core": {Kind: "surface", Status: "current", SupportedPlatforms: []string{"darwin_arm64"}}}
	if _, err := provision.Build(m, provision.Options{Selection: &manifest.Selection{Tags: []string{"core"}}, OS: "linux", Arch: "amd64"}); err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("platform constraint: %v", err)
	}
	p.Spec.Plugin = "--evil/repo"
	m = manifestWithProvisioners(p)
	if err := m.Validate(); err == nil {
		t.Fatal("custom provisioner bypassed plugin validation")
	}
}
