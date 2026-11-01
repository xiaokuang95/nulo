package codemode

import (
	"context"
	"testing"
)

func BenchmarkRun(b *testing.B) {
	fns := map[string]func(map[string]any) (string, error){
		"echo": func(a map[string]any) (string, error) { return "E:" + a["x"].(string), nil },
	}
	opt := RunOptions{
		Tools:    fns,
		ToolInfo: []ToolInfo{{Name: "echo", Description: "Echo a value back"}},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := Run(context.Background(), "return await tools.echo({x:'z'})", opt)
		if err != nil || !r.OK {
			b.Fatalf("run: %v %+v", err, r)
		}
	}
}
