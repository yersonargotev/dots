package deps_test

import (
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
)

func TestResolveTier(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		release deps.OSRelease
		want    deps.Tier
	}{
		{name: "macOS uses homebrew", goos: "darwin", want: deps.TierHomebrew},
		{name: "ubuntu by id", goos: "linux", release: deps.OSRelease{ID: "ubuntu"}, want: deps.TierDebian},
		{name: "debian by id", goos: "linux", release: deps.OSRelease{ID: "debian"}, want: deps.TierDebian},
		{name: "mint by id_like debian", goos: "linux", release: deps.OSRelease{ID: "linuxmint", IDLike: "ubuntu debian"}, want: deps.TierDebian},
		{name: "fedora by id", goos: "linux", release: deps.OSRelease{ID: "fedora"}, want: deps.TierFedora},
		{name: "rhel by id_like fedora", goos: "linux", release: deps.OSRelease{ID: "rocky", IDLike: "rhel centos fedora"}, want: deps.TierFedora},
		{name: "arch by id", goos: "linux", release: deps.OSRelease{ID: "arch"}, want: deps.TierArch},
		{name: "manjaro by id_like arch", goos: "linux", release: deps.OSRelease{ID: "manjaro", IDLike: "arch"}, want: deps.TierArch},
		{name: "unknown linux falls back to generic", goos: "linux", release: deps.OSRelease{ID: "void"}, want: deps.TierGeneric},
		{name: "empty linux falls back to generic", goos: "linux", want: deps.TierGeneric},
		{name: "unknown os falls back to generic", goos: "plan9", want: deps.TierGeneric},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deps.ResolveTier(tt.goos, tt.release); got != tt.want {
				t.Fatalf("ResolveTier(%q, %#v) = %q, want %q", tt.goos, tt.release, got, tt.want)
			}
		})
	}
}

func TestParseOSRelease(t *testing.T) {
	content := `NAME="Ubuntu"
VERSION_ID="22.04"
ID=ubuntu
ID_LIKE="debian"
PRETTY_NAME="Ubuntu 22.04.3 LTS"
`
	got := deps.ParseOSRelease(content)
	if got.ID != "ubuntu" {
		t.Fatalf("ID = %q, want ubuntu", got.ID)
	}
	if got.IDLike != "debian" {
		t.Fatalf("IDLike = %q, want debian", got.IDLike)
	}
}
