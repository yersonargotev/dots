package catalog

import (
	"os"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

func TestRepositoryManifestDocumentationCatalogIsCurrent(t *testing.T) {
	m, err := manifest.LoadFile("../../dots.yaml")
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := Markdown(*m)
	if err != nil {
		t.Fatal(err)
	}
	document, err := os.ReadFile("../../docs/manifest.md")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := ReplaceDocumentationBlock(string(document), fragment)
	if err != nil {
		t.Fatal(err)
	}
	if updated != string(document) {
		t.Fatal("docs/manifest.md catalog block is stale; run go generate ./internal/catalog")
	}
}

func TestReplaceDocumentationBlockRequiresOneOrderedMarkerPair(t *testing.T) {
	valid := DocumentationStartMarker + "\nold\n" + DocumentationEndMarker
	updated, err := ReplaceDocumentationBlock(valid, "new\n")
	if err != nil || updated != DocumentationStartMarker+"\n\nnew\n\n"+DocumentationEndMarker {
		t.Fatalf("ReplaceDocumentationBlock() = %q, %v", updated, err)
	}
	for _, document := range []string{
		DocumentationStartMarker,
		DocumentationEndMarker,
		DocumentationEndMarker + DocumentationStartMarker,
		DocumentationStartMarker + DocumentationStartMarker + DocumentationEndMarker,
		DocumentationStartMarker + DocumentationEndMarker + DocumentationEndMarker,
	} {
		if _, err := ReplaceDocumentationBlock(document, "new"); err == nil {
			t.Fatalf("ReplaceDocumentationBlock(%q) succeeded", strings.ReplaceAll(document, "\n", "\\n"))
		}
	}
}
