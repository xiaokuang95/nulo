package codemode

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/MoeForge/moejs"
)

// preludeJS defines the globals a script runs with: tools, ALL_TOOLS,
// searchTools, text, image, console, store, load and exit.
//
//go:embed prelude.js
var preludeJS string

// defaultMemoryLimit caps what one script may keep and allocate. A script that
// passes it is interrupted and cannot catch the error, so a runaway string
// cannot take the host down.
const defaultMemoryLimit = 256 << 20

// compiledPrelude is parsed and compiled once; every run's runtime evaluates
// the same immutable script.
var compiledPrelude = sync.OnceValues(func() (*moejs.Script, error) {
	return moejs.CompileScript("codemode-prelude.js", preludeJS)
})

// jsVM is one JavaScript global environment. A run gets its own, so scripts
// share no globals and the runtime is cheap enough to rebuild per call.
type jsVM struct {
	rt *moejs.Runtime
	h  *Host
}

// memoryLimitBytes resolves the configured limit: zero means the default and a
// negative value means no limit.
func memoryLimitBytes(configured int64) int64 {
	switch {
	case configured < 0:
		return 0
	case configured == 0:
		return defaultMemoryLimit
	}
	return configured
}

func newJSVM(memoryLimit int64, h *Host) *jsVM {
	return &jsVM{rt: moejs.NewRuntime(moejs.Options{MemoryLimit: memoryLimitBytes(memoryLimit)}), h: h}
}

// installBridge exposes the host to scripts as `nulo_call`.
func (v *jsVM) installBridge() error {
	return v.rt.SetGlobal("nulo_call", moejs.NativeFunc(
		func(_ *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
			in := make([]string, 0, len(args))
			for i := range args {
				in = append(in, args[i].String())
			}
			return moejs.String(v.h.dispatch(in)), nil
		}))
}

func (v *jsVM) runPrelude() error {
	script, err := compiledPrelude()
	if err != nil {
		return err
	}
	_, err = v.rt.RunScript(script)
	return err
}

// eval compiles and runs one script. Jobs the script queued, promise reactions
// included, run before RunScript returns.
func (v *jsVM) eval(source string) error {
	script, err := moejs.CompileScript("<codemode>", source)
	if err != nil {
		return err
	}
	_, err = v.rt.RunScript(script)
	return err
}

// interrupt stops the running script from another goroutine, even one in an
// endless loop. reason reaches the run as the interrupt's error text.
func (v *jsVM) interrupt(reason string) { v.rt.Interrupt(reason) }

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
	case "done":
		h.mu.Lock()
		h.doneSeen = true
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

	// doneSeen reports whether the prelude reported an outcome, so a run that
	// silently never settled can be told apart from a clean one.
	doneSeen bool
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

// wrapScript builds the program evaluated for one run: the compiled prelude
// first, then the user's code inside an async function so top-level await
// works.
func wrapScript(user, toolsJSON, storeJSON, namespacesJSON string) string {
	var b strings.Builder
	b.WriteString("(function(){try{__nulo_init(")
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
