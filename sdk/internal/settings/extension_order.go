package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// orderedExtensionJSON renders the merged settings in Pi's member order: the
// global file's members, then project-only members, then override-only members.
// Nested objects merge member by member with the same order.
func (m *Manager) orderedExtensionJSON() ([]byte, error) {
	global, err := settingsOrderedTree(m.globalPath())
	if err != nil {
		return nil, err
	}
	project := &jsonValue{object: []jsonMember{}}
	if m.trusted {
		project, err = settingsOrderedTree(m.projectPath())
		if err != nil {
			return nil, err
		}
	}
	order := mergeOrderedTrees(global, project)
	if len(m.overrides) > 0 {
		overrideTree := &jsonValue{object: []jsonMember{}}
		raw, err := marshalJSONValue(m.overrides)
		if err == nil {
			if parsed, parseErr := parseJSONValue(raw); parseErr == nil && parsed.object != nil {
				overrideTree = parsed
			}
		}
		order = mergeOrderedTrees(order, overrideTree)
	}
	content, err := marshalJSONValue(decodeSettings(m.merged))
	if err != nil {
		return order.MarshalJSON()
	}
	tree, err := parseJSONValue(content)
	if err != nil {
		return order.MarshalJSON()
	}
	return reorderOrderedValue(tree, order).MarshalJSON()
}

func settingsOrderedTree(path string) (*jsonValue, error) {
	tree := &jsonValue{object: []jsonMember{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return tree, nil
		}
		return nil, err
	}
	originalLength := len(data)
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if originalLength > 0 && len(data) == 0 {
		return nil, fmt.Errorf("unexpected end of JSON input")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return tree, nil
	}
	parsed, err := parseJSONValue(data)
	if err != nil {
		return nil, err
	}
	if parsed.object != nil {
		tree = parsed
	} else if parsed.array != nil {
		for i, item := range parsed.array {
			tree.set(fmt.Sprint(i), item)
		}
	}
	raw, err := tree.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var existing map[string]json.RawMessage
	if json.Unmarshal(raw, &existing) == nil && existing != nil {
		migrated := cloneRawMap(existing)
		migrationOrder := migrateSettingsOrdered(migrated)
		applyRawDiffOrdered(tree, existing, migrated, migrationOrder)
	}
	return tree, nil
}

// reorderOrderedValue returns content's values with the member order of order,
// recursively for nested objects. Members missing from content stay dropped,
// as Pi first encodes its typed Settings and only then applies the file order.
func reorderOrderedValue(content, order *jsonValue) *jsonValue {
	if content == nil {
		return nil
	}
	if content.object == nil || order == nil || order.object == nil {
		return content
	}
	result := &jsonValue{object: []jsonMember{}}
	seen := map[string]bool{}
	for _, member := range order.object {
		value, ok := content.get(member.key)
		if !ok {
			continue
		}
		seen[member.key] = true
		result.set(member.key, reorderOrderedValue(value, member.value))
	}
	for _, member := range content.object {
		if seen[member.key] {
			continue
		}
		result.set(member.key, member.value)
	}
	return result
}

func mergeOrderedTrees(base, override *jsonValue) *jsonValue {
	if base == nil {
		return cloneOrderedTree(override)
	}
	if override == nil {
		return cloneOrderedTree(base)
	}
	if base.object == nil || override.object == nil {
		return cloneOrderedTree(override)
	}
	merged := cloneOrderedTree(base)
	for _, member := range override.object {
		current, ok := merged.get(member.key)
		if ok && current.object != nil && member.value.object != nil {
			merged.set(member.key, mergeOrderedTrees(current, member.value))
			continue
		}
		merged.set(member.key, cloneOrderedTree(member.value))
	}
	return merged
}

func cloneOrderedTree(value *jsonValue) *jsonValue {
	if value == nil {
		return nil
	}
	out := &jsonValue{raw: append(json.RawMessage(nil), value.raw...)}
	if value.object != nil {
		out.object = make([]jsonMember, 0, len(value.object))
		for _, member := range value.object {
			out.object = append(out.object, jsonMember{key: member.key, value: cloneOrderedTree(member.value)})
		}
	}
	if value.array != nil {
		out.array = make([]*jsonValue, 0, len(value.array))
		for _, item := range value.array {
			out.array = append(out.array, cloneOrderedTree(item))
		}
	}
	return out
}
