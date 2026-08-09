// catalogdoc refreshes only the marked catalog block in the manifest guide.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yersonargotev/dots/internal/catalog"
	"github.com/yersonargotev/dots/internal/manifest"
)

func main() {
	manifestPath := flag.String("manifest", "dots.yaml", "Install Manifest")
	docPath := flag.String("doc", "docs/manifest.md", "manifest documentation")
	flag.Parse()
	m, err := manifest.LoadFile(*manifestPath)
	if err != nil {
		fail(err)
	}
	fragment, err := catalog.Markdown(*m)
	if err != nil {
		fail(err)
	}
	document, err := os.ReadFile(*docPath)
	if err != nil {
		fail(err)
	}
	updated, err := catalog.ReplaceDocumentationBlock(string(document), fragment)
	if err != nil {
		fail(err)
	}
	if updated != string(document) {
		if err := os.WriteFile(*docPath, []byte(updated), 0o644); err != nil {
			fail(err)
		}
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "catalogdoc:", err); os.Exit(1) }
