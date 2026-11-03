package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
)

type fakeLibrary struct {
	mu           sync.Mutex
	allocs       [][]byte
	registration string
	callback     uintptr
	hostReply    string
	calls        []string
}

func (f *fakeLibrary) keep(b []byte) uintptr {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allocs = append(f.allocs, b)
	return uintptr(unsafe.Pointer(&b[0]))
}

func (f *fakeLibrary) call(symbol string, args ...uintptr) (uintptr, error) {
	switch symbol {
	case InitSymbol:
		f.mu.Lock()
		f.callback = args[2]
		f.mu.Unlock()
		return f.keep(append([]byte(f.registration), 0)), nil
	case CallSymbol:
		request := readCString(args[0], int(args[1]))
		var invoke Invoke
		_ = json.Unmarshal([]byte(request), &invoke)
		f.mu.Lock()
		f.calls = append(f.calls, invoke.Type+":"+invoke.Name+invoke.Event)
		cb := f.callback
		reply := f.hostReply
		f.mu.Unlock()
		if cb != 0 && invoke.Type == "tool" {
			// Exercise the host-call bridge from inside a plugin call.
			hostReq := []byte(`{"id":7,"plugin":"fake","method":"getActiveTools"}`)
			out := make([]byte, 1<<16)
			var needed int32
			res, _, _ := purego.SyscallN(cb,
				uintptr(unsafe.Pointer(&hostReq[0])), uintptr(len(hostReq)),
				uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)),
				uintptr(unsafe.Pointer(&needed)))
			if res == 0 {
				return 0, os.ErrInvalid
			}
			_ = out[:needed]
			reply = `{"content":[{"type":"text","text":"echo"}],"isError":false}`
		}
		result, _ := json.Marshal(InvokeResult{Result: json.RawMessage(reply)})
		return f.keep(append([]byte(result), 0)), nil
	case FreeSymbol:
		return 1, nil
	}
	return 0, os.ErrInvalid
}

func (f *fakeLibrary) close() error { return nil }

func TestHostLoadsRegistrationAndCallsPlugin(t *testing.T) {
	registration := `{"apiVersion":1,"name":"fake","version":"1","tools":[{"name":"echo","description":"e","parameters":{"type":"object"}}],"commands":[{"name":"hello"}],"events":["tool_call"]}`
	fake := &fakeLibrary{registration: registration}
	old := openNativeLibrary
	openNativeLibrary = func(string) (nativeLibrary, error) { return fake, nil }
	defer func() { openNativeLibrary = old }()

	var hostMethods []string
	h := NewHost(HostOptions{
		Info: HostInfo{CWD: "C:/tmp"},
		Handler: func(ctx context.Context, method string, params json.RawMessage) (any, error) {
			hostMethods = append(hostMethods, method+"|"+NameFromContext(ctx))
			return []string{"echo"}, nil
		},
	})
	defer h.Close()
	path := filepath.Join(t.TempDir(), "fake.dll")
	instance, err := h.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if instance.Registration.Name != "fake" || len(instance.Registration.Tools) != 1 {
		t.Fatalf("registration: %+v", instance.Registration)
	}
	result, err := instance.ExecuteTool(context.Background(), "c1", "echo", map[string]any{"text": "hi"}, map[string]any{"cwd": "C:/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "echo") {
		t.Fatalf("tool result: %+v", result)
	}
	if len(hostMethods) != 1 || hostMethods[0] != "getActiveTools|fake" {
		t.Fatalf("host methods: %v", hostMethods)
	}
	if err := instance.ExecuteCommand(context.Background(), "hello", "x", nil); err != nil {
		t.Fatal(err)
	}
}

func TestRegistrationValidation(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"bad version", `{"apiVersion":2,"name":"x"}`, "API version"},
		{"no name", `{"apiVersion":1}`, "no name"},
		{"dup tool", `{"apiVersion":1,"name":"x","tools":[{"name":"a"},{"name":"a"}]}`, "duplicate tool"},
		{"bad event", `{"apiVersion":1,"name":"x","events":["nope"]}`, "unknown event"},
		{"bad exposure", `{"apiVersion":1,"name":"x","tools":[{"name":"a","exposure":"weird"}]}`, "unknown exposure"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeRegistration([]byte(tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestDiscoverManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "libfake.so"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	libName := "libfake.so"
	switch runtime.GOOS {
	case "windows":
		libName = "fake.dll"
		if err := os.WriteFile(filepath.Join(dir, libName), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	case "darwin":
		libName = "libfake.dylib"
		if err := os.WriteFile(filepath.Join(dir, libName), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := `{"name":"fake","version":"1","library":"` + libName + `"}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	libs, err := DiscoverPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 1 || filepath.Base(libs[0]) != libName {
		t.Fatalf("libs=%v", libs)
	}
}

func TestDiscoverProtocolManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := `{"name":"proto","version":"1","protocol":"stdio","command":"./proto-plugin.exe","args":["--stdio"]}`
	path := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	sources, err := Discover(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Kind != SourceProtocol || sources[0].Path != path {
		t.Fatalf("sources=%+v", sources)
	}
	badDir := filepath.Join(dir, "bad")
	if err := os.Mkdir(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(badDir, "plugin.json")
	if err := os.WriteFile(bad, []byte(`{"name":"x","protocol":"http","command":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(bad); err == nil || !strings.Contains(err.Error(), "unsupported plugin protocol") {
		t.Fatalf("err=%v", err)
	}
}
