// Package completion builds deterministic, manifest-backed completion values.
// It reads only the supplied Install Manifest and never inspects workstation or
// Installation Metadata state.
package completion

import (
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/catalog"
	"github.com/yersonargotev/dots/internal/manifest"
)

// Kind selects the manifest declaration completed by Complete.
type Kind uint8

const (
	Profile Kind = iota
	Tag
)

// Complete loads one Install Manifest and returns current declarations whose
// names match prefix. Values are sorted and carry Cobra-compatible descriptions.
func Complete(path string, kind Kind, prefix string) ([]string, error) {
	m, err := manifest.LoadFile(path)
	if err != nil {
		return nil, err
	}

	report, err := catalog.Build(*m, catalog.Options{OS: "all"})
	if err != nil {
		return nil, err
	}

	switch kind {
	case Profile:
		values := make([]string, 0, len(report.Profiles))
		for _, profile := range report.Profiles {
			if strings.HasPrefix(profile.Name, prefix) {
				values = append(values, annotate(profile.Name, profile.Description))
			}
		}
		return values, nil
	case Tag:
		values := make([]string, 0, len(report.Tags))
		for _, tag := range report.Tags {
			if strings.HasPrefix(tag.Name, prefix) {
				values = append(values, annotate(tag.Name, tag.Description))
			}
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported completion kind %d", kind)
	}
}

func annotate(name, description string) string {
	if strings.TrimSpace(description) == "" {
		return name
	}
	return name + "\t" + description
}
