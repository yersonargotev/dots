package provision_test

import (
	"errors"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
	"reflect"
	"testing"
)

func TestRetiredHerdrProvisionerOnlyRendersHistoricalIdentity(t *testing.T) {
	p := herdrProvisioner("yersonargotev/herdr-tab-git", herdrGitCommit)
	p.Tool = "herdr-custom"
	executable, args := provision.RenderCommand(p)
	if executable != "herdr-custom" || !reflect.DeepEqual(args, []string{"plugin", "install", p.Spec.Plugin, "--ref", p.Spec.Ref, "--yes"}) {
		t.Fatalf("lost historical identity: %q %v", executable, args)
	}
	m := manifestWithProvisioners(p)
	if err := m.Validate(); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
		t.Fatalf("Validate: %v", err)
	}
	// A retained Tag must not lend execution authority to an old implementation.
	for _, tags := range [][]string{{"core"}, {"herdr-custom"}} {
		m.Provisioners[0].Tags = tags
		opts := provision.Options{Selection: &manifest.Selection{Tags: tags}, OS: "darwin", Arch: "arm64"}
		if _, err := provision.Build(m, opts); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
			t.Fatalf("Build: %v", err)
		}
		runner := &fakeRunner{}
		if _, err := provision.Apply(m, opts, lookupWith("herdr-custom"), nil, runner); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
			t.Fatalf("Apply: %v", err)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("retired executable ran: %v", runner.calls)
		}
	}
}
