package configsubset

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/tailscale/hujson"
)

// CanonicalJSONC parses JSONC and returns deterministic strict JSON. Numbers
// remain json.Number values while canonicalizing, so large integer literals are
// never converted through float64.
func CanonicalJSONC(data []byte) ([]byte, error) {
	value, err := decodeJSONC(data)
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode canonical JSONC: %w", err)
	}
	return append(encoded, '\n'), nil
}

// AnalyzeJSONC compares a live JSONC target with a dots-owned JSONC source.
// Object keys are recursively owned; scalar and array values are atomic.
func AnalyzeJSONC(targetData, sourceData []byte) (JSONFileRelation, error) {
	source, err := decodeJSONC(sourceData)
	if err != nil {
		return JSONFileRelation{}, fmt.Errorf("parse source JSONC: %w", err)
	}
	target, err := decodeJSONC(targetData)
	if err != nil {
		return JSONFileRelation{}, nil
	}
	_, changed, compatible := mergeJSONCValue(target, source)
	return JSONFileRelation{Contains: compatible && !changed, Mergeable: compatible && changed}, nil
}

// MergeJSONC adds missing source-owned object keys while preserving the live
// target's comments, whitespace, and trailing-comma style where HuJSON permits.
func MergeJSONC(targetData, sourceData []byte) ([]byte, error) {
	source, err := decodeJSONC(sourceData)
	if err != nil {
		return nil, fmt.Errorf("parse source JSONC: %w", err)
	}
	target, err := decodeJSONC(targetData)
	if err != nil {
		return nil, fmt.Errorf("parse target JSONC: %w", err)
	}
	desired, _, compatible := mergeJSONCValue(target, source)
	if !compatible {
		return nil, fmt.Errorf("source-owned JSONC differences cannot be merged safely")
	}
	return patchJSONC(targetData, target, desired)
}

// ReconcileJSONC updates JSONC using the last recorded dots-owned content.
// previousData accepts either canonical strict JSON metadata or legacy JSONC.
func ReconcileJSONC(targetData, previousData, currentData []byte) (JSONReconciliation, error) {
	previous, err := decodeJSONC(previousData)
	if err != nil {
		return JSONReconciliation{}, fmt.Errorf("parse previous owned JSONC: %w", err)
	}
	current, err := decodeJSONC(currentData)
	if err != nil {
		return JSONReconciliation{}, fmt.Errorf("parse current owned JSONC: %w", err)
	}
	target, err := decodeJSONC(targetData)
	if err != nil {
		return JSONReconciliation{}, nil
	}
	desired, changed, compatible := reconcileJSONCValue(target, previous, current)
	if !compatible {
		return JSONReconciliation{}, nil
	}
	content, err := patchJSONC(targetData, target, desired)
	if err != nil {
		return JSONReconciliation{}, err
	}
	return JSONReconciliation{Content: content, Changed: changed, Compatible: true}, nil
}

// RemoveJSONC removes only the exact JSONC contribution proven by ownedData.
func RemoveJSONC(targetData, ownedData []byte) (content []byte, changed, empty, compatible bool, err error) {
	owned, decodeErr := decodeJSONC(ownedData)
	if decodeErr != nil {
		return nil, false, false, false, fmt.Errorf("parse owned JSONC: %w", decodeErr)
	}
	target, decodeErr := decodeJSONC(targetData)
	if decodeErr != nil {
		return nil, false, false, false, nil
	}
	desired, changed, compatible := subtractJSONCValue(target, owned)
	if !compatible {
		return nil, false, false, false, nil
	}
	content, err = patchJSONC(targetData, target, desired)
	if err != nil {
		return nil, false, false, false, err
	}
	return content, changed, jsonValueEmpty(desired) && !jsonCContainsComments(content), true, nil
}

func decodeJSONC(data []byte) (any, error) {
	standard, err := hujson.Standardize(append([]byte(nil), data...))
	if err != nil {
		return nil, err
	}
	var value any
	if err := decodeJSON(standard, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func mergeJSONCValue(target, source any) (any, bool, bool) {
	sourceObject, sourceIsObject := source.(map[string]any)
	if !sourceIsObject {
		return target, false, reflect.DeepEqual(target, source)
	}
	targetObject, ok := target.(map[string]any)
	if !ok {
		return target, false, false
	}
	result := cloneJSONObject(targetObject)
	changed := false
	for key, sourceChild := range sourceObject {
		targetChild, exists := targetObject[key]
		if !exists {
			result[key] = sourceChild
			changed = true
			continue
		}
		merged, childChanged, compatible := mergeJSONCValue(targetChild, sourceChild)
		if !compatible {
			return target, false, false
		}
		if childChanged {
			result[key] = merged
			changed = true
		}
	}
	return result, changed, true
}

func reconcileJSONCValue(target, previous, current any) (any, bool, bool) {
	previousObject, previousIsObject := previous.(map[string]any)
	currentObject, currentIsObject := current.(map[string]any)
	if previousIsObject && currentIsObject {
		targetObject, ok := target.(map[string]any)
		if !ok {
			return target, false, false
		}
		result := cloneJSONObject(targetObject)
		changed := false
		for key, previousChild := range previousObject {
			currentChild, remainsOwned := currentObject[key]
			targetChild, exists := targetObject[key]
			if !exists {
				if !remainsOwned {
					// A prior interrupted reconciliation may already have
					// removed this retired value.
					continue
				}
				return target, false, false
			}
			if !remainsOwned {
				value, childChanged, compatible := subtractJSONCValue(targetChild, previousChild)
				if !compatible {
					return target, false, false
				}
				if jsonValueEmpty(value) {
					delete(result, key)
					changed = true
				} else {
					result[key] = value
					changed = changed || childChanged
				}
				continue
			}
			value, childChanged, compatible := reconcileJSONCValue(targetChild, previousChild, currentChild)
			if !compatible {
				return target, false, false
			}
			result[key] = value
			changed = changed || childChanged
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
			value, childChanged, compatible := mergeJSONCValue(targetChild, currentChild)
			if !compatible {
				return target, false, false
			}
			result[key] = value
			changed = changed || childChanged
		}
		return result, changed, true
	}
	if reflect.DeepEqual(previous, current) {
		return target, false, reflect.DeepEqual(target, current)
	}
	if reflect.DeepEqual(target, current) {
		return target, false, true
	}
	if reflect.DeepEqual(target, previous) {
		return current, true, true
	}
	return target, false, false
}

func subtractJSONCValue(target, owned any) (any, bool, bool) {
	ownedObject, ownedIsObject := owned.(map[string]any)
	if !ownedIsObject {
		if !reflect.DeepEqual(target, owned) {
			return target, false, false
		}
		return nil, true, true
	}
	targetObject, ok := target.(map[string]any)
	if !ok {
		return target, false, false
	}
	result := cloneJSONObject(targetObject)
	changed := false
	for key, ownedChild := range ownedObject {
		targetChild, exists := targetObject[key]
		if !exists {
			return target, false, false
		}
		value, childChanged, compatible := subtractJSONCValue(targetChild, ownedChild)
		if !compatible {
			return target, false, false
		}
		if jsonValueEmpty(value) {
			delete(result, key)
			changed = true
		} else {
			result[key] = value
			changed = changed || childChanged
		}
	}
	return result, changed, true
}

func cloneJSONObject(input map[string]any) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func jsonCContainsComments(data []byte) bool {
	value, err := hujson.Parse(data)
	return err == nil && jsonCValueContainsComments(value)
}

func jsonCValueContainsComments(value hujson.Value) bool {
	if jsonCExtraContainsComment(value.BeforeExtra) || jsonCExtraContainsComment(value.AfterExtra) {
		return true
	}
	switch typed := value.Value.(type) {
	case *hujson.Object:
		if jsonCExtraContainsComment(typed.AfterExtra) {
			return true
		}
		for _, member := range typed.Members {
			if jsonCValueContainsComments(member.Name) || jsonCValueContainsComments(member.Value) {
				return true
			}
		}
	case *hujson.Array:
		if jsonCExtraContainsComment(typed.AfterExtra) {
			return true
		}
		for _, element := range typed.Elements {
			if jsonCValueContainsComments(element) {
				return true
			}
		}
	}
	return false
}

func jsonCExtraContainsComment(extra hujson.Extra) bool {
	return bytes.Contains(extra, []byte("//")) || bytes.Contains(extra, []byte("/*"))
}

func patchJSONC(original []byte, target, desired any) ([]byte, error) {
	if reflect.DeepEqual(target, desired) {
		return append([]byte(nil), original...), nil
	}
	tree, err := hujson.Parse(original)
	if err != nil {
		return nil, fmt.Errorf("parse target JSONC: %w", err)
	}
	if err := applyJSONCDesired(&tree, target, desired); err != nil {
		return nil, err
	}
	return tree.Pack(), nil
}

func applyJSONCDesired(value *hujson.Value, target, desired any) error {
	targetObject, targetIsObject := target.(map[string]any)
	desiredObject, desiredIsObject := desired.(map[string]any)
	object, treeIsObject := value.Value.(*hujson.Object)
	if targetIsObject && desiredIsObject && treeIsObject {
		for index := len(object.Members) - 1; index >= 0; index-- {
			key := jsonCMemberName(object.Members[index])
			desiredChild, keep := desiredObject[key]
			if !keep {
				moveJSONCMemberExtra(object, index)
				object.Members = append(object.Members[:index], object.Members[index+1:]...)
				continue
			}
			if err := applyJSONCDesired(&object.Members[index].Value, targetObject[key], desiredChild); err != nil {
				return err
			}
		}
		keys := make([]string, 0, len(desiredObject))
		for key := range desiredObject {
			if _, exists := targetObject[key]; !exists {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			member, err := newJSONCMember(key, desiredObject[key])
			if err != nil {
				return err
			}
			member.Name.BeforeExtra = jsonCMemberIndent(object)
			if len(object.Members) > 0 {
				member.Name.AfterExtra = append(hujson.Extra(nil), object.Members[len(object.Members)-1].Name.AfterExtra...)
				member.Value.BeforeExtra = append(hujson.Extra(nil), object.Members[len(object.Members)-1].Value.BeforeExtra...)
			}
			object.Members = append(object.Members, member)
		}
		return nil
	}
	if reflect.DeepEqual(target, desired) {
		return nil
	}
	encoded, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("encode JSONC replacement: %w", err)
	}
	replacement, err := hujson.Parse(encoded)
	if err != nil {
		return fmt.Errorf("parse JSONC replacement: %w", err)
	}
	value.Value = replacement.Value
	return nil
}

func moveJSONCMemberExtra(object *hujson.Object, index int) {
	member := object.Members[index]
	extra := append(hujson.Extra(nil), member.Name.BeforeExtra...)
	extra = appendJSONCCommentExtra(extra, member.Name.AfterExtra)
	extra = appendJSONCValueComments(extra, member.Value)
	if len(extra) == 0 {
		return
	}
	if index+1 < len(object.Members) {
		object.Members[index+1].Name.BeforeExtra = append(extra, object.Members[index+1].Name.BeforeExtra...)
		return
	}
	object.AfterExtra = append(extra, object.AfterExtra...)
}

func appendJSONCValueComments(extra hujson.Extra, value hujson.Value) hujson.Extra {
	extra = appendJSONCCommentExtra(extra, value.BeforeExtra)
	extra = appendJSONCCommentExtra(extra, value.AfterExtra)
	switch typed := value.Value.(type) {
	case *hujson.Object:
		extra = appendJSONCCommentExtra(extra, typed.AfterExtra)
		for _, member := range typed.Members {
			extra = appendJSONCValueComments(extra, member.Name)
			extra = appendJSONCValueComments(extra, member.Value)
		}
	case *hujson.Array:
		extra = appendJSONCCommentExtra(extra, typed.AfterExtra)
		for _, element := range typed.Elements {
			extra = appendJSONCValueComments(extra, element)
		}
	}
	return extra
}

func appendJSONCCommentExtra(dst, src hujson.Extra) hujson.Extra {
	if jsonCExtraContainsComment(src) {
		return append(dst, src...)
	}
	return dst
}

func newJSONCMember(key string, value any) (hujson.ObjectMember, error) {
	encodedKey, err := json.Marshal(key)
	if err != nil {
		return hujson.ObjectMember{}, fmt.Errorf("encode JSONC member name: %w", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return hujson.ObjectMember{}, fmt.Errorf("encode JSONC member: %w", err)
	}
	data := append([]byte{'{'}, encodedKey...)
	data = append(data, ':')
	data = append(data, encoded...)
	data = append(data, '}')
	fragment, err := hujson.Parse(data)
	if err != nil {
		return hujson.ObjectMember{}, fmt.Errorf("parse JSONC member: %w", err)
	}
	object, ok := fragment.Value.(*hujson.Object)
	if !ok || len(object.Members) != 1 {
		return hujson.ObjectMember{}, fmt.Errorf("parse JSONC member: expected object")
	}
	return object.Members[0], nil
}

func jsonCMemberIndent(object *hujson.Object) hujson.Extra {
	if len(object.Members) > 0 {
		extra := object.Members[len(object.Members)-1].Name.BeforeExtra
		if index := bytesLastIndexNewline(extra); index >= 0 {
			return append(hujson.Extra(nil), extra[index:]...)
		}
	}
	if bytesLastIndexNewline(object.AfterExtra) >= 0 {
		extra := object.AfterExtra
		return append(hujson.Extra(nil), extra[bytesLastIndexNewline(extra):]...)
	}
	return hujson.Extra(" ")
}

func bytesLastIndexNewline(extra hujson.Extra) int { return bytes.LastIndexByte(extra, '\n') }

func jsonCMemberName(member hujson.ObjectMember) string {
	var name string
	literal, ok := member.Name.Value.(hujson.Literal)
	if !ok || json.Unmarshal(literal, &name) != nil {
		return ""
	}
	return name
}
