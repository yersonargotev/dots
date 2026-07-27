// Package configsubset compares dots-owned configuration fragments against
// co-owned agent configuration files.
package configsubset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
)

func decodeJSON(data []byte, value *any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

// JSONFileRelation describes how a target relates to a source-owned JSON
// subset. At most one field can be true.
type JSONFileRelation struct {
	Contains  bool
	Mergeable bool
}

// ComposeJSONFiles reads source-owned JSON fragments and composes them in
// manifest order. Compatible object and array values are merged using the same
// subset semantics as MergeJSONFile. The returned JSON is deterministic and
// suitable for a later plan or apply step; no source file is modified.
func ComposeJSONFiles(sources []string) ([]byte, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("compose JSON: no sources")
	}

	var composed any
	for index, source := range sources {
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read source JSON %s: %w", source, err)
		}
		var sourceValue any
		if err := decodeJSON(data, &sourceValue); err != nil {
			return nil, fmt.Errorf("parse source JSON %s: %w", source, err)
		}
		if index == 0 {
			composed = sourceValue
			continue
		}
		result := mergeJSONValue(composed, sourceValue)
		if !result.compatible {
			return nil, fmt.Errorf("compose source JSON %s: incompatible overlap", source)
		}
		composed = result.value
	}

	data, err := json.MarshalIndent(composed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode composed JSON: %w", err)
	}
	return append(data, '\n'), nil
}

// AnalyzeJSON compares target JSON content with a source-owned JSON subset.
// Invalid target JSON is incompatible; invalid source JSON is returned as an
// error because source content belongs to the repository-owned baseline.
func AnalyzeJSON(targetData, sourceData []byte) (JSONFileRelation, error) {
	var sourceValue, targetValue any
	if err := decodeJSON(sourceData, &sourceValue); err != nil {
		return JSONFileRelation{}, fmt.Errorf("parse source JSON: %w", err)
	}
	if err := decodeJSON(targetData, &targetValue); err != nil {
		return JSONFileRelation{}, nil
	}
	result := mergeJSONValue(targetValue, sourceValue)
	return JSONFileRelation{
		Contains:  result.compatible && !result.changed,
		Mergeable: result.compatible && result.changed,
	}, nil
}

// MergeJSON adds source-owned values missing from target JSON content. It
// returns deterministic indented JSON without mutating either input.
func MergeJSON(targetData, sourceData []byte) ([]byte, error) {
	var sourceValue, targetValue any
	if err := decodeJSON(sourceData, &sourceValue); err != nil {
		return nil, fmt.Errorf("parse source JSON: %w", err)
	}
	if err := decodeJSON(targetData, &targetValue); err != nil {
		return nil, fmt.Errorf("parse target JSON: %w", err)
	}
	result := mergeJSONValue(targetValue, sourceValue)
	if !result.compatible {
		return nil, fmt.Errorf("source-owned JSON differences cannot be merged safely")
	}
	data, err := json.MarshalIndent(result.value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode merged JSON: %w", err)
	}
	return append(data, '\n'), nil
}

// AnalyzeJSONFiles compares target with source in one read and parse pass.
// Invalid target JSON is incompatible; invalid source JSON is an error because
// source belongs to the repository-owned baseline.
func AnalyzeJSONFiles(target, source string) (JSONFileRelation, error) {
	sourceData, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return JSONFileRelation{}, nil
		}
		return JSONFileRelation{}, fmt.Errorf("read %s: %w", source, err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return JSONFileRelation{}, fmt.Errorf("read %s: %w", target, err)
	}

	relation, err := AnalyzeJSON(targetData, sourceData)
	if err != nil {
		return JSONFileRelation{}, fmt.Errorf("analyze source JSON %s: %w", source, err)
	}
	return relation, nil
}

// JSONFileContains reports whether target contains every value present in
// source. Object keys are subset-owned and array items may be a subset of the
// target array.
func JSONFileContains(target, source string) (bool, error) {
	relation, err := AnalyzeJSONFiles(target, source)
	return relation.Contains, err
}

// MergeJSONFile adds source-owned values that are missing from target while
// preserving target-only object keys and array elements. Existing incompatible
// values are left untouched and reported as a conflict.
func MergeJSONFile(target, source string) error {
	sourceData, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read %s: %w", target, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("stat %s: %w", target, err)
	}

	data, err := MergeJSON(targetData, sourceData)
	if err != nil {
		return fmt.Errorf("merge target JSON %s with source JSON %s: %w", target, source, err)
	}
	if err := os.WriteFile(target, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

// TOMLFileContains reports whether target contains every scalar/array setting
// present in source. The source is parsed strictly because it is the dots-owned
// fragment. The target is parsed only for source-owned paths, so unrelated TOML
// added by Codex or provisioners cannot make the dots-owned subset drift.
func TOMLFileContains(target, source string) (bool, error) {
	sourceData, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", source, err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", target, err)
	}

	sourceValues, err := parseSimpleTOML(sourceData, nil)
	if err != nil {
		return false, fmt.Errorf("parse source TOML %s: %w", source, err)
	}
	targetValues, err := parseSimpleTOML(targetData, sourceValuePaths(sourceValues))
	if err != nil {
		return false, nil
	}
	for key, sourceValue := range sourceValues {
		if targetValues[key] != sourceValue {
			return false, nil
		}
	}
	return true, nil
}

// MergeTOMLFile appends missing source-owned array-of-table blocks to target.
// It is intentionally narrower than a full TOML formatter: dots only needs this
// for co-owned Codex config hooks, where preserving user/Codex-owned settings is
// more important than reformatting the whole file. Scalar/table value changes
// remain conflicts unless a caller adds a dedicated migration.
func MergeTOMLFile(target, source string) error {
	contains, err := TOMLFileContains(target, source)
	if err != nil {
		return err
	}
	if contains {
		return nil
	}

	sourceData, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read %s: %w", target, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("stat %s: %w", target, err)
	}
	blocks, err := missingArrayTableBlocks(targetData, sourceData)
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return fmt.Errorf("source-owned TOML differences cannot be merged safely")
	}

	merged := append([]byte(nil), targetData...)
	if len(merged) > 0 && merged[len(merged)-1] != '\n' {
		merged = append(merged, '\n')
	}
	merged = append(merged, '\n')
	for _, block := range blocks {
		merged = append(merged, strings.TrimRight(block, "\n")...)
		merged = append(merged, '\n', '\n')
	}
	if err := os.WriteFile(target, merged, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	contains, err = TOMLFileContains(target, source)
	if err != nil {
		return err
	}
	if !contains {
		return fmt.Errorf("merged TOML target still misses source-owned values")
	}
	return nil
}

type jsonMergeResult struct {
	value      any
	changed    bool
	compatible bool
}

func mergeJSONValue(target, source any) jsonMergeResult {
	switch sourceTyped := source.(type) {
	case map[string]any:
		targetTyped, ok := target.(map[string]any)
		if !ok {
			return jsonMergeResult{value: target}
		}
		merged := make(map[string]any, len(targetTyped)+len(sourceTyped))
		for key, value := range targetTyped {
			merged[key] = value
		}
		changed := false
		for key, sourceChild := range sourceTyped {
			targetChild, exists := targetTyped[key]
			if !exists {
				merged[key] = sourceChild
				changed = true
				continue
			}
			child := mergeJSONValue(targetChild, sourceChild)
			if !child.compatible {
				return jsonMergeResult{value: target}
			}
			if child.changed {
				merged[key] = child.value
				changed = true
			}
		}
		return jsonMergeResult{value: merged, changed: changed, compatible: true}
	case []any:
		targetTyped, ok := target.([]any)
		if !ok {
			return jsonMergeResult{value: target}
		}
		merged := append([]any(nil), targetTyped...)
		changed := false
		for _, sourceItem := range sourceTyped {
			found := false
			for _, targetItem := range merged {
				relation := mergeJSONValue(targetItem, sourceItem)
				if relation.compatible && !relation.changed {
					found = true
					break
				}
			}
			if !found {
				merged = append(merged, sourceItem)
				changed = true
			}
		}
		return jsonMergeResult{value: merged, changed: changed, compatible: true}
	default:
		return jsonMergeResult{
			value:      target,
			compatible: reflect.DeepEqual(target, source),
		}
	}
}

func sourceValuePaths(values map[string]string) map[string]struct{} {
	paths := make(map[string]struct{}, len(values))
	for path := range values {
		paths[path] = struct{}{}
	}
	return paths
}

func parseSimpleTOML(data []byte, wanted map[string]struct{}) (map[string]string, error) {
	values := map[string]string{}
	section := ""
	for lineNo, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripTOMLComment(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[[") {
			if !strings.HasSuffix(line, "]]") {
				return nil, fmt.Errorf("line %d: malformed array-of-table header", lineNo+1)
			}
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
			if section == "" {
				return nil, fmt.Errorf("line %d: empty array-of-table header", lineNo+1)
			}
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, fmt.Errorf("line %d: malformed table header", lineNo+1)
			}
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if section == "" {
				return nil, fmt.Errorf("line %d: empty table header", lineNo+1)
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			if wanted != nil {
				continue
			}
			return nil, fmt.Errorf("line %d: expected key/value assignment", lineNo+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNo+1)
		}
		path := key
		if section != "" {
			path = section + "." + key
		}
		if wanted != nil {
			if _, ok := wanted[path]; !ok {
				continue
			}
		}
		canonical, err := canonicalTOMLValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
		}
		values[path] = canonical
	}
	return values, nil
}

func missingArrayTableBlocks(targetData, sourceData []byte) ([]string, error) {
	sourceValues, err := parseSimpleTOML(sourceData, nil)
	if err != nil {
		return nil, err
	}
	targetValues, err := parseSimpleTOML(targetData, sourceValuePaths(sourceValues))
	if err != nil {
		return nil, err
	}
	missing := map[string]struct{}{}
	for path, sourceValue := range sourceValues {
		if targetValues[path] != sourceValue {
			missing[path] = struct{}{}
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}

	var blocks []string
	lines := strings.Split(string(sourceData), "\n")
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(stripTOMLComment(lines[i]))
		if !strings.HasPrefix(line, "[[") || !strings.HasSuffix(line, "]]") {
			i++
			continue
		}
		start := i
		section := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
		i++
		blockValues := map[string]string{}
		for i < len(lines) {
			next := strings.TrimSpace(stripTOMLComment(lines[i]))
			if strings.HasPrefix(next, "[") && strings.HasSuffix(next, "]") {
				break
			}
			key, value, ok := strings.Cut(next, "=")
			if ok {
				path := section + "." + strings.TrimSpace(key)
				canonical, err := canonicalTOMLValue(strings.TrimSpace(value))
				if err != nil {
					return nil, err
				}
				blockValues[path] = canonical
			}
			i++
		}
		appendBlock := false
		for path := range blockValues {
			if _, ok := missing[path]; ok {
				appendBlock = true
				break
			}
		}
		if appendBlock {
			blocks = append(blocks, strings.Join(lines[start:i], "\n"))
		}
	}
	return blocks, nil
}

func stripTOMLComment(line string) string {
	inString := false
	escaped := false
	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == '#' && !inString:
			return line[:i]
		}
	}
	return line
}

func canonicalTOMLValue(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("empty value")
	}
	if strings.HasPrefix(value, "[") {
		return canonicalTOMLStringArray(value)
	}
	if strings.HasPrefix(value, "\"") {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		encoded, _ := json.Marshal(unquoted)
		return string(encoded), nil
	}
	lower := strings.ToLower(value)
	if lower == "true" || lower == "false" {
		return lower, nil
	}
	return value, nil
}

func canonicalTOMLStringArray(value string) (string, error) {
	if !strings.HasSuffix(value, "]") {
		return "", fmt.Errorf("multi-line arrays are unsupported")
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if inner == "" {
		return "[]", nil
	}
	parts, err := splitTOMLArray(inner)
	if err != nil {
		return "", err
	}
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item, err := strconv.Unquote(strings.TrimSpace(part))
		if err != nil {
			return "", err
		}
		items = append(items, item)
	}
	encoded, _ := json.Marshal(items)
	return string(encoded), nil
}

func splitTOMLArray(inner string) ([]string, error) {
	var parts []string
	start := 0
	inString := false
	escaped := false
	for i, r := range inner {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == ',' && !inString:
			parts = append(parts, inner[start:i])
			start = i + 1
		}
	}
	if inString {
		return nil, fmt.Errorf("unterminated string in array")
	}
	parts = append(parts, inner[start:])
	return parts, nil
}
