package completion

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompleteReturnsCurrentAnnotatedValuesInDeterministicOrder(t *testing.T) {
	path := writeManifest(t)

	tests := []struct {
		name   string
		kind   Kind
		prefix string
		want   []string
	}{
		{
			name:   "profiles",
			kind:   Profile,
			prefix: "",
			want:   []string{"core\tCore preset", "workstation\tComplete workstation preset"},
		},
		{
			name:   "tags with prefix",
			kind:   Tag,
			prefix: "z",
			want:   []string{"zed\tZed editor", "zsh\tZ shell"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Complete(path, tt.kind, tt.prefix)
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Complete() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCompleteReturnsManifestErrors(t *testing.T) {
	if _, err := Complete(filepath.Join(t.TempDir(), "missing.yaml"), Tag, ""); err == nil {
		t.Fatal("Complete() error = nil, want missing manifest error")
	}
}

func writeManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dots.yaml")
	contents := `version: 1
tags:
  zsh:
    description: Z shell
    kind: surface
    status: current
  legacy:
    description: Legacy alias
    kind: compatibility
    status: legacy
    replaced_by: [zsh]
  zed:
    description: Zed editor
    kind: surface
    status: current
profiles:
  workstation:
    description: Complete workstation preset
    tags: [zsh, zed]
  legacy:
    description: Legacy preset
    status: legacy
    tags: [legacy]
  core:
    description: Core preset
    tags: [zsh]
entries:
  - source: zshrc
    target: ~/.zshrc
    strategy: copy
    tags: [zsh]
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}
