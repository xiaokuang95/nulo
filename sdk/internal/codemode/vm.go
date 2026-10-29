package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const evalAsync = 128

type jsVM struct {
	ctx context.Context
	rt  wazero.Runtime
	mod api.Module
	mem api.Memory
	mu  sync.Mutex
	h   *Host
}

func newJSVM(ctx context.Context, h *Host) (*jsVM, error) {
	rt := wazero.NewRuntime(ctx)
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
	case "store":
		if len(args) > 2 {
			h.mu.Lock()
			if args[2] == "undefined" || args[2] == "" {
				delete(h.store, args[1])
			} else {
				var v any
				_ = json.Unmarshal([]byte(args[2]), &v)
				h.store[args[1]] = v
			}
			h.wrote = true
			h.mu.Unlock()
		}
		return "null"
	case "load":
		h.mu.Lock()
		v := h.store[args[1]]
		h.mu.Unlock()
		b, _ := json.Marshal(v)
		return string(b)
	case "output":
		if len(args) > 2 {
			h.mu.Lock()
			h.out = append(h.out, args[2])
			h.mu.Unlock()
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
	out         []string
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

func wrapScript(user string, toolNames []string, descriptions map[string]string) string {
	var all []string
	for _, n := range toolNames {
		d := descriptions[n]
		all = append(all, fmt.Sprintf("{name:%q,description:%q}", n, d))
	}
	var toolAssign strings.Builder
	for _, n := range toolNames {
		js := toIdent(n)
		toolAssign.WriteString(fmt.Sprintf("tools[%q]=tools[%q]=function(args){var r=JSON.parse(nulo_call('call',%q,JSON.stringify(args||{})));if(r.error)throw new Error(r.error);return r.ok;};\n", n, js, n))
	}
	return fmt.Sprintf(`
(function(){
  "use strict";
  var tools = {};
  %s
  function text(v){ nulo_call("output","text", typeof v==="string"?v:JSON.stringify(v)); }
  function image(v){ nulo_call("output","image", typeof v==="string"?v:JSON.stringify(v)); }
  var console = {log:function(){ nulo_call("output","console", Array.prototype.join.call(arguments," ")); },
    info:function(){ console.log.apply(console, arguments); },
    warn:function(){ console.log.apply(console, arguments); },
    error:function(){ console.log.apply(console, arguments); },
    debug:function(){ console.log.apply(console, arguments); } };
  function store(k,v){ nulo_call("store", String(k), v===undefined?"undefined":JSON.stringify(v)); }
  function load(k){ var s=nulo_call("load", String(k)); return s==="null"?undefined:JSON.parse(s); }
  function exit(){ throw {__nulo_exit:true}; }
  var ALL_TOOLS = [%s];
  function searchTools(q){ q=String(q||"").toLowerCase(); return ALL_TOOLS.filter(function(t){return t.name.toLowerCase().indexOf(q)>=0||(t.description||"").toLowerCase().indexOf(q)>=0;}).slice(0,8); }
  function describeTool(n){ for(var i=0;i<ALL_TOOLS.length;i++) if(ALL_TOOLS[i].name===n) return ALL_TOOLS[i].description; }
  function describeNamespace(){ return undefined; }
  var models = { list:function(){return [];} };
  globalThis.tools = tools;
  globalThis.text = text;
  globalThis.image = image;
  globalThis.console = console;
  globalThis.store = store;
  globalThis.load = load;
  globalThis.exit = exit;
  globalThis.ALL_TOOLS = ALL_TOOLS;
  globalThis.searchTools = searchTools;
  globalThis.describeTool = describeTool;
  globalThis.describeNamespace = describeNamespace;
  globalThis.models = models;
  var __out = [];
  var _text = text;
  globalThis.text = function(v){ var s=typeof v==="string"?v:JSON.stringify(v); __out.push(s); _text(v); };
  globalThis.__nulo_run = async function(){
    var ret;
    try { ret = await (async function(){ %s
    })(); if(ret!==undefined && ret!==null) globalThis.text(ret); nulo_call("done","1", JSON.stringify({text:__out.join("\n")})); }
    catch(e){ if(e&&e.__nulo_exit){ nulo_call("done","1", JSON.stringify({text:__out.join("\n")})); return; } nulo_call("done","0", String(e && e.message ? e.message : e)); throw e; }
  };
  return globalThis.__nulo_run();
})()
`, toolAssign.String(), strings.Join(all, ","), user)
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
