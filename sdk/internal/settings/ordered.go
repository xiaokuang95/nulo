package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// jsonValue is an ordered JSON tree. Settings writes patch the file's existing
// tree so untouched members and their member order survive, as Pi's
// `{ ...current, key: value }` spread does.
type jsonValue struct {
	object []jsonMember
	array  []*jsonValue
	raw    json.RawMessage
}

type jsonMember struct {
	key   string
	value *jsonValue
}

func parseJSONValue(data []byte) (*jsonValue, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if decoder.More() {
		return nil, fmt.Errorf("trailing JSON")
	}
	return value, nil
}

func decodeJSONValue(decoder *json.Decoder) (*jsonValue, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token := token.(type) {
	case json.Delim:
		switch token {
		case '{':
			value := &jsonValue{object: []jsonMember{}}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object key is not a string")
				}
				member, err := decodeJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				value.set(key, member)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return value, nil
		case '[':
			value := &jsonValue{array: []*jsonValue{}}
			for decoder.More() {
				item, err := decodeJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				value.array = append(value.array, item)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return value, nil
		}
	case json.Number:
		return &jsonValue{raw: json.RawMessage(normalizeJSONNumber(token.String()))}, nil
	case string:
		encoded, err := marshalJSONString(token)
		if err != nil {
			return nil, err
		}
		return &jsonValue{raw: encoded}, nil
	case bool:
		if token {
			return &jsonValue{raw: json.RawMessage("true")}, nil
		}
		return &jsonValue{raw: json.RawMessage("false")}, nil
	case nil:
		return &jsonValue{raw: json.RawMessage("null")}, nil
	}
	return nil, fmt.Errorf("unsupported JSON token %v", token)
}

func (v *jsonValue) get(key string) (*jsonValue, bool) {
	if v == nil || v.object == nil {
		return nil, false
	}
	for _, member := range v.object {
		if member.key == key {
			return member.value, true
		}
	}
	return nil, false
}

func (v *jsonValue) set(key string, value *jsonValue) {
	for i := range v.object {
		if v.object[i].key == key {
			v.object[i].value = value
			return
		}
	}
	v.object = append(v.object, jsonMember{key: key, value: value})
}

func (v *jsonValue) remove(key string) {
	if v == nil || v.object == nil {
		return
	}
	for i := range v.object {
		if v.object[i].key == key {
			v.object = append(v.object[:i], v.object[i+1:]...)
			return
		}
	}
}

func (v *jsonValue) MarshalJSON() ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	if v.object != nil {
		var buffer bytes.Buffer
		buffer.WriteByte('{')
		members := objectMembersInJSOrder(v.object)
		for i, member := range members {
			if i > 0 {
				buffer.WriteByte(',')
			}
			key, err := marshalJSONString(member.key)
			if err != nil {
				return nil, err
			}
			buffer.Write(key)
			buffer.WriteByte(':')
			encoded, err := member.value.MarshalJSON()
			if err != nil {
				return nil, err
			}
			buffer.Write(encoded)
		}
		buffer.WriteByte('}')
		return buffer.Bytes(), nil
	}
	if v.array != nil {
		var buffer bytes.Buffer
		buffer.WriteByte('[')
		for i, item := range v.array {
			if i > 0 {
				buffer.WriteByte(',')
			}
			encoded, err := item.MarshalJSON()
			if err != nil {
				return nil, err
			}
			buffer.Write(encoded)
		}
		buffer.WriteByte(']')
		return buffer.Bytes(), nil
	}
	if len(v.raw) == 0 {
		return []byte("null"), nil
	}
	return v.raw, nil
}

func marshalJSONString(value string) ([]byte, error) {
	return marshalJSONValue(value)
}

// marshalJSONValue matches JSON.stringify's escaping: HTML-sensitive
// characters and U+2028/U+2029 stay literal.
func marshalJSONValue(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	encoded := bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
	encoded = bytes.ReplaceAll(encoded, []byte(`\u2028`), []byte("\u2028"))
	encoded = bytes.ReplaceAll(encoded, []byte(`\u2029`), []byte("\u2029"))
	return encoded, nil
}

// objectMembersInJSOrder returns JSON.stringify's property order: integer
// index keys ascending, then the remaining keys in insertion order.
func objectMembersInJSOrder(members []jsonMember) []jsonMember {
	var indices []jsonMember
	var names []jsonMember
	for _, member := range members {
		if jsArrayIndex(member.key) {
			indices = append(indices, member)
		} else {
			names = append(names, member)
		}
	}
	sort.SliceStable(indices, func(i, j int) bool {
		left, _ := strconv.ParseUint(indices[i].key, 10, 64)
		right, _ := strconv.ParseUint(indices[j].key, 10, 64)
		return left < right
	})
	return append(indices, names...)
}

func jsArrayIndex(key string) bool {
	if key == "" || (len(key) > 1 && key[0] == '0') {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '0' || key[i] > '9' {
			return false
		}
	}
	value, err := strconv.ParseUint(key, 10, 64)
	return err == nil && value < 4294967295
}

// applyRawDiff updates tree with the differences between before and after.
// An object that exists in both layers is diffed recursively so its members
// keep their positions.
func applyRawDiff(tree *jsonValue, before, after map[string]json.RawMessage) {
	applyRawDiffOrdered(tree, before, after, nil)
}

func applyRawDiffOrdered(tree *jsonValue, before, after map[string]json.RawMessage, newKeys []string) {
	if tree == nil {
		return
	}
	processed := map[string]bool{}
	for _, key := range newKeys {
		if processed[key] {
			continue
		}
		if _, exists := after[key]; !exists {
			continue
		}
		if _, existed := before[key]; existed {
			continue
		}
		applyRawDiffKey(tree, key, before, after)
		processed[key] = true
	}
	keys := changedSettingKeys(before, after)
	for _, key := range keys {
		if processed[key] {
			continue
		}
		applyRawDiffKey(tree, key, before, after)
	}
}

func applyRawDiffKey(tree *jsonValue, key string, before, after map[string]json.RawMessage) {
	next, exists := after[key]
	previous, hadPrevious := before[key]
	if !exists {
		tree.remove(key)
		return
	}
	if hadPrevious && bytes.Equal(previous, next) {
		return
	}
	current, hasCurrent := tree.get(key)
	var beforeObject, afterObject map[string]json.RawMessage
	if hasCurrent && current.object != nil &&
		json.Unmarshal(previous, &beforeObject) == nil && beforeObject != nil &&
		json.Unmarshal(next, &afterObject) == nil && afterObject != nil {
		applyRawDiff(current, beforeObject, afterObject)
		return
	}
	parsed, err := parseJSONValue(next)
	if err != nil {
		parsed = &jsonValue{raw: append(json.RawMessage(nil), next...)}
	}
	tree.set(key, parsed)
}

// changedSettingKeys returns every key whose value differs, in a stable order:
// settings schema order for new known keys, then lexical order for unknown
// keys. Existing file order is preserved by set(), so this only controls key
// insertion points.
func changedSettingKeys(before, after map[string]json.RawMessage) []string {
	seen := map[string]bool{}
	var keys []string
	for key := range after {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range before {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left, right := settingsKeyRank(keys[i]), settingsKeyRank(keys[j])
		if left != right {
			return left < right
		}
		return keys[i] < keys[j]
	})
	return keys
}

var settingsKeyOrder = []string{
	"$schema", "lastChangelogVersion", "defaultProvider", "defaultModel",
	"defaultThinkingLevel", "transport", "sessionShareDestination", "steeringMode",
	"followUpMode", "tuiMode", "fullscreenExitOutput", "fullscreenScrollbar",
	"fullscreenCopyOnSelect", "fullscreenWheelScrollLines", "deviceId", "codemode",
	"maskSecretInput", "extensionsAutoUpgrade", "extensionsTypescriptRuntime",
	"theme", "compaction", "branchSummary", "retry", "hideThinkingBlock",
	"shellPath", "quietStartup", "shellCommandPrefix", "npmCommand",
	"collapseChangelog", "showCacheMissNotices", "enableInstallTelemetry",
	"enableAnalytics", "trackingId", "packages", "extensions", "skills",
	"prompts", "themes", "enableSkillCommands", "terminal", "images",
	"enabledModels", "defaultTools", "modelThinkingLevels", "doubleEscapeAction",
	"treeFilterMode", "defaultProjectTrust", "thinkingBudgets", "editorPaddingX",
	"outputPad", "externalEditor", "autocompleteMaxVisible", "showHardwareCursor",
	"markdown", "warnings", "sessionDir", "httpProxy",
	"websocketConnectTimeoutMs", "httpIdleTimeoutMs", "cacheWarming",
}

var settingsKeyRanks = func() map[string]int {
	ranks := make(map[string]int, len(settingsKeyOrder))
	for i, key := range settingsKeyOrder {
		ranks[key] = i
	}
	return ranks
}()

func settingsKeyRank(key string) int {
	if rank, ok := settingsKeyRanks[key]; ok {
		return rank
	}
	return len(settingsKeyOrder)
}

// writeSettingsPatch writes after onto the current file at path, retaining
// untouched members and their member order.
var settingsWriteMu sync.Mutex

func writeSettingsPatch(path string, before, after map[string]json.RawMessage) error {
	return writeSettingsPatchOrdered(path, before, after, nil)
}

func writeSettingsPatchOrdered(path string, before, after map[string]json.RawMessage, order []string) error {
	settingsWriteMu.Lock()
	defer settingsWriteMu.Unlock()
	if _, err := os.Stat(path + ".lock"); err == nil {
		return fmt.Errorf("%w: %s", ErrSettingsLocked, path)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	exists := readErr == nil
	pretty, err := patchedSettingsBytes(data, exists, before, after, order)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	if _, err = temp.Write(pretty); err == nil {
		err = temp.Chmod(0o644)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return nil
}

// patchedSettingsBytes applies a settings diff to an ordered JSON document.
func patchedSettingsBytes(data []byte, exists bool, before, after map[string]json.RawMessage, order []string) ([]byte, error) {
	tree := &jsonValue{object: []jsonMember{}}
	originalLength := len(data)
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if exists && originalLength > 0 && len(data) == 0 {
		return nil, fmt.Errorf("unexpected end of JSON input")
	}
	if len(bytes.TrimSpace(data)) > 0 {
		parsed, parseErr := parseJSONValue(data)
		if parseErr != nil {
			return nil, parseErr
		}
		if parsed.object != nil {
			tree = parsed
		} else if parsed.array != nil {
			// JSON.stringify({ ...array }) keeps numeric index keys.
			tree = &jsonValue{object: []jsonMember{}}
			for i, item := range parsed.array {
				tree.set(strconv.Itoa(i), item)
			}
		}
	}
	if raw, err := tree.MarshalJSON(); err == nil {
		var existing map[string]json.RawMessage
		if json.Unmarshal(raw, &existing) == nil && existing != nil {
			migrated := cloneRawMap(existing)
			migrationOrder := migrateSettingsOrdered(migrated)
			applyRawDiffOrdered(tree, existing, migrated, migrationOrder)
		}
	}
	applyRawDiffOrdered(tree, before, after, order)
	compact, err := tree.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact, "", "  "); err != nil {
		return nil, err
	}
	return pretty.Bytes(), nil
}

// normalizeJSONNumber writes a JSON number the way JavaScript's JSON.stringify
// writes it: integral values lose a trailing .0 and exponents are expanded
// when the value fits.
func normalizeJSONNumber(raw string) string {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return raw
	}
	if value == 0 {
		return "0"
	}
	absolute := math.Abs(value)
	if absolute >= 1e-6 && absolute < 1e21 {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	formatted := strconv.FormatFloat(value, 'e', -1, 64)
	// JSON.stringify omits exponent zero padding: 1e-07 -> 1e-7.
	if index := strings.IndexAny(formatted, "eE"); index >= 0 {
		mantissa, exponent := formatted[:index], formatted[index+1:]
		sign := ""
		if len(exponent) > 0 && (exponent[0] == '+' || exponent[0] == '-') {
			sign, exponent = exponent[:1], exponent[1:]
		}
		exponent = strings.TrimLeft(exponent, "0")
		if exponent == "" {
			exponent = "0"
		}
		formatted = mantissa + "e" + sign + exponent
	}
	return formatted
}
