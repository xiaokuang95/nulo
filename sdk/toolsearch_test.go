package sdk

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolSearchToolShape(t *testing.T) {
	sess := &Session{}
	tool := sess.toolSearchTool()
	if tool.Name != "tool_search" {
		t.Fatalf("%q", tool.Name)
	}
	if !strings.Contains(tool.Description, "BM25") {
		t.Fatalf("%q", tool.Description)
	}
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "query" {
		t.Fatalf("%+v", schema)
	}
	if schema.Properties["limit"].Description == "" {
		t.Fatalf("%+v", schema.Properties)
	}
}
