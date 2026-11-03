package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

func cloneRawMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

func mergeSettingsLayers(global, project, overrides map[string]json.RawMessage) map[string]json.RawMessage {
	merged := cloneRawMap(global)
	mergeLayerInto(merged, project)
	mergeLayerInto(merged, overrides)
	return merged
}

func mergeLayerInto(merged, layer map[string]json.RawMessage) {
	for key, value := range layer {
		if key == "defaultTools" {
			merged[key] = mergeDefaultToolsRaw(merged[key], value)
			continue
		}
		trimmed := bytes.TrimSpace(value)
		if isKnownSettingKey(key) {
			// Pi's typed merge ignores a null for most known settings. Theme,
			// quietStartup and the raw timeout values keep an authored null.
			if bytes.Equal(trimmed, []byte("null")) {
				if nullOverrideKeys[key] {
					merged[key] = value
				}
				continue
			}
			// Most known string settings treat an empty project value as
			// unset; only presence-tracked strings override to empty.
			if len(trimmed) >= 2 && trimmed[0] == '"' && !emptyStringOverrides[key] {
				var text string
				if json.Unmarshal(trimmed, &text) == nil && text == "" {
					continue
				}
			}
		}
		switch key {
		case "terminal":
			merged[key] = mergeTerminalRaw(merged[key], value)
		case "markdown":
			merged[key] = mergeMarkdownRaw(merged[key], value)
		case "compaction":
			merged[key] = mergeCompactionRaw(merged[key], value)
		default:
			if isKnownSettingKey(key) && key != "theme" {
				merged[key] = mergeKnownRaw(merged[key], value)
			} else {
				merged[key] = deepMergeRaw(merged[key], value)
			}
		}
	}
}

// mergeTerminalRaw deep-merges terminal, but Pi's raw capability keys replace
// a lower layer even when the project authors null.
func mergeTerminalRaw(base, override json.RawMessage) json.RawMessage {
	trimmedOverride := bytes.TrimSpace(override)
	if bytes.Equal(trimmedOverride, []byte("null")) {
		return mergeKnownRaw(base, override)
	}
	var baseObject, overrideObject map[string]json.RawMessage
	if json.Unmarshal(base, &baseObject) != nil || json.Unmarshal(override, &overrideObject) != nil ||
		baseObject == nil || overrideObject == nil {
		return override
	}
	merged := cloneRawMap(baseObject)
	for key, value := range overrideObject {
		switch key {
		case "hyperlinks", "images", "trueColor":
			merged[key] = value
		default:
			merged[key] = mergeKnownRaw(merged[key], value)
		}
	}
	encoded, err := marshalJSONValue(merged)
	if err != nil {
		return override
	}
	return encoded
}

// mergeMarkdownRaw deep-merges markdown; an authored null codeBlockIndent
// replaces the lower layer and reads as the default.
func mergeMarkdownRaw(base, override json.RawMessage) json.RawMessage {
	trimmedOverride := bytes.TrimSpace(override)
	if bytes.Equal(trimmedOverride, []byte("null")) {
		return mergeKnownRaw(base, override)
	}
	var baseObject, overrideObject map[string]json.RawMessage
	if json.Unmarshal(base, &baseObject) != nil || json.Unmarshal(override, &overrideObject) != nil ||
		baseObject == nil || overrideObject == nil {
		return override
	}
	merged := cloneRawMap(baseObject)
	for key, value := range overrideObject {
		if key == "codeBlockIndent" {
			merged[key] = value
			continue
		}
		merged[key] = mergeKnownRaw(merged[key], value)
	}
	encoded, err := marshalJSONValue(merged)
	if err != nil {
		return override
	}
	return encoded
}

// mergeCompactionRaw deep-merges compaction, where an authored null token
// setting replaces the lower layer and is rejected by the resolver.
func mergeCompactionRaw(base, override json.RawMessage) json.RawMessage {
	trimmedOverride := bytes.TrimSpace(override)
	if bytes.Equal(trimmedOverride, []byte("null")) {
		return mergeKnownRaw(base, override)
	}
	var baseObject, overrideObject map[string]json.RawMessage
	if json.Unmarshal(base, &baseObject) != nil || json.Unmarshal(override, &overrideObject) != nil ||
		baseObject == nil || overrideObject == nil {
		return override
	}
	merged := cloneRawMap(baseObject)
	for key, value := range overrideObject {
		switch key {
		case "reserveTokens", "keepRecentTokens":
			merged[key] = value
		case "modelOverrides":
			merged[key] = mergeCompactionOverridesRaw(merged[key], value)
		default:
			merged[key] = mergeKnownRaw(merged[key], value)
		}
	}
	encoded, err := marshalJSONValue(merged)
	if err != nil {
		return override
	}
	return encoded
}

func mergeCompactionOverridesRaw(base, override json.RawMessage) json.RawMessage {
	var baseObject, overrideObject map[string]json.RawMessage
	if json.Unmarshal(base, &baseObject) != nil || json.Unmarshal(override, &overrideObject) != nil ||
		baseObject == nil || overrideObject == nil {
		return mergeKnownRaw(base, override)
	}
	merged := cloneRawMap(baseObject)
	for key, value := range overrideObject {
		var baseEntry, overrideEntry map[string]json.RawMessage
		if json.Unmarshal(merged[key], &baseEntry) != nil || json.Unmarshal(value, &overrideEntry) != nil ||
			baseEntry == nil || overrideEntry == nil {
			merged[key] = value
			continue
		}
		entry := cloneRawMap(baseEntry)
		for field, fieldValue := range overrideEntry {
			if field == "reserveTokens" || field == "keepRecentTokens" {
				entry[field] = fieldValue
				continue
			}
			entry[field] = mergeKnownRaw(entry[field], fieldValue)
		}
		encoded, err := marshalJSONValue(entry)
		if err != nil {
			merged[key] = value
			continue
		}
		merged[key] = encoded
	}
	encoded, err := marshalJSONValue(merged)
	if err != nil {
		return override
	}
	return encoded
}

// mergeKnownRaw deep-merges a known settings object, treating a null member as
// unset when the lower layer has a value. Pi's typed merge ignores null for
// every known optional field; unknown top-level settings keep the raw rule.
func mergeKnownRaw(base, override json.RawMessage) json.RawMessage {
	trimmedOverride := bytes.TrimSpace(override)
	if bytes.Equal(trimmedOverride, []byte("null")) {
		if len(bytes.TrimSpace(base)) == 0 || bytes.Equal(bytes.TrimSpace(base), []byte("null")) {
			return override
		}
		return base
	}
	var baseObject, overrideObject map[string]json.RawMessage
	if json.Unmarshal(base, &baseObject) != nil || json.Unmarshal(override, &overrideObject) != nil ||
		baseObject == nil || overrideObject == nil {
		return override
	}
	merged := cloneRawMap(baseObject)
	for key, value := range overrideObject {
		merged[key] = mergeKnownRaw(merged[key], value)
	}
	encoded, err := marshalJSONValue(merged)
	if err != nil {
		return override
	}
	return encoded
}

var nullOverrideKeys = map[string]bool{
	"theme":                      true,
	"quietStartup":               true,
	"httpIdleTimeoutMs":          true,
	"websocketConnectTimeoutMs":  true,
	"fullscreenWheelScrollLines": true,
}

var emptyStringOverrides = map[string]bool{
	"defaultProvider": true,
	"defaultModel":    true,
	"theme":           true,
	"shellPath":       true,
	// The internal shellCommandPrefix key and any legacy alias.
	"shellCommandPrefix": true,
	"commandPrefix":      true,
	"externalEditor":     true,
}

var knownSettingKeys = func() map[string]bool {
	keys := map[string]bool{}
	for _, key := range settingKeys() {
		keys[key] = true
	}
	return keys
}()

func isKnownSettingKey(key string) bool { return knownSettingKeys[key] }

func mergeDefaultToolsRaw(base, override json.RawMessage) json.RawMessage {
	if len(override) == 0 {
		return base
	}
	var baseTools, overrideTools []string
	if !jsonArrayOrNull(base, &baseTools) || !jsonArrayOrNull(override, &overrideTools) {
		return override
	}
	encoded, err := marshalJSONValue(MergeDefaultTools(baseTools, overrideTools))
	if err != nil {
		return override
	}
	return encoded
}

func jsonArrayOrNull(raw json.RawMessage, out *[]string) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		*out = nil
		return true
	}
	if bytes.Equal(trimmed, []byte("null")) {
		*out = nil
		return true
	}
	if err := json.Unmarshal(trimmed, out); err != nil {
		return false
	}
	return true
}

// deepMergeRaw implements Pi's deepMergeObjects: two objects merge member by
// member, and every other value, arrays included, replaces the base.
func deepMergeRaw(base, override json.RawMessage) json.RawMessage {
	var baseObject, overrideObject map[string]json.RawMessage
	if json.Unmarshal(base, &baseObject) != nil || json.Unmarshal(override, &overrideObject) != nil ||
		baseObject == nil || overrideObject == nil {
		return override
	}
	merged := cloneRawMap(baseObject)
	for key, value := range overrideObject {
		merged[key] = deepMergeRaw(merged[key], value)
	}
	encoded, err := marshalJSONValue(merged)
	if err != nil {
		return override
	}
	return encoded
}

func decodeRawString(raw json.RawMessage) (string, bool) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func decodeRawBool(raw json.RawMessage) (bool, bool) {
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false
	}
	return value, true
}

// objectForSpread mirrors JavaScript's { ...value }: objects keep their
// members, arrays and strings spread into numeric keys, and scalars spread to
// nothing. Pi's nested setters use it even when the file holds an odd shape.
func objectForSpread(raw json.RawMessage) map[string]json.RawMessage {
	if object, ok := decodeRawObject(raw); ok {
		return object
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]json.RawMessage{}
	}
	object := map[string]json.RawMessage{}
	switch trimmed[0] {
	case '[':
		var items []json.RawMessage
		if json.Unmarshal(trimmed, &items) == nil {
			for i, item := range items {
				object[strconv.Itoa(i)] = item
			}
		}
	case '"':
		var text string
		if json.Unmarshal(trimmed, &text) == nil {
			for i, r := range []rune(text) {
				encoded, _ := marshalJSONValue(string(r))
				object[strconv.Itoa(i)] = encoded
			}
		}
	}
	return object
}

func decodeRawObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return map[string]json.RawMessage{}, true
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, false
	}
	return value, true
}

func decodeRawStrings(raw json.RawMessage) ([]string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, true
	}
	var value []string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, false
	}
	return value, true
}

// decodeRawInt reads a JavaScript number as an integer. A fractional, negative
// or non-finite value is an error, as Pi's validated token settings are.
func decodeRawInt(raw json.RawMessage) (int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, fmt.Errorf("missing number")
	}
	number, err := strconv.ParseFloat(string(trimmed), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number != math.Trunc(number) {
		return 0, fmt.Errorf("expected a non-negative integer")
	}
	if number > float64(1<<53-1) || number > float64(math.MaxInt) {
		return 0, fmt.Errorf("integer is too large")
	}
	return int(number), nil
}

func decodeRawNumber(raw json.RawMessage) (float64, bool) {
	number, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func rawMember(object map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	value, ok := object[name]
	if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, false
	}
	return value, true
}

func parseSettingsMap(data []byte) (map[string]json.RawMessage, error) {
	if len(data) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("unexpected end of JSON input")
	}
	switch trimmed[0] {
	case '[':
		// Pi spreads an array root into an object with numeric keys before
		// saving; keep those members so the first write matches that shape.
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		object := make(map[string]json.RawMessage, len(items))
		for i, item := range items {
			object[strconv.Itoa(i)] = item
		}
		return object, nil
	case '{':
	default:
		return nil, fmt.Errorf("settings must be a JSON object")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return map[string]json.RawMessage{}, nil
	}
	migrateSettingsOrdered(object)
	return object, nil
}

// migrateSettings applies Pi's legacy-settings migrations.
func migrateSettings(object map[string]json.RawMessage) {
	migrateSettingsOrdered(object)
}

// migrateSettingsOrdered applies the migrations and returns the new
// top-level keys in execution order, which is how Pi appends them.
func migrateSettingsOrdered(object map[string]json.RawMessage) []string {
	var newKeys []string
	if queue, ok := object["queueMode"]; ok {
		if _, exists := object["steeringMode"]; !exists {
			object["steeringMode"] = queue
			newKeys = append(newKeys, "steeringMode")
			delete(object, "queueMode")
		}
	}
	if _, exists := object["transport"]; !exists {
		if raw, ok := object["websockets"]; ok {
			if value, ok := decodeRawBool(raw); ok {
				if value {
					object["transport"] = json.RawMessage(`"websocket"`)
				} else {
					object["transport"] = json.RawMessage(`"sse"`)
				}
				newKeys = append(newKeys, "transport")
				delete(object, "websockets")
			}
		}
	}
	if raw, ok := object["skills"]; ok && isJSONObjectRaw(raw) {
		if skills, ok := decodeRawObject(raw); ok && skills != nil {
			if enable, present := skills["enableSkillCommands"]; present {
				if _, exists := object["enableSkillCommands"]; !exists {
					object["enableSkillCommands"] = enable
					newKeys = append(newKeys, "enableSkillCommands")
				}
			}
			if directories, present := skills["customDirectories"]; present {
				if list, valid := decodeRawStrings(directories); valid && len(list) > 0 {
					object["skills"] = directories
				} else {
					delete(object, "skills")
				}
			} else {
				delete(object, "skills")
			}
		}
	}
	if raw, ok := object["retry"]; ok && isJSONObjectRaw(raw) {
		if retry, valid := decodeRawObject(raw); valid && retry != nil {
			if maxDelay, present := retry["maxDelayMs"]; present {
				trimmed := bytes.TrimSpace(maxDelay)
				numeric := len(trimmed) > 0 && (trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9'))
				if numeric {
					provider, valid := decodeRawObject(retry["provider"])
					if !valid || provider == nil {
						provider = map[string]json.RawMessage{}
						if rawProvider, ok := retry["provider"]; ok && isJSONArray(rawProvider) {
							var items []json.RawMessage
							if json.Unmarshal(rawProvider, &items) == nil {
								for i, item := range items {
									provider[strconv.Itoa(i)] = item
								}
							}
						}
					}
					current := bytes.TrimSpace(provider["maxRetryDelayMs"])
					if len(current) == 0 || bytes.Equal(current, []byte("null")) {
						provider["maxRetryDelayMs"] = maxDelay
					}
					encoded, err := marshalJSONValue(provider)
					if err == nil {
						retry["provider"] = encoded
					}
				}
			}
			delete(retry, "maxDelayMs")
			encoded, err := marshalJSONValue(retry)
			if err == nil {
				object["retry"] = encoded
			}
		}
	}
	return newKeys
}

func isJSONObjectRaw(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func isJSONArray(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '['
}

// ErrSettingsLocked reports Pi's proper-lockfile writer lock.
var ErrSettingsLocked = errors.New("ELOCKED: settings lockfile is held")

func readSettingsFile(path string) (map[string]json.RawMessage, error) {
	if _, err := os.Stat(path + ".lock"); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrSettingsLocked, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, err
	}
	originalLength := len(data)
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if originalLength > 0 && len(data) == 0 {
		return nil, fmt.Errorf("unexpected end of JSON input")
	}
	return parseSettingsMap(data)
}
