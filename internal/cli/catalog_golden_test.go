package cli

import (
	"bytes"
	"testing"
)

func TestCatalogTextGolden(t *testing.T) {
	manifestPath := writeCatalogManifest(t)
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"catalog", "tag", "theme", "--file", manifestPath, "--os", "all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertGolden(t, "catalog_tag.golden", out.Bytes())
}

func TestCatalogJSONGolden(t *testing.T) {
	manifestPath := writeCatalogManifest(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"catalog", "tag", "theme", "--file", manifestPath, "--os", "all", "--output", "json"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("Run() exit code = %d\nstderr:\n%s\nstdout:\n%s", code, stderr.String(), stdout.String())
	}
	assertGolden(t, "envelope_catalog_tag.golden", stdout.Bytes())
}

func TestCatalogComparisonTextGolden(t *testing.T) {
	manifestPath := writeCatalogManifest(t)
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"catalog", "compare", "core", "desktop", "--file", manifestPath, "--os", "all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertGolden(t, "catalog_compare.golden", out.Bytes())
}

func TestCatalogComparisonJSONGolden(t *testing.T) {
	manifestPath := writeCatalogManifest(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"catalog", "compare", "core", "desktop", "--file", manifestPath, "--os", "all", "--output", "json"}, &stdout, &stderr)
	if code != ExitOK {
		t.Fatalf("Run() exit code = %d\nstderr:\n%s\nstdout:\n%s", code, stderr.String(), stdout.String())
	}
	assertGolden(t, "envelope_catalog_compare.golden", stdout.Bytes())
}
