package codemode

import (
	"context"
	"strings"
	"testing"
	"time"
)

// 1x1 PNG.
const pngData = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func run(t *testing.T, src string, opt RunOptions) Result {
	t.Helper()
	r, err := Run(context.Background(), src, opt)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return r
}

func TestTextAndConsoleLayout(t *testing.T) {
	r := run(t, `
		text("first");
		console.log("logged", {a: 1});
		text("second");
		return 42;`, RunOptions{})
	if !r.OK {
		t.Fatalf("not ok: %+v", r)
	}
	if !strings.Contains(r.Text, "==> text 1/3 <==") || !strings.Contains(r.Text, "==> text 3/3 <==") {
		t.Fatalf("each text item should be numbered: %q", r.Text)
	}
	if !strings.Contains(r.Text, "<console_output>\nlogged {\"a\":1}\n</console_output>") {
		t.Fatalf("console lines should be grouped last: %q", r.Text)
	}
	if !strings.Contains(r.Text, "==> text 3/3 <==\n42") {
		t.Fatalf("a returned value is output too: %q", r.Text)
	}
}

func TestImageOutput(t *testing.T) {
	r := run(t, `image("data:image/png;base64,`+pngData+`"); image({type:"image",data:"`+pngData+`"})`, RunOptions{})
	if !r.OK {
		t.Fatalf("not ok: %+v", r)
	}
	images := 0
	for _, block := range r.Content {
		if block.Type == "image" {
			images++
			if block.MIMEType != "image/png" || block.Data != pngData {
				t.Fatalf("bad image block: %+v", block)
			}
		}
	}
	if images != 2 {
		t.Fatalf("want 2 images, got %d", images)
	}
}

func TestImageRejectsNonImage(t *testing.T) {
	r := run(t, `image("data:text/plain;base64,aGVsbG8=")`, RunOptions{})
	if r.OK {
		t.Fatalf("a text payload should be rejected: %+v", r)
	}
	if !strings.Contains(r.Error, "PNG, JPEG, GIF, or WebP") {
		t.Fatalf("error should explain the accepted formats: %q", r.Error)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	r := run(t, `store("seen", {id: 7}); return load("seen")`, RunOptions{Store: map[string]any{"kept": "v"}})
	if !r.OK {
		t.Fatalf("not ok: %+v", r)
	}
	if !strings.Contains(r.Text, `{"id":7}`) {
		t.Fatalf("load should return what store wrote: %q", r.Text)
	}
	if !r.Wrote {
		t.Fatal("store() should mark the store dirty")
	}
	if r.Store["kept"] != "v" {
		t.Fatalf("existing keys should survive: %+v", r.Store)
	}
	if _, ok := r.Store["seen"]; !ok {
		t.Fatalf("new key should be saved: %+v", r.Store)
	}
}

func TestExitStopsScript(t *testing.T) {
	r := run(t, `text("before"); exit(); text("after")`, RunOptions{})
	if !r.OK {
		t.Fatalf("exit is a normal end: %+v", r)
	}
	if strings.Contains(r.Text, "after") {
		t.Fatalf("exit should stop the script: %q", r.Text)
	}
	if !strings.Contains(r.Text, "before") {
		t.Fatalf("output before exit should be kept: %q", r.Text)
	}
}

func TestUnknownToolSuggestsNames(t *testing.T) {
	opt := RunOptions{
		Tools: map[string]func(map[string]any) (string, error){
			"read_file": func(map[string]any) (string, error) { return "", nil },
		},
		ToolInfo: []ToolInfo{{Name: "read_file", Description: "Read a file"}},
	}
	r := run(t, `return tools.readFile({})`, opt)
	if r.OK {
		t.Fatalf("a missing tool should throw: %+v", r)
	}
	if !strings.Contains(r.Error, "tools.readFile does not exist") || !strings.Contains(r.Error, "Did you mean tools.read_file") {
		t.Fatalf("error should name the close match: %q", r.Error)
	}
}

func TestToolErrorSurfaces(t *testing.T) {
	opt := RunOptions{
		Tools: map[string]func(map[string]any) (string, error){
			"boom": func(map[string]any) (string, error) { return "", &testError{"kaput"} },
		},
		ToolInfo: []ToolInfo{{Name: "boom", Description: "Fails"}},
	}
	r := run(t, `await tools.boom({})`, opt)
	if r.OK {
		t.Fatalf("a failing tool should reject: %+v", r)
	}
	if !strings.Contains(r.Error, "kaput") {
		t.Fatalf("the message should reach the model: %q", r.Error)
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestDescribeToolReturnsSample(t *testing.T) {
	opt := RunOptions{
		Tools: map[string]func(map[string]any) (string, error){
			"read_file": func(map[string]any) (string, error) { return "", nil },
		},
		ToolInfo: []ToolInfo{{Name: "read_file", Description: "Read a file", Sample: "### `read_file`\nRead a file\n\ndeclare const tools: { read_file(args: { path: string }): Promise<string>; };"}},
	}
	r := run(t, `return [describeTool("read_file"), describeTool("nope")]`, opt)
	if !r.OK {
		t.Fatalf("not ok: %+v", r)
	}
	if !strings.Contains(r.Text, "declare const tools") || !strings.Contains(r.Text, "null") {
		t.Fatalf("describeTool should return the declaration and undefined for unknown: %q", r.Text)
	}
}

func TestOptionsBudgetIsReported(t *testing.T) {
	r := run(t, "// @options: {\"max_output_tokens\": 5, \"timeout_ms\": 1000}\nreturn 1", RunOptions{})
	if !r.OK {
		t.Fatalf("not ok: %+v", r)
	}
	if r.MaxOutputTokens != 5 {
		t.Fatalf("the script's budget should reach the caller: %d", r.MaxOutputTokens)
	}
}

func TestTimeoutStopsRunawayLoop(t *testing.T) {
	r, err := Run(context.Background(), "// @options: {\"timeout_ms\": 100}\nwhile (true) {}", RunOptions{}) //nolint:all
	if err == nil && r.OK {
		t.Fatalf("an endless loop should not finish: %+v", r)
	}
	t.Logf("err=%v r=%+v", err, r)
}

func TestMemoryLimitStopsScript(t *testing.T) {
	r := run(t, `var a = []; for (;;) { a.push(new Array(100000).fill(1)); }`, RunOptions{MemoryLimit: 8 << 20})
	if r.OK || !strings.Contains(r.Error, "memory limit") {
		t.Fatalf("a runaway allocation should hit the limit: %+v", r)
	}
}

func TestCancellationStopsScript(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	r, err := Run(ctx, `for (;;) {}`, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.OK || !strings.Contains(r.Error, "aborted") {
		t.Fatalf("cancelling the context should stop the script: %+v", r)
	}
}
