// Package codexconfig manages dots-owned overlays inside Codex configuration
// files without taking ownership of the whole file.
package codexconfig

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/yersonargotev/dots/internal/textblock"
)

const (
	codeGraphStart       = "<!-- dots:codegraph-mode -->"
	codeGraphEnd         = "<!-- /dots:codegraph-mode -->"
	legacyCodeGraphStart = "<!-- gentle-ai:codegraph-mode -->"
	legacyCodeGraphEnd   = "<!-- /gentle-ai:codegraph-mode -->"
)

const codeGraphInstructions = "CodeGraph Mode: enabled\n\n" +
	"If `.codegraph/` exists in the project, use CodeGraph first for architecture, trace, impact, and symbol-discovery questions instead of re-deriving the same answer with grep/read loops.\n\n" +
	"Use CodeGraph by intent:\n\n" +
	"- `codegraph_context`: map a feature or area first.\n" +
	"- `codegraph_trace`: trace how one symbol reaches another.\n" +
	"- `codegraph_explore`: inspect related symbols in one bounded call.\n" +
	"- `codegraph_search`: find a symbol by name.\n" +
	"- `codegraph_callers` / `codegraph_callees`: walk call flow.\n" +
	"- `codegraph_impact`: check affected code before edits.\n" +
	"- `codegraph_node`: inspect one symbol.\n" +
	"- `codegraph_files`: inspect indexed file structure.\n" +
	"- `codegraph_status`: verify index health.\n\n" +
	"Treat CodeGraph-returned source as already read. Use raw file reads only to verify details CodeGraph did not cover.\n\n" +
	"If `.codegraph/` is missing, ask before running `codegraph init -i`."

// EnsureCodeGraphMode inserts or updates the dots-owned CodeGraph instruction
// block in <home>/.codex/AGENTS.md. It migrates the old manual gentle-ai block
// to dots markers while preserving non-managed content.
func EnsureCodeGraphMode(home string) error {
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	mode := os.FileMode(0o600)
	content, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}

	updated, err := textblock.Upsert(string(content), textblock.Markers{Start: codeGraphStart, End: codeGraphEnd}, codeGraphInstructions, textblock.Markers{Start: legacyCodeGraphStart, End: legacyCodeGraphEnd})
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(updated), mode)
}
