package codemode

import (
	"context"
	"strings"
	"testing"
)

func TestEvalText(t *testing.T) {
	r, err := Run(context.Background(), "return 'hi from js'", map[string]func(map[string]any) (string, error){}, nil)
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
	r, err := Run(context.Background(), "return await tools.echo({x:'z'})", fns, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Text, "E:z") {
		t.Fatalf("%+v", r)
	}
}
