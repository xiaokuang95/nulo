package codemode

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const evalAsync = 128

// preludeJS defines the globals a script runs with: tools, ALL_TOOLS,
// searchTools, text, image, console, store, load and exit.
//
//go:embed prelude.js
var preludeJS string

type jsVM struct {
	ctx context.Context
	rt  wazero.Runtime
	mod api.Module
	mem api.Memory
	mu  sync.Mutex
	h   *Host
}

func newJSVM(ctx context.Context, h *Host) (*jsVM, error) {
	// WithCloseOnContextDone lets the script's deadline abort a runaway loop: the
	// runtime closes the module and the call in flight returns.
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(true))
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		rt.Close(ctx)
		return nil, err
	}
	v := &jsVM{ctx: ctx, rt: rt, h: h}
	env := rt.NewHostModuleBuilder("env")
	env.NewFunctionBuilder().WithFunc(v.hostCall).Export("host_call")
	env.NewFunctionBuilder().WithFunc(func() uint32 { return 0 }).Export("host_interrupt")
	env.NewFunctionBuilder().WithFunc(func(uint32, uint32, uint32) {}).Export("host_promise_rejection")
	env.NewFunctionBuilder().WithFunc(func(uint32, uint32) uint32 { return 0 }).Export("host_module_normalize")
	env.NewFunctionBuilder().WithFunc(func(uint32, uint32) uint32 { return 0 }).Export("host_module_load")
	env.NewFunctionBuilder().WithFunc(func(int32, int32) int32 { return 0 }).Export("host_get_timezone_offset")
	if _, err := env.Instantiate(ctx); err != nil {
		rt.Close(ctx)
		return nil, err
	}
	mod, err := rt.Instantiate(ctx, quickjsWasm)
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}
	v.mod = mod
	v.mem = mod.Memory()
	if err := v.callVoid("_initialize"); err != nil {
		v.Close()
		return nil, err
	}
	if _, err := v.call("qjs_init"); err != nil {
		v.Close()
		return nil, err
	}
	return v, nil
}

func (v *jsVM) Close() {
	if v.rt != nil {
		_ = v.rt.Close(v.ctx)
	}
}

func (v *jsVM) call(name string, args ...uint64) (uint64, error) {
	fn := v.mod.ExportedFunction(name)
	if fn == nil {
		return 0, fmt.Errorf("missing export %s", name)
	}
	res, err := fn.Call(v.ctx, args...)
	if err != nil {
		return 0, err
	}
	if len(res) == 0 {
		return 0, nil
	}
	return res[0], nil
}

func (v *jsVM) callVoid(name string, args ...uint64) error {
	fn := v.mod.ExportedFunction(name)
	if fn == nil {
		return fmt.Errorf("missing export %s", name)
	}
	_, err := fn.Call(v.ctx, args...)
	return err
}

func (v *jsVM) malloc(n uint32) (uint32, error) {
	p, err := v.call("wasm_malloc", uint64(n))
	return uint32(p), err
}

func (v *jsVM) writeCString(s string) (ptr uint32, n uint32, err error) {
	b := append([]byte(s), 0)
	p, err := v.malloc(uint32(len(b)))
	if err != nil {
		return 0, 0, err
	}
	if !v.mem.Write(p, b) {
		return 0, 0, fmt.Errorf("write mem")
	}
	return p, uint32(len(s)), nil
}

func (v *jsVM) eval(code, filename string, flags uint64) (uint32, error) {
	cp, clen, err := v.writeCString(code)
	if err != nil {
		return 0, err
	}
	fp, _, err := v.writeCString(filename)
	if err != nil {
		return 0, err
	}
	res, err := v.call("qjs_eval", uint64(cp), uint64(clen), uint64(fp), flags)
	_ = v.callVoid("wasm_free", uint64(cp))
	_ = v.callVoid("wasm_free", uint64(fp))
	return uint32(res), err
}

func (v *jsVM) isException(ptr uint32) bool {
	r, _ := v.call("qjs_is_exception", uint64(ptr))
	return r != 0
}

func (v *jsVM) toString(ptr uint32) string {
	sp, err := v.call("qjs_get_string", uint64(ptr))
	if err != nil || sp == 0 {
		return ""
	}
	return v.readCString(uint32(sp))
}

func (v *jsVM) readCString(ptr uint32) string {
	if ptr == 0 {
		return ""
	}
	var b []byte
	for i := uint32(0); i < 1<<20; i++ {
		by, ok := v.mem.ReadByte(ptr + i)
		if !ok || by == 0 {
			break
		}
		b = append(b, by)
	}
	return string(b)
}

func (v *jsVM) hostCall(namePtr, nameLen, thisPtr, argc, argvPtr uint32) uint32 {
	name, _ := v.mem.Read(namePtr, nameLen)
	if string(name) != "nulo_call" {
		return 0
	}
	args := make([]string, 0, argc)
	for i := uint32(0); i < argc; i++ {
		p, _ := v.mem.ReadUint32Le(argvPtr + i*4)
		args = append(args, v.toString(p))
	}
	out := v.h.dispatch(args)
	sp, sl, err := v.writeCString(out)
	if err != nil {
		return 0
	}
	js, err := v.call("qjs_new_string", uint64(sp), uint64(sl))
	_ = v.callVoid("wasm_free", uint64(sp))
	if err != nil {
		return 0
	}
	return uint32(js)
}

func (v *jsVM) installBridge() error {
	np, nl, err := v.writeCString("nulo_call")
	if err != nil {
		return err
	}
	fn, err := v.call("qjs_new_host_function", uint64(np), uint64(nl), 0)
	_ = v.callVoid("wasm_free", uint64(np))
	if err != nil {
		return err
	}
	g, err := v.call("qjs_get_global")
	if err != nil {
		return err
	}
	gn, _, err := v.writeCString("nulo_call")
	if err != nil {
		return err
	}
	_, err = v.call("qjs_set_prop_string", g, uint64(gn), fn)
	_ = v.callVoid("wasm_free", uint64(gn))
	return err
}

func (v *jsVM) drainJobs() {
	for i := 0; i < 10000; i++ {
		pending, _ := v.call("qjs_is_job_pending")
		if pending == 0 {
			return
		}
		_, _ = v.call("qjs_execute_pending_job")
	}
}

func (h *Host) dispatch(args []string) string {
	if len(args) == 0 {
		return `{"error":"no args"}`
	}
	kind := args[0]
	switch kind {
	case "call":
		name := ""
		raw := "{}"
		if len(args) > 1 {
			name = args[1]
		}
		if len(args) > 2 {
			raw = args[2]
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(raw), &m)
		if m == nil {
			m = map[string]any{}
		}
		res, err := h.callTool(name, m)
		if err != nil {
			b, _ := json.Marshal(map[string]any{"error": err.Error()})
			return string(b)
		}
		b, _ := json.Marshal(map[string]any{"ok": res})
		return string(b)
	case "output":
		if len(args) > 2 {
			item := OutputItem{Kind: OutputKind(args[1]), Text: args[2]}
			if item.Kind == OutputImage {
				item.Data = args[2]
				item.Text = ""
				if len(args) > 3 {
					item.MIME = args[3]
				}
			}
			h.mu.Lock()
			h.out = append(h.out, item)
			h.mu.Unlock()
		}
		return "null"
	case "search":
		if len(args) > 1 {
			return h.search(args[1])
		}
		return "[]"
	case "writes":
		// The prelude batches store() changes: one entry per key, [key] for a
		// delete and [key, json] for a write.
		if len(args) > 1 {
			var entries [][]json.RawMessage
			if json.Unmarshal([]byte(args[1]), &entries) == nil {
				h.mu.Lock()
				for _, entry := range entries {
					if len(entry) == 0 {
						continue
					}
					var key string
					if json.Unmarshal(entry[0], &key) != nil {
						continue
					}
					if len(entry) == 1 {
						delete(h.store, key)
					} else {
						var value any
						if json.Unmarshal(entry[1], &value) == nil {
							h.store[key] = value
						}
					}
					h.wrote = true
				}
				h.mu.Unlock()
			}
		}
		return "null"
	case "done":
		h.mu.Lock()
		h.doneOK = len(args) > 1 && args[1] == "1"
		if len(args) > 2 {
			h.donePayload = args[2]
		}
		h.mu.Unlock()
		return "null"
	default:
		return "null"
	}
}

type Host struct {
	mu          sync.Mutex
	store       map[string]any
	wrote       bool
	tools       map[string]func(map[string]any) (string, error)
	info        map[string]ToolInfo
	namespaces  []Namespace
	out         []OutputItem
	doneOK      bool
	donePayload string
}

func (h *Host) callTool(name string, args map[string]any) (string, error) {
	h.mu.Lock()
	fn := h.tools[name]
	h.mu.Unlock()
	if fn == nil {
		return "", fmt.Errorf("unknown tool %s", name)
	}
	return fn(args)
}

// wrapScript builds the program evaluated for one run: the prelude, then the
// user's code inside an async function so top-level await works.
func wrapScript(user, toolsJSON, storeJSON, namespacesJSON string) string {
	var b strings.Builder
	b.WriteString(preludeJS)
	b.WriteString("\n(function(){try{__nulo_init(")
	b.WriteString(jsString(toolsJSON))
	b.WriteString(",")
	b.WriteString(jsString(storeJSON))
	b.WriteString(",")
	b.WriteString(jsString(namespacesJSON))
	b.WriteString(");}catch(e){__nulo_fail(e);return;}(async function(){\n")
	b.WriteString(user)
	b.WriteString("\n}).call(globalThis).then(__nulo_succeed,__nulo_fail);})()")
	return b.String()
}

// jsString quotes a JSON document as a JavaScript string literal. JSON escapes
// leave `<`, `>` and `&` alone, and the prelude never evaluates HTML, so plain
// quoting is enough.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func toIdent(n string) string {
	var b strings.Builder
	for _, r := range n {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
