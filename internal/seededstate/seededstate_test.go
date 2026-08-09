package seededstate

import (
	"bytes"
	"testing"
)

func TestReconcile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		live       []byte
		previous   []byte
		current    []byte
		wantClass  Classification
		wantChange bool
		wantData   []byte
	}{
		{
			name:      "live matches current baseline",
			live:      []byte("revision: current\n"),
			previous:  []byte("revision: previous\n"),
			current:   []byte("revision: current\n"),
			wantClass: AlignedCurrent,
		},
		{
			name:       "unchanged live state advances to new baseline",
			live:       []byte("revision: previous\n"),
			previous:   []byte("revision: previous\n"),
			current:    []byte("revision: current\n"),
			wantClass:  AdvanceBaseline,
			wantChange: true,
			wantData:   []byte("revision: current\n"),
		},
		{
			name:      "local evolution remains untouched",
			live:      []byte("revision: local\n"),
			previous:  []byte("revision: previous\n"),
			current:   []byte("revision: current\n"),
			wantClass: LocalEvolution,
		},
		{
			name:      "equal baselines are aligned",
			live:      []byte("revision: stable\n"),
			previous:  []byte("revision: stable\n"),
			current:   []byte("revision: stable\n"),
			wantClass: AlignedCurrent,
		},
		{
			name:      "difference from equal baselines is local evolution",
			live:      []byte("revision: local\n"),
			previous:  []byte("revision: stable\n"),
			current:   []byte("revision: stable\n"),
			wantClass: LocalEvolution,
		},
		{
			name:       "current baseline removes old content",
			live:       []byte("plugin = old\n"),
			previous:   []byte("plugin = old\n"),
			current:    []byte("plugin = current\n"),
			wantClass:  AdvanceBaseline,
			wantChange: true,
			wantData:   []byte("plugin = current\n"),
		},
		{
			name:       "current baseline may be empty",
			live:       []byte("plugin = old\n"),
			previous:   []byte("plugin = old\n"),
			current:    []byte{},
			wantClass:  AdvanceBaseline,
			wantChange: true,
			wantData:   []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Reconcile(tt.live, tt.previous, tt.current)
			if got.Classification != tt.wantClass {
				t.Fatalf("classification = %q, want %q", got.Classification, tt.wantClass)
			}
			if got.Changed != tt.wantChange {
				t.Fatalf("changed = %t, want %t", got.Changed, tt.wantChange)
			}
			if !bytes.Equal(got.Content, tt.wantData) {
				t.Fatalf("content = %q, want %q", got.Content, tt.wantData)
			}
		})
	}
}

func TestReconcileCopiesAdvanceContent(t *testing.T) {
	current := []byte("revision: current\n")
	result := Reconcile([]byte("revision: previous\n"), []byte("revision: previous\n"), current)
	result.Content[0] = 'R'

	if string(current) != "revision: current\n" {
		t.Fatalf("current baseline was mutated: %q", current)
	}
}
