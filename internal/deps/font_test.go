package deps_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
)

func TestScanFontsMatchesRecursivelyCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "CascadiaCode")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	// Installed font file lives in a nested directory and its casing differs
	// from the declared glob; detection must still find it.
	fontFile := filepath.Join(nested, "cascadiacodenf-regular.ttf")
	if err := os.WriteFile(fontFile, []byte("font"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if !deps.ScanFonts([]string{root}, "CascadiaCodeNF*") {
		t.Fatalf("ScanFonts() = false, want true for installed matching font")
	}
}

func TestScanFontsToleratesUnreadableSubdir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := t.TempDir()
	// A matching font lives at the top level; a sibling subdirectory is
	// unreadable. The permission error on the subtree must not abort the scan.
	if err := os.WriteFile(filepath.Join(root, "CascadiaCodeNF-Bold.ttf"), []byte("font"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o000); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if !deps.ScanFonts([]string{root}, "CascadiaCodeNF*") {
		t.Fatalf("ScanFonts() = false, want true despite an unreadable subdirectory")
	}
}

func TestScanFontsToleratesMissingRootAndNoMatch(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "Arial.ttf")
	if err := os.WriteFile(other, []byte("font"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	missing := filepath.Join(root, "does-not-exist")

	// A missing font directory must not abort the scan; with no matching file
	// anywhere, detection reports absence.
	if deps.ScanFonts([]string{missing, root}, "CascadiaCodeNF*") {
		t.Fatalf("ScanFonts() = true, want false when no font matches")
	}
}
