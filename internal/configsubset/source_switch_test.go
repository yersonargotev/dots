package configsubset

import (
	"bytes"
	"testing"
)

func TestReplaceOwnedTOML(t *testing.T) {
	tests := []struct {
		name, previous, current, live, want string
		compatible                          bool
	}{
		{"edited value and retired path", "[ui]\nwidth=38\nold=true\n", "[ui]\nwidth=26\nnext=true\n", "# head\n[ui]\nwidth=99 # note\nold=false # keep comment\n\n[personal]\nkeep=\"yes\"\n", "# head\n[ui]\nwidth=26 # note\n# keep comment\nnext=true\n\n[personal]\nkeep=\"yes\"\n", true},
		{"missing formerly owned path", "width=38\n", "width=26\n", "# note\n", "# note\nwidth=26\n", true},
		{"new path collision", "old=true\n", "next=true\n", "old=true\nnext=false\n", "", false},
		{"equal new path is still external", "old=true\n", "next=true\n", "old=true\nnext=true\n", "", false},
		{"scalar became external table", "ui=38\n", "ui=26\n", "[ui]\nexternal=true\n", "", false},
		{"new child beneath old scalar", "ui=38\n", "[ui]\nwidth=26\n", "ui=99\n", "", false},
		{"array tables", "[[ui]]\nwidth=38\n", "[[ui]]\nwidth=26\n", "[[ui]]\nwidth=99\n", "", false},
		{"empty evidence cannot own anything", "", "width=26\n", "width=99\n", "", false},
		{"invalid target", "width=38\n", "width=26\n", "[invalid", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReplaceOwnedTOML([]byte(tt.live), []byte(tt.previous), []byte(tt.current))
			if err != nil {
				t.Fatal(err)
			}
			if got.Compatible != tt.compatible {
				t.Fatalf("compatible=%t, want %t; result %s", got.Compatible, tt.compatible, got.Content)
			}
			if tt.compatible && !bytes.Equal(got.Content, []byte(tt.want)) {
				t.Fatalf("content=%q want %q", got.Content, tt.want)
			}
		})
	}
}
