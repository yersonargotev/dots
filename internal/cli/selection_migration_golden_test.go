package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/selectionmigration"
)

func TestSelectionMigrationTextGolden(t *testing.T) {
	candidate := selectionmigration.Candidate{
		Profiles:           []string{"core"},
		ExtraTags:          []string{"adaptive-theme"},
		EffectiveTags:      []string{"core", "adaptive-theme"},
		Confidence:         selectionmigration.ConfidenceHigh,
		AmbiguityReasons:   []selectionmigration.AmbiguityReason{},
		RecommendedCommand: "dots install --profile core --tag adaptive-theme",
	}
	var out bytes.Buffer
	renderMigrationCandidate(&out, candidate)

	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetOut(&out)
	confirmed, err := confirmSelectionMigration(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed {
		t.Fatal("declined migration was confirmed")
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "Non-interactive error:")
	fmt.Fprintln(&out, "Error:", (&selectionMigrationRequiredError{candidate: &candidate}).Error())

	assertGolden(t, "selection_migration_text.golden", out.Bytes())
}
