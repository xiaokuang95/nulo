package codemode

import (
	"context"
	"strings"
	"testing"
)

func TestEvalText(t *testing.T) {
	r, err := Run(context.Background(), "return 'hi from js'", RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Text, "hi from js") && !r.OK && r.Error == "" {
		t.Fatalf("%+v", r)
	}
	t.Logf("%+v", r)
}

func TestCallTool(t *testing.T) {
	fns := map[string]func(map[string]any) (string, error){
		"echo": func(a map[string]any) (string, error) {
			s, _ := a["x"].(string)
			return "E:" + s, nil
		},
	}
	r, err := Run(context.Background(), "return await tools.echo({x:'z'})", RunOptions{
		Tools: fns,
		ToolInfo: []ToolInfo{
			{Name: "echo", Description: "Echo a value back"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Text, "E:z") {
		t.Fatalf("%+v", r)
	}
}

func TestSearchToolsReportsDescriptions(t *testing.T) {
	r, err := Run(context.Background(), `
		return {
			byDescription: searchTools("mcp").map(t => t.name),
			limited: searchTools("mcp", {limit: 1}).length,
		};`, RunOptions{
		Tools: map[string]func(map[string]any) (string, error){
			"nulo__alpha": func(map[string]any) (string, error) { return "a", nil },
			"nulo__beta":  func(map[string]any) (string, error) { return "b", nil },
		},
		ToolInfo: []ToolInfo{
			{Name: "nulo__alpha", Description: "An MCP server tool", Namespace: "mcp__srv"},
			{Name: "nulo__beta", Description: "Another one", Namespace: "mcp__srv"},
		},
		Namespaces: []Namespace{{Name: "mcp__srv", Description: "Srv", Tools: []string{"nulo__alpha", "nulo__beta"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Text, "nulo__alpha") {
		t.Fatalf("searchTools should match on the description: %+v", r)
	}
	if !strings.Contains(r.Text, `"limited":1`) {
		t.Fatalf("searchTools should honour the limit option: %+v", r)
	}
}

func TestDescribeNamespaceAliases(t *testing.T) {
	// describeNamespace accepts mcp__-prefixed, bare, dashed and underscored forms.
	r, err := Run(context.Background(), `
		return ["mcp__dev-radius", "mcp__dev_radius", "dev-radius", "dev_radius", "nope"]
			.map(n => describeNamespace(n) === undefined ? null : describeNamespace(n).name);`, RunOptions{
		Tools:      map[string]func(map[string]any) (string, error){},
		Namespaces: []Namespace{{Name: "mcp__dev-radius", Description: "Radius", Instructions: "Use it", Tools: []string{"mcp__dev-radius__search"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(r.Text, "mcp__dev-radius") != 4 {
		t.Fatalf("aliases should all resolve: %+v", r)
	}
	if !strings.Contains(r.Text, "null") {
		t.Fatalf("an unknown namespace should be undefined: %+v", r)
	}
}
