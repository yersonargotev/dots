package bootstrap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/bootstrap"
	"github.com/yersonargotev/dots/internal/testrepo"
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

func TestEnsureLeavesExistingInstalledRepositoryUnlessUpdateRequested(t *testing.T) {
	sourceRepo, sourceRoot := seedStaleBootstrapInstalledRepository(t)

	if _, err := bootstrap.Ensure(bootstrap.Options{
		SourceRoot:    sourceRoot,
		RepositoryURL: sourceRepo,
		RepositoryRef: "v0.99.1",
	}); err != nil {
		t.Fatalf("Ensure() without update request error = %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(sourceRoot, "dots.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if strings.Contains(string(manifest), "configs/herdr/config.toml") {
		t.Fatalf("Ensure() should not update an existing source root, got:\n%s", manifest)
	}
}

func TestRequireCurrentRefRejectsStaleInstalledRepositoryWithoutUpdating(t *testing.T) {
	_, sourceRoot := seedStaleBootstrapInstalledRepository(t)

	err := bootstrap.RequireCurrentRef(bootstrap.Options{SourceRoot: sourceRoot, RepositoryRef: "v0.99.1"})
	if err == nil {
		t.Fatal("RequireCurrentRef() should reject a stale Installed Repository")
	}
	if !strings.Contains(err.Error(), "not at v0.99.1") {
		t.Fatalf("stale ref error should explain remediation, got: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(sourceRoot, "dots.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if strings.Contains(string(manifest), "configs/herdr/config.toml") {
		t.Fatalf("RequireCurrentRef() must not update the source root, got:\n%s", manifest)
	}
}

func seedStaleBootstrapInstalledRepository(t *testing.T) (string, string) {
	t.Helper()
	sourceRepo := newBootstrapSourceRepo(t)
	if err := testrepo.TagWithHerdrManifest(sourceRepo, "v0.99.1"); err != nil {
		t.Fatalf("tag Source of Truth with Herdr manifest: %v", err)
	}

	sourceRoot := filepath.Join(t.TempDir(), ".local", "share", "dots")
	if _, err := bootstrap.Ensure(bootstrap.Options{
		SourceRoot:    sourceRoot,
		RepositoryURL: sourceRepo,
		RepositoryRef: "v0.99.0",
	}); err != nil {
		t.Fatalf("seed old Installed Repository: %v", err)
	}
	return sourceRepo, sourceRoot
}
