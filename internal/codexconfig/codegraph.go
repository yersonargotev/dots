// Package codexconfig manages dots-owned CodeGraph overlays inside agent
// instruction files without taking ownership of the whole file.
package codexconfig

import (
	"errors"
	"fmt"
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
	"Use CodeGraph for architecture questions, symbol discovery, call flow, impact analysis, and locating relevant source files before edits.\n\n" +
	"For manifest, docs, config, and script changes, prefer `rg`, `sed`, targeted file reads, and tests. Never use CodeGraph just because `.codegraph/` exists.\n\n" +
	"Do NOT use CodeGraph as proof for runtime behavior. Always verify CLI behavior, installers, filesystem writes, `$HOME` and config paths, network tools, GitHub, and CI with real commands or tests."

// EnsureCodeGraphMode inserts or updates the dots-owned CodeGraph instruction
// block in the selected agents' instruction files. With no agents it preserves
// the historical Codex-only behavior. It migrates the old manual gentle-ai block
// to dots markers while preserving non-managed content.
func EnsureCodeGraphMode(home string, agents ...string) error {
	for _, path := range codeGraphInstructionPaths(home, agents) {
		if err := upsertCodeGraphMode(path); err != nil {
			return err
		}
	}
	return nil
}

func codeGraphInstructionPaths(home string, agents []string) []string {
	if len(agents) == 0 {
		return []string{filepath.Join(home, ".codex", "AGENTS.md")}
	}

	var paths []string
	seen := map[string]bool{}
	add := func(path string) {
		if seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}
	for _, agent := range agents {
		switch agent {
		case "codex":
			add(filepath.Join(home, ".codex", "AGENTS.md"))
		case "claude", "claude-code":
			add(filepath.Join(home, ".claude", "CLAUDE.md"))
		case "antigravity":
			add(filepath.Join(home, ".gemini", "GEMINI.md"))
		}
	}
	return paths
}

func upsertCodeGraphMode(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create agent instructions directory: %w", err)
	}

	mode := os.FileMode(0o600)
	content, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read agent instructions %s: %w", path, err)
		}
	} else if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}

	updated, err := textblock.Upsert(string(content), textblock.Markers{Start: codeGraphStart, End: codeGraphEnd}, codeGraphInstructions, textblock.Markers{Start: legacyCodeGraphStart, End: legacyCodeGraphEnd})
	if err != nil {
		return fmt.Errorf("update agent instructions %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(updated), mode); err != nil {
		return fmt.Errorf("write agent instructions %s: %w", path, err)
	}
	return nil
}
