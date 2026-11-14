package main

import (
	"strings"
	"testing"
)

func TestAdvancedJSONRoundTrip(t *testing.T) {
	handled := jsonSet("name", "models")
	entry := map[string]any{
		"name":    "Acme",
		"models":  []any{map[string]any{"id": "one"}},
		"headers": map[string]any{"X-Test": "1"},
		"cost":    map[string]any{"input": float64(2)},
	}
	text := advancedJSON(entry, handled)
	if strings.Contains(text, `"name"`) || strings.Contains(text, `"models"`) {
		t.Fatalf("handled keys leaked into advanced JSON: %s", text)
	}
	updated := map[string]any{"name": "Acme", "stale": true}
	if err := applyAdvancedJSON(updated, handled, text); err != nil {
		t.Fatal(err)
	}
	if _, ok := updated["stale"]; ok {
		t.Fatalf("stale key was not removed: %#v", updated)
	}
	headers, _ := updated["headers"].(map[string]any)
	cost, _ := updated["cost"].(map[string]any)
	if headers["X-Test"] != "1" || cost["input"] != float64(2) || updated["name"] != "Acme" {
		t.Fatalf("updated entry = %#v", updated)
	}
}

func TestApplyAdvancedJSONRejectsInvalidShape(t *testing.T) {
	if err := applyAdvancedJSON(map[string]any{}, jsonSet(), "[]"); err == nil {
		t.Fatal("array should be rejected")
	}
	entry := map[string]any{"kept": "yes"}
	if err := applyAdvancedJSON(entry, jsonSet("kept"), `{"kept":"overridden","extra":true}`); err != nil {
		t.Fatal(err)
	}
	if entry["kept"] != "yes" || entry["extra"] != true {
		t.Fatalf("handled core keys must win: %#v", entry)
	}
}
