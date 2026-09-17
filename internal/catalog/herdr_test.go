package catalog

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

func TestHerdrProvisionerCatalogPreservesPinAndArchitecture(t *testing.T) {
	input := manifest.Provisioner{
		Tool: "herdr", Tags: []string{"herdr"}, OS: []string{"darwin"}, Arch: []string{"arm64"},
		Spec: manifest.ProvisionerSpec{Plugin: "yersonargotev/tabby", Ref: "34c01f9791dd3228acae7ca378adb38e09d9fb6c"},
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
