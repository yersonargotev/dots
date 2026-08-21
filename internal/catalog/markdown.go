package catalog

import (
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// Markdown returns the deterministic documentation fragment shared with the
// catalog report. Narrative documentation deliberately remains outside this
// generated fragment.
func Markdown(m manifest.Manifest) (string, error) {
	report, err := Build(m, Options{OS: "all", IncludeLegacy: true})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString("### Profiles\n\n")
	out.WriteString("| Profile | Status | Tags | Description |\n")
	out.WriteString("|---------|--------|------|-------------|\n")
	for _, profile := range report.Profiles {
		fmt.Fprintf(&out, "| `%s` | %s | %s | %s |\n", profile.Name, profile.Status, codeList(profile.Tags), markdownCell(profile.Description))
	}
	out.WriteString("\n### Tags\n\n")
	out.WriteString("| Tag | Kind | Status | Description | Replacement |\n")
	out.WriteString("|-----|------|--------|-------------|-------------|\n")
	for _, tag := range report.Tags {
		replacement := ""
		if len(tag.ReplacedBy) > 0 {
			replacement = codeList(tag.ReplacedBy)
		}
		fmt.Fprintf(&out, "| `%s` | %s | %s | %s | %s |\n", tag.Name, tag.Kind, tag.Status, markdownCell(tag.Description), replacement)
	}
	return out.String(), nil
}

func codeList(items []string) string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, "`"+item+"`")
	}
	return strings.Join(result, ", ")
}

func markdownCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}
