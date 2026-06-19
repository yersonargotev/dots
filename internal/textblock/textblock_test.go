package textblock

import (
	"strings"
	"testing"
)

func TestUpsertInsertsUpdatesAndMigratesBlocks(t *testing.T) {
	primary := Markers{Start: "<!-- dots:block -->", End: "<!-- /dots:block -->"}
	legacy := Markers{Start: "<!-- old:block -->", End: "<!-- /old:block -->"}

	tests := []struct {
		name    string
		content string
		wantHas []string
		wantNot []string
	}{
		{
			name:    "inserts into existing content",
			content: "before\n",
			wantHas: []string{"before", primary.Start, "new body", primary.End},
		},
		{
			name:    "updates primary block",
			content: "before\n" + primary.Start + "\nstale\n" + primary.End + "\nafter\n",
			wantHas: []string{"before", "new body", "after"},
			wantNot: []string{"stale"},
		},
		{
			name:    "migrates legacy block",
			content: "before\n" + legacy.Start + "\nold body\n" + legacy.End + "\nafter\n",
			wantHas: []string{"before", primary.Start, "new body", primary.End, "after"},
			wantNot: []string{legacy.Start, legacy.End, "old body"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Upsert(tt.content, primary, "new body", legacy)
			if err != nil {
				t.Fatalf("Upsert() error = %v", err)
			}
			for _, want := range tt.wantHas {
				if !strings.Contains(got, want) {
					t.Fatalf("Upsert() missing %q\ncontent:\n%s", want, got)
				}
			}
			for _, not := range tt.wantNot {
				if strings.Contains(got, not) {
					t.Fatalf("Upsert() kept %q\ncontent:\n%s", not, got)
				}
			}
			if count := strings.Count(got, primary.Start); count != 1 {
				t.Fatalf("primary start marker count = %d, want 1\ncontent:\n%s", count, got)
			}
		})
	}
}
