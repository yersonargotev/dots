package catalog

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

func TestHerdrProvisionerCatalogPreservesPinAndArchitecture(t *testing.T) {
	input := manifest.Provisioner{
		Tool: "herdr", Tags: []string{"herdr"}, OS: []string{"darwin"}, Arch: []string{"arm64"},
		Spec: manifest.ProvisionerSpec{Plugin: "yersonargotev/tabby", Ref: "39960fe0fbc19f02f0b7f69ab8fe3027fe7bc749"},
	}
	got := provisioner(input)
	if got.Identity != input.Spec.Plugin+"@"+input.Spec.Ref || !reflect.DeepEqual(got.Arch, input.Arch) {
		t.Fatalf("catalog loses pinned platform identity: %#v", got)
	}
	other := got
	other.Arch = []string{"amd64"}
	if provisionerKey(got) == provisionerKey(other) {
		t.Fatal("catalog comparison ignores architecture")
	}
	input.Spec.Ref = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if provisionerKey(got) == provisionerKey(provisioner(input)) {
		t.Fatal("catalog comparison ignores plugin revision")
	}
}

func TestOSCatalogIncludesEveryArchitectureRegardlessOfHost(t *testing.T) {
	m := manifest.Manifest{Provisioners: []manifest.Provisioner{
		{Tool: "herdr", Tags: []string{"herdr"}, OS: []string{"darwin"}, Arch: []string{"arm64"}},
		{Tool: "herdr", Tags: []string{"herdr"}, OS: []string{"darwin"}, Arch: []string{"amd64"}},
	}}
	darwin, excluded := selectedSurfaces(m, []string{"herdr"}, "darwin")
	if len(darwin.Provisioners) != 2 || len(excluded.Provisioners) != 0 {
		t.Fatalf("Darwin catalog must include both architectures: selected=%v excluded=%v", darwin.Provisioners, excluded.Provisioners)
	}
	linux, excluded := selectedSurfaces(m, []string{"herdr"}, "linux")
	if len(linux.Provisioners) != 0 || len(excluded.Provisioners) != 2 {
		t.Fatalf("Linux catalog must explain both OS exclusions: selected=%v excluded=%v", linux.Provisioners, excluded.Provisioners)
	}
}
