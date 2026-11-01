package tools

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestPowerShellIsWindowsOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("powershell resolves on Windows")
	}
	res, err := PowerShell(t.TempDir()).Execute(context.Background(), "1", map[string]any{"command": "Write-Output hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].Text, "only available on Windows") {
		t.Fatalf("%+v", res)
	}
}

func TestPowerShell(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("powershell is Windows-only")
	}
	ctx := context.Background()
	ps := PowerShell(t.TempDir())

	run := func(command string, args map[string]any) (string, bool) {
		t.Helper()
		if args == nil {
			args = map[string]any{}
		}
		args["command"] = command
		res, err := ps.Execute(ctx, "1", args)
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		return res.Content[0].Text, res.IsError
	}

	if text, isErr := run("Write-Output 'hi'", nil); isErr || !strings.Contains(text, "hi") {
		t.Fatalf("stdout %q isError=%v", text, isErr)
	}

	// The UTF-8 prefix keeps non-ASCII output intact.
	if text, isErr := run("Write-Output '中文'", nil); isErr || !strings.Contains(text, "中文") {
		t.Fatalf("utf8 %q isError=%v", text, isErr)
	}

	// A non-zero exit is an error result that names the code.
	text, isErr := run("exit 3", nil)
	if !isErr || !strings.Contains(text, "Command exited with code 3") {
		t.Fatalf("exit code %q isError=%v", text, isErr)
	}
	if !strings.Contains(text, "(no output)") {
		t.Fatalf("exit code should report empty output: %q", text)
	}

	if text, isErr := run("exit 0", nil); isErr || text != "(no output)" {
		t.Fatalf("silent command %q isError=%v", text, isErr)
	}

	if text, isErr := run("Write-Output 'pwd'", nil); isErr || !strings.Contains(text, "pwd") {
		t.Fatalf("%q isError=%v", text, isErr)
	}

	if text, isErr := run("Write-Output hi", map[string]any{"timeout": 0.0}); !isErr || !strings.Contains(text, "Invalid timeout") {
		t.Fatalf("invalid timeout %q isError=%v", text, isErr)
	}

	// A timeout kills the process tree and reports the caller's own number.
	if text, isErr := run("Start-Sleep -Seconds 30", map[string]any{"timeout": 1.0}); !isErr ||
		!strings.Contains(text, "Command timed out after 1 seconds") {
		t.Fatalf("timeout %q isError=%v", text, isErr)
	}

	// Output past the tail limit is truncated and the full text lands in a temp file.
	text, isErr = run("1..3000 | ForEach-Object { \"line $_\" }", nil)
	if isErr || !strings.Contains(text, "[Showing lines") {
		t.Fatalf("truncation %q isError=%v", text, isErr)
	}
	if !strings.Contains(text, "line 3000") {
		t.Fatalf("truncation should keep the tail: %q", text)
	}
}
