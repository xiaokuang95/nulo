package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// readJSONObject reads a standard JSON object and leaves comments to the raw
// editor. Missing and empty files become an empty object.
func readJSONObject(path string) (map[string]any, []byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, data, true, nil
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, nil, true, fmt.Errorf("%s uses comments or invalid JSON; use the raw editor", path)
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, data, true, nil
}

func writeJSONObject(path string, root map[string]any) error {
	if root == nil {
		root = map[string]any{}
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeConfigFile(path, data)
}

func jsonObjectField(root map[string]any, key string) map[string]any {
	if root == nil {
		return nil
	}
	value, _ := root[key].(map[string]any)
	return value
}

func jsonStringField(root map[string]any, key string) string {
	if root == nil {
		return ""
	}
	value, _ := root[key].(string)
	return value
}

func jsonBoolField(root map[string]any, key string, fallback bool) bool {
	if root == nil {
		return fallback
	}
	value, ok := root[key].(bool)
	if !ok {
		return fallback
	}
	return value
}

func jsonStringList(value any) []string {
	values, _ := value.([]any)
	out := make([]string, 0, len(values))
	for _, item := range values {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func parseKeyValueLines(text string, separator string) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, separator)
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("expected KEY%sVALUE: %s", separator, line)
		}
		out[key] = strings.TrimSpace(value)
	}
	return out, nil
}

func advancedJSON(entry map[string]any, handled map[string]bool) string {
	extras := map[string]any{}
	for key, value := range entry {
		if !handled[key] {
			extras[key] = value
		}
	}
	if len(extras) == 0 {
		return "{}"
	}
	data, err := json.MarshalIndent(extras, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func applyAdvancedJSON(entry map[string]any, handled map[string]bool, text string) error {
	text = strings.TrimSpace(text)
	extras := map[string]any{}
	if text != "" && text != "{}" {
		if err := json.Unmarshal([]byte(text), &extras); err != nil {
			return err
		}
		if extras == nil {
			extras = map[string]any{}
		}
	}
	for key := range entry {
		if !handled[key] {
			delete(entry, key)
		}
	}
	for key, value := range extras {
		if !handled[key] {
			entry[key] = value
		}
	}
	return nil
}

func jsonSet(handled ...string) map[string]bool {
	out := make(map[string]bool, len(handled))
	for _, key := range handled {
		out[key] = true
	}
	return out
}
