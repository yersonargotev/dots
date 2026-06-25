package bootstrap_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/bootstrap"
)

func TestEnsureClonesMissingInstalledRepository(t *testing.T) {
	sourceRepo := newBootstrapSourceRepo(t)
	sourceRoot := filepath.Join(t.TempDir(), ".local", "share", "dots")

	result, err := bootstrap.Ensure(bootstrap.Options{
		SourceRoot:    sourceRoot,
		RepositoryURL: sourceRepo,
		RepositoryRef: "v0.99.0",
	})
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !result.Cloned {
		t.Fatal("Ensure() should report that it cloned the Installed Repository")
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "dots.yaml")); err != nil {
		t.Fatalf("expected cloned dots.yaml: %v", err)
	}
}

func TestEnsureAcceptsExistingInstalledRepository(t *testing.T) {
	sourceRoot := t.TempDir()
	writeFile(t, filepath.Join(sourceRoot, "dots.yaml"), "version: 1\n", 0o600)

	result, err := bootstrap.Ensure(bootstrap.Options{SourceRoot: sourceRoot})
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if result.Cloned {
		t.Fatal("Ensure() should not clone over a valid Installed Repository")
	}
}

func TestEnsureRejectsNonEmptyInvalidInstalledRepository(t *testing.T) {
	sourceRoot := t.TempDir()
	writeFile(t, filepath.Join(sourceRoot, "partial.txt"), "partial clone\n", 0o600)

	_, err := bootstrap.Ensure(bootstrap.Options{
		SourceRoot:    sourceRoot,
		RepositoryURL: newBootstrapSourceRepo(t),
	})
	if err == nil {
		t.Fatal("Ensure() should reject a non-empty invalid Installed Repository")
	}
}
