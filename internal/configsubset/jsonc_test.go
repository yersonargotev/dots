package configsubset

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestMergeJSONCPreservesCommentsAndAddsNestedOwnedKey(t *testing.T) {
	target := []byte("{\n  // kept\n  \"settings\": {\n    \"local\": true,\n  },\n}\n")
	source := []byte(`{"settings":{"owned":9007199254740993}}`)

	got, err := MergeJSONC(target, source)
	if err != nil {
		t.Fatalf("MergeJSONC() error = %v", err)
	}
	if !bytes.Contains(got, []byte("// kept")) || !bytes.Contains(got, []byte(`"local": true`)) || !bytes.Contains(got, []byte(`9007199254740993`)) {
		t.Fatalf("MergeJSONC() = %s, want comment, local content, and precise owned number", got)
	}
	if got[len(got)-1] != '\n' {
		t.Fatalf("MergeJSONC() did not preserve final newline: %q", got)
	}
}

func TestAnalyzeJSONCUsesAtomicArraysAndScalars(t *testing.T) {
	for _, tt := range []struct {
		name   string
		target string
		source string
	}{
		{name: "array content", target: `{"items":["local","owned"]}`, source: `{"items":["owned"]}`},
		{name: "array order", target: `{"items":["two","one"]}`, source: `{"items":["one","two"]}`},
		{name: "scalar", target: `{"theme":"light"}`, source: `{"theme":"dark"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			relation, err := AnalyzeJSONC([]byte(tt.target), []byte(tt.source))
			if err != nil {
				t.Fatalf("AnalyzeJSONC() error = %v", err)
			}
			if relation.Contains || relation.Mergeable {
				t.Fatalf("AnalyzeJSONC() = %#v, want conflict for atomic value", relation)
			}
		})
	}
}

func TestReconcileJSONCReplacesUnchangedAtomicArrayAndRejectsLiveDrift(t *testing.T) {
	previous := []byte(`{"items":["one","two"]}`)
	current := []byte(`{"items":["two","three"]}`)

	got, err := ReconcileJSONC([]byte("{\n  // ordered\n  \"items\": [\"one\", \"two\"],\n}\n"), previous, current)
	if err != nil {
		t.Fatalf("ReconcileJSONC() error = %v", err)
	}
	if !got.Compatible || !got.Changed || !bytes.Contains(got.Content, []byte("// ordered")) || !bytes.Contains(got.Content, []byte(`["two","three"]`)) {
		t.Fatalf("ReconcileJSONC() = %#v, want atomic replacement preserving comment", got)
	}

	drifted, err := ReconcileJSONC([]byte(`{"items":["local"]}`), previous, current)
	if err != nil {
		t.Fatalf("ReconcileJSONC(drifted) error = %v", err)
	}
	if drifted.Compatible {
		t.Fatalf("ReconcileJSONC(drifted) = %#v, want incompatible live array", drifted)
	}
}

func TestReconcileJSONCRemovesRetiredKeysAndPreservesTargetOnlyContent(t *testing.T) {
	target := []byte("{\n  \"settings\": {\n    // local comment\n    \"retired\": \"old\",\n    \"local\": true,\n  },\n}\n")
	previous := []byte(`{"settings":{"retired":"old"}}`)
	current := []byte(`{"settings":{"added":"new"}}`)

	got, err := ReconcileJSONC(target, previous, current)
	if err != nil {
		t.Fatalf("ReconcileJSONC() error = %v", err)
	}
	if !got.Compatible || !got.Changed || !bytes.Contains(got.Content, []byte("// local comment")) || !bytes.Contains(got.Content, []byte(`"local": true`)) || !bytes.Contains(got.Content, []byte(`"added"`)) {
		t.Fatalf("ReconcileJSONC() = %#v, want compatible JSONC edit preserving target-only content", got)
	}
	if bytes.Contains(got.Content, []byte(`"retired"`)) {
		t.Fatalf("ReconcileJSONC() = %s, retained removed key", got.Content)
	}
}

func TestReconcileJSONCReportsRemovingEmptyOwnedObjectAsChange(t *testing.T) {
	got, err := ReconcileJSONC(
		[]byte(`{"settings":{},"local":true}`),
		[]byte(`{"settings":{}}`),
		[]byte(`{}`),
	)
	if err != nil {
		t.Fatalf("ReconcileJSONC() error = %v", err)
	}
	if !got.Compatible || !got.Changed || bytes.Contains(got.Content, []byte(`"settings"`)) || !bytes.Contains(got.Content, []byte(`"local"`)) {
		t.Fatalf("ReconcileJSONC() = %#v, want changed removal of empty owned object", got)
	}
}

func TestRemoveJSONCRemovesOnlyOwnedObjectValues(t *testing.T) {
	target := []byte("{\n  // root\n  \"owned\": true,\n  \"local\": false,\n}\n")
	content, changed, empty, compatible, err := RemoveJSONC(target, []byte(`{"owned":true}`))
	if err != nil {
		t.Fatalf("RemoveJSONC() error = %v", err)
	}
	if !compatible || !changed || empty || !bytes.Contains(content, []byte("// root")) || !bytes.Contains(content, []byte(`"local": false`)) {
		t.Fatalf("RemoveJSONC() = %s, %t, %t, %t", content, changed, empty, compatible)
	}
}

func TestRemoveJSONCPreservesCommentOnlyExternalContent(t *testing.T) {
	target := []byte("{\n  // Added locally and not proven dots-owned.\n  \"owned\": true,\n}\n")
	content, changed, empty, compatible, err := RemoveJSONC(target, []byte(`{"owned":true}`))
	if err != nil {
		t.Fatalf("RemoveJSONC() error = %v", err)
	}
	if !compatible || !changed || empty || !bytes.Contains(content, []byte("Added locally")) {
		t.Fatalf("RemoveJSONC() = %s, changed=%t empty=%t compatible=%t; want preserved comment-only target", content, changed, empty, compatible)
	}
}

func TestJSONCRejectsInvalidOwnedDataAndTreatsInvalidTargetAsIncompatible(t *testing.T) {
	if _, err := AnalyzeJSONC([]byte(`{`), []byte(`{`)); err == nil {
		t.Fatal("AnalyzeJSONC() error = nil, want invalid source error")
	}
	got, err := ReconcileJSONC([]byte(`{`), []byte(`{}`), []byte(`{}`))
	if err != nil || got.Compatible {
		t.Fatalf("ReconcileJSONC() = %#v, %v; want invalid target incompatible", got, err)
	}
}

func TestCanonicalJSONCStripsSyntaxExtensionsWithoutLosingLargeNumbers(t *testing.T) {
	got, err := CanonicalJSONC([]byte("{ // comment\n \"counter\": 9007199254740993,\n}"))
	if err != nil {
		t.Fatalf("CanonicalJSONC() error = %v", err)
	}
	var decoded map[string]json.Number
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("canonical output is not strict JSON: %v", err)
	}
	if gotNumber := decoded["counter"]; gotNumber != "9007199254740993" {
		t.Fatalf("counter = %q, want original integer digits", gotNumber)
	}
}
