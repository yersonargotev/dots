package cli

import (
	"path/filepath"
	"testing"
)

func TestFontDirectoriesOmitsPerUserDirsWhenHomeEmpty(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			for _, dir := range fontDirectories(goos, "") {
				// A missing home must never yield a path relative to the process
				// working directory; only absolute system directories remain.
				if !filepath.IsAbs(dir) {
					t.Fatalf("fontDirectories(%q, \"\") returned non-absolute %q", goos, dir)
				}
			}
		})
	}
}

func TestFontDirectoriesRootsPerUserDirsAtHome(t *testing.T) {
	home := "/tmp/home"
	got := fontDirectories("darwin", home)
	if len(got) == 0 || got[0] != filepath.Join(home, "Library", "Fonts") {
		t.Fatalf("fontDirectories(darwin, home)[0] = %q, want per-user Fonts dir first", got[0])
	}
}
