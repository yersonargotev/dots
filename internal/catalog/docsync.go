package catalog

import (
	"fmt"
	"strings"
)

const (
	DocumentationStartMarker = "<!-- dots:catalog:start -->"
	DocumentationEndMarker   = "<!-- dots:catalog:end -->"
)

// ReplaceDocumentationBlock changes only the one generated catalog block. It
// rejects missing, duplicate, and malformed markers so generation cannot alter
// an unintended portion of the narrative guide.
func ReplaceDocumentationBlock(document, fragment string) (string, error) {
	if strings.Count(document, DocumentationStartMarker) != 1 || strings.Count(document, DocumentationEndMarker) != 1 {
		return "", fmt.Errorf("catalog documentation must contain exactly one start and end marker")
	}
	from := strings.Index(document, DocumentationStartMarker)
	to := strings.Index(document, DocumentationEndMarker)
	if to < from {
		return "", fmt.Errorf("catalog documentation markers are malformed")
	}
	contentStart := from + len(DocumentationStartMarker)
	return document[:contentStart] + "\n\n" + strings.TrimSuffix(fragment, "\n") + "\n\n" + document[to:], nil
}
