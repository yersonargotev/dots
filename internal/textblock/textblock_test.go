package textblock

import (
	"bytes"
	"strings"
	"testing"
)

func TestOwnedBlockReconciliationFailsClosedAndPreservesExternalBytes(t *testing.T) {
	markers := DotsManagedMarkers()
	previous := []byte(markers.Start + "\nsource old\n" + markers.End + "\n")
	current := []byte(markers.Start + "\nsource new\n" + markers.End + "\n")
	prefix := []byte("\n# installed by a tool\n")
	suffix := []byte("export THIRD_PARTY=1\n")
	live := append(append(append([]byte(nil), prefix...), previous...), suffix...)
	got := ReconcileOwned(live, previous, current, markers)
	want := append(append(append([]byte(nil), prefix...), current...), suffix...)
	if !got.Compatible || !got.Changed || !bytes.Equal(got.Content, want) {
		t.Fatalf("ReconcileOwned() = %#v, want %q", got, want)
	}
	invalid := map[string][]byte{
		"duplicate":     append(append([]byte(nil), previous...), previous...),
		"missing close": []byte(markers.Start + "\nsource old\n"),
		"moved":         append([]byte("export BEFORE=1\n"), previous...),
		"modified":      []byte(markers.Start + "\nsource changed\n" + markers.End + "\n"),
	}
	for name, content := range invalid {
		t.Run(name, func(t *testing.T) {
			if result := ReconcileOwned(content, previous, current, markers); result.Compatible {
				t.Fatalf("ReconcileOwned() = %#v, want incompatible", result)
			}
		})
	}
}

func TestOwnedBlockMigrationAndRemoval(t *testing.T) {
	markers := DotsManagedMarkers()
	legacy := []byte("source legacy\n")
	current := []byte(markers.Start + "\nsource portable\n" + markers.End + "\n")
	external := []byte("third-party init\n")
	got := MigrateLegacyOwned(append(append([]byte(nil), legacy...), external...), legacy, current, markers)
	want := append(append([]byte(nil), current...), external...)
	if !got.Compatible || !bytes.Equal(got.Content, want) {
		t.Fatalf("MigrateLegacyOwned() = %#v, want %q", got, want)
	}
	content, changed, empty, compatible := RemoveOwned(want, current, markers)
	if !compatible || !changed || empty || !bytes.Equal(content, external) {
		t.Fatalf("RemoveOwned() = (%q, %t, %t, %t)", content, changed, empty, compatible)
	}
	_, _, empty, compatible = RemoveOwned(current, current, markers)
	if !compatible || !empty {
		t.Fatalf("RemoveOwned(empty) = empty %t, compatible %t", empty, compatible)
	}
}

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

func TestExtractBodyReturnsTrimmedMarkedBlock(t *testing.T) {
	markers := Markers{Start: "<!-- dots:block -->", End: "<!-- /dots:block -->"}
	content := "prefix\n" + markers.Start + "\n\nbody\n\n" + markers.End + "\nsuffix\n"

	got, ok, err := ExtractBody(content, markers)
	if err != nil {
		t.Fatalf("ExtractBody() error = %v", err)
	}
	if !ok {
		t.Fatal("ExtractBody() ok = false, want true")
	}
	if got != "body" {
		t.Fatalf("ExtractBody() = %q, want %q", got, "body")
	}
}

func TestExtractBodyReportsMissingClosingMarker(t *testing.T) {
	markers := Markers{Start: "<!-- dots:block -->", End: "<!-- /dots:block -->"}

	_, _, err := ExtractBody(markers.Start+"\nbody\n", markers)
	if err == nil {
		t.Fatal("ExtractBody() error = nil, want missing closing marker error")
	}
}
