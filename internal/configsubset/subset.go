// Package configsubset compares dots-owned configuration fragments against
// co-owned structured configuration files.
package configsubset

import (
	"bytes"
	"encoding/json"
	"errors"
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

// JSONReconciliation describes a safe three-state update from a previously
// recorded dots-owned contribution to the current Source of Truth. Compatible
// is false when the live target changed a formerly owned value. Changed reports
// whether applying Content would mutate the live target.
type JSONReconciliation struct {
	Content    []byte
	Changed    bool
	Compatible bool
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

// ReconcileJSON compares the live target with the previously recorded and
// current dots-owned JSON contributions. It adds new owned values, removes
// retired values only while they remain unchanged, and preserves target-only
// content. Invalid target JSON is incompatible; invalid owned content is an
// error because both snapshots are dots-controlled evidence.
func ReconcileJSON(targetData, previousData, currentData []byte) (JSONReconciliation, error) {
	var targetValue, previousValue, currentValue any
	if err := decodeJSON(previousData, &previousValue); err != nil {
		return JSONReconciliation{}, fmt.Errorf("parse previous owned JSON: %w", err)
	}
	if err := decodeJSON(currentData, &currentValue); err != nil {
		return JSONReconciliation{}, fmt.Errorf("parse current owned JSON: %w", err)
	}
	if err := decodeJSON(targetData, &targetValue); err != nil {
		return JSONReconciliation{}, nil
	}

	result := reconcileJSONValue(targetValue, previousValue, currentValue)
	if !result.compatible {
		return JSONReconciliation{}, nil
	}
	data, err := json.MarshalIndent(result.value, "", "  ")
	if err != nil {
		return JSONReconciliation{}, fmt.Errorf("encode reconciled JSON: %w", err)
	}
	return JSONReconciliation{
		Content:    append(data, '\n'),
		Changed:    result.changed,
		Compatible: true,
	}, nil
}

// RemoveJSON subtracts a recorded dots-owned contribution from a live target.
// It removes only values that still match the ownership evidence and preserves
// target-only content. Empty reports whether no content remains in the root
// container, allowing uninstall to remove the physical target safely.
func RemoveJSON(targetData, ownedData []byte) (content []byte, changed, empty, compatible bool, err error) {
	var targetValue, ownedValue any
	if decodeErr := decodeJSON(ownedData, &ownedValue); decodeErr != nil {
		err = fmt.Errorf("parse owned JSON: %w", decodeErr)
		return
	}
	if decodeErr := decodeJSON(targetData, &targetValue); decodeErr != nil {
		return nil, false, false, false, nil
	}

	result := subtractJSONValue(targetValue, ownedValue)
	if !result.compatible {
		return nil, false, false, false, nil
	}
	empty = jsonValueEmpty(result.value)
	data, encodeErr := json.MarshalIndent(result.value, "", "  ")
	if encodeErr != nil {
		return nil, false, false, false, fmt.Errorf("encode JSON after removing owned content: %w", encodeErr)
	}
	return append(data, '\n'), result.changed, empty, true, nil
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

// mergeLegacyTOML preserves the existing array-of-table append behavior used by
// Codex hook fragments. Ordinary table/scalar ownership uses toml.go.
func mergeLegacyTOML(targetData, sourceData []byte) ([]byte, error) {
	sourceValues, err := parseSimpleTOML(sourceData, nil)
	if err != nil {
		return nil, fmt.Errorf("parse source TOML: %w", err)
	}
	targetValues, err := parseSimpleTOML(targetData, sourceValuePaths(sourceValues))
	if err == nil {
		contains := true
		for key, sourceValue := range sourceValues {
			if targetValues[key] != sourceValue {
				contains = false
				break
			}
		}
		if contains {
			return append([]byte(nil), targetData...), nil
		}
	}
	blocks, err := missingArrayTableBlocks(targetData, sourceData)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, errors.New("source-owned TOML differences cannot be merged safely")
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
	mergedValues, err := parseSimpleTOML(merged, sourceValuePaths(sourceValues))
	if err != nil {
		return nil, fmt.Errorf("parse merged TOML: %w", err)
	}
	for key, sourceValue := range sourceValues {
		if mergedValues[key] != sourceValue {
			return nil, errors.New("merged TOML target still misses source-owned values")
		}
	}
	return merged, nil
}

type jsonMergeResult struct {
	value      any
	changed    bool
	compatible bool
}

func reconcileJSONValue(target, previous, current any) jsonMergeResult {
	previousObject, previousIsObject := previous.(map[string]any)
	currentObject, currentIsObject := current.(map[string]any)
	if previousIsObject && currentIsObject {
		targetObject, ok := target.(map[string]any)
		if !ok {
			return jsonMergeResult{value: target}
		}
		result := make(map[string]any, len(targetObject)+len(currentObject))
		for key, value := range targetObject {
			result[key] = value
		}
		changed := false
		for key, previousChild := range previousObject {
			targetChild, targetExists := targetObject[key]
			currentChild, currentExists := currentObject[key]
			if !currentExists {
				if !targetExists {
					return jsonMergeResult{value: target}
				}
				removed := subtractJSONValue(targetChild, previousChild)
				if !removed.compatible {
					return jsonMergeResult{value: target}
				}
				if jsonValueEmpty(removed.value) {
					delete(result, key)
					changed = true
				} else {
					result[key] = removed.value
					changed = changed || removed.changed
				}
				continue
			}
			if !targetExists {
				result[key] = currentChild
				changed = true
				continue
			}
			child := reconcileJSONValue(targetChild, previousChild, currentChild)
			if !child.compatible {
				return jsonMergeResult{value: target}
			}
			if child.changed {
				result[key] = child.value
				changed = true
			}
		}
		for key, currentChild := range currentObject {
			if _, existed := previousObject[key]; existed {
				continue
			}
			targetChild, exists := targetObject[key]
			if !exists {
				result[key] = currentChild
				changed = true
				continue
			}
			merged := mergeJSONValue(targetChild, currentChild)
			if !merged.compatible {
				return jsonMergeResult{value: target}
			}
			if merged.changed {
				result[key] = merged.value
				changed = true
			}
		}
		return jsonMergeResult{value: result, changed: changed, compatible: true}
	}

	previousArray, previousIsArray := previous.([]any)
	currentArray, currentIsArray := current.([]any)
	if previousIsArray && currentIsArray {
		targetArray, ok := target.([]any)
		if !ok {
			return jsonMergeResult{value: target}
		}
		result := append([]any(nil), targetArray...)
		changed := false
		for _, previousItem := range multisetDifference(previousArray, currentArray) {
			index := equalJSONIndex(result, previousItem)
			if index < 0 {
				return jsonMergeResult{value: target}
			}
			result = append(result[:index], result[index+1:]...)
			changed = true
		}
		merged := mergeJSONValue(result, currentArray)
		if !merged.compatible {
			return jsonMergeResult{value: target}
		}
		return jsonMergeResult{value: merged.value, changed: changed || merged.changed, compatible: true}
	}

	if reflect.DeepEqual(previous, current) {
		return jsonMergeResult{value: target, compatible: reflect.DeepEqual(target, current)}
	}
	if reflect.DeepEqual(target, current) {
		return jsonMergeResult{value: target, compatible: true}
	}
	if reflect.DeepEqual(target, previous) {
		return jsonMergeResult{value: current, changed: true, compatible: true}
	}
	return jsonMergeResult{value: target}
}

func subtractJSONValue(target, owned any) jsonMergeResult {
	switch ownedTyped := owned.(type) {
	case map[string]any:
		targetTyped, ok := target.(map[string]any)
		if !ok {
			return jsonMergeResult{value: target}
		}
		result := make(map[string]any, len(targetTyped))
		for key, value := range targetTyped {
			result[key] = value
		}
		changed := false
		for key, ownedChild := range ownedTyped {
			targetChild, exists := targetTyped[key]
			if !exists {
				return jsonMergeResult{value: target}
			}
			removed := subtractJSONValue(targetChild, ownedChild)
			if !removed.compatible {
				return jsonMergeResult{value: target}
			}
			if jsonValueEmpty(removed.value) {
				delete(result, key)
				changed = true
			} else {
				result[key] = removed.value
				changed = changed || removed.changed
			}
		}
		return jsonMergeResult{value: result, changed: changed, compatible: true}
	case []any:
		targetTyped, ok := target.([]any)
		if !ok {
			return jsonMergeResult{value: target}
		}
		result := append([]any(nil), targetTyped...)
		for _, ownedItem := range ownedTyped {
			index := equalJSONIndex(result, ownedItem)
			if index < 0 {
				return jsonMergeResult{value: target}
			}
			result = append(result[:index], result[index+1:]...)
		}
		return jsonMergeResult{value: result, changed: len(ownedTyped) > 0, compatible: true}
	default:
		if !reflect.DeepEqual(target, owned) {
			return jsonMergeResult{value: target}
		}
		return jsonMergeResult{changed: true, compatible: true}
	}
}

func multisetDifference(left, right []any) []any {
	remaining := append([]any(nil), right...)
	var difference []any
	for _, item := range left {
		index := equalJSONIndex(remaining, item)
		if index < 0 {
			difference = append(difference, item)
			continue
		}
		remaining = append(remaining[:index], remaining[index+1:]...)
	}
	return difference
}

func equalJSONIndex(values []any, want any) int {
	for i, value := range values {
		if reflect.DeepEqual(value, want) {
			return i
		}
	}
	return -1
}

func jsonValueEmpty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case map[string]any:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	default:
		return false
	}
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
