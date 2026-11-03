package sdk

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xiaokuang95/nulo/sdk/internal/plugin"
)

type server struct {
	mu               sync.Mutex
	dispatchMu       sync.Mutex
	hostMu           sync.Mutex
	name             string
	version          string
	factory          Factory
	started          bool
	ready            bool
	hostCallPtr      uintptr
	hostCallFn       func([]byte) ([]byte, error)
	hostInfo         map[string]any
	handlers         map[string][]Handler
	busHandlers      map[string][]func(any)
	tools            map[string]*toolEntry
	commands         map[string]*commandEntry
	shortcuts        []plugin.ShortcutSpec
	shortcutHandlers map[string]func(*Context) error
	flags            map[string]plugin.FlagSpec
	flagValues       map[string]any
	renderers        []plugin.RendererSpec
}

var current = &server{
	handlers:         map[string][]Handler{},
	busHandlers:      map[string][]func(any){},
	tools:            map[string]*toolEntry{},
	commands:         map[string]*commandEntry{},
	shortcutHandlers: map[string]func(*Context) error{},
	flags:            map[string]plugin.FlagSpec{},
	flagValues:       map[string]any{},
}

func (s *server) serve(factory Factory) { s.serveNamed("", "", factory) }

func (s *server) serveNamed(name, version string, factory Factory) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		panic("plugin: Serve may be called only once")
	}
	s.started = true
	s.factory = factory
	s.name = name
	s.version = version
	s.mu.Unlock()
}

// Init is the Go half of NuloPluginInit. Plugin main packages wrap it with the
// cgo export; see the package documentation.
func Init(hostInfo string, hostCall uintptr) (result string) {
	reg := current.initialize(hostInfo, hostCall, nil)
	raw, err := json.Marshal(reg)
	if err != nil {
		fallback, _ := json.Marshal(plugin.Registration{APIVersion: plugin.APIVersion, Name: reg.Name, Error: err.Error()})
		return string(fallback)
	}
	return string(raw)
}

// initialize resets the plugin and runs its factory. It is shared by the C ABI
// and the stdio protocol runtime. hostCallFn is nil for the C ABI.
func (s *server) initialize(hostInfo string, hostCall uintptr, hostCallFn func([]byte) ([]byte, error)) (reg plugin.Registration) {
	defer func() {
		if recovered := recover(); recovered != nil {
			reg = plugin.Registration{
				APIVersion: plugin.APIVersion,
				Name:       s.name,
				Version:    s.version,
				Error:      fmt.Sprint(recovered),
			}
		}
	}()
	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	s.hostCallPtr = hostCall
	s.hostCallFn = hostCallFn
	_ = json.Unmarshal([]byte(hostInfo), &s.hostInfo)
	s.mu.Lock()
	factory := s.factory
	s.ready = false
	s.handlers = map[string][]Handler{}
	s.busHandlers = map[string][]func(any){}
	s.tools = map[string]*toolEntry{}
	s.commands = map[string]*commandEntry{}
	s.shortcuts = nil
	s.shortcutHandlers = map[string]func(*Context) error{}
	s.flags = map[string]plugin.FlagSpec{}
	s.flagValues = map[string]any{}
	s.renderers = nil
	s.mu.Unlock()
	api := newAPI(s)
	if factory != nil {
		factory(api)
	}
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	reg = api.registration()
	if reg.Name == "" {
		reg.Name = "unnamed"
	}
	if reg.Version == "" {
		reg.Version = s.version
	}
	s.mu.Lock()
	s.name = reg.Name
	if s.version == "" {
		s.version = reg.Version
	}
	s.mu.Unlock()
	return reg
}

// Call is the Go half of NuloPluginCall.
func Call(request string) (result string) {
	var invoke plugin.Invoke
	if err := json.Unmarshal([]byte(request), &invoke); err != nil {
		raw, _ := json.Marshal(plugin.InvokeResult{Error: "invalid invoke: " + err.Error()})
		return string(raw)
	}
	raw, _ := json.Marshal(dispatchInvoke(invoke))
	return string(raw)
}

func dispatchInvoke(invoke plugin.Invoke) (result plugin.InvokeResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = plugin.InvokeResult{Error: fmt.Sprint(recovered)}
		}
	}()
	var value any
	var err error
	switch invoke.Type {
	case "event":
		value, err = current.event(invoke)
	case "tool":
		value, err = current.tool(invoke)
	case "command":
		value, err = current.command(invoke)
	case "shortcut":
		value, err = current.shortcut(invoke)
	case "host":
		value, err = current.hostInvoke(invoke)
	default:
		err = fmt.Errorf("unknown invoke type %q", invoke.Type)
	}
	if err != nil {
		return plugin.InvokeResult{Error: err.Error()}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return plugin.InvokeResult{Error: err.Error()}
	}
	return plugin.InvokeResult{Result: raw}
}

func (s *server) event(invoke plugin.Invoke) (any, error) {
	s.mu.Lock()
	handlers := append([]Handler(nil), s.handlers[invoke.Event]...)
	s.mu.Unlock()
	if len(handlers) == 0 {
		return nil, nil
	}
	event := Event{}
	if len(invoke.Data) > 0 {
		if err := json.Unmarshal(invoke.Data, &event); err != nil {
			return nil, fmt.Errorf("event %s data: %w", invoke.Event, err)
		}
	}
	ctx := contextFrom(invoke.Context)
	combined := map[string]any{}
	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		result, err := handler(ctx, event)
		if err != nil {
			return nil, fmt.Errorf("event %s: %w", invoke.Event, err)
		}
		if result == nil {
			continue
		}
		patch := result
		if _, ok := patch.(map[string]any); !ok {
			raw, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("event %s result: %w", invoke.Event, err)
			}
			var object map[string]any
			if err := json.Unmarshal(raw, &object); err == nil {
				patch = object
			} else if invoke.Event == "before_provider_request" {
				combined["payload"] = result
				continue
			} else {
				continue
			}
		}
		for k, v := range patch.(map[string]any) {
			combined[k] = v
		}
	}
	return map[string]any{"event": event, "result": combined}, nil
}

func (s *server) tool(invoke plugin.Invoke) (any, error) {
	s.mu.Lock()
	entry := s.tools[invoke.Name]
	s.mu.Unlock()
	if entry == nil || entry.execute == nil {
		return nil, fmt.Errorf("tool %q is not registered", invoke.Name)
	}
	args := map[string]any{}
	if len(invoke.Input) > 0 {
		if err := json.Unmarshal(invoke.Input, &args); err != nil {
			return nil, fmt.Errorf("tool %s input: %w", invoke.Name, err)
		}
	}
	result, err := entry.execute(contextFrom(invoke.Context), invoke.ToolCallID, args)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *server) command(invoke plugin.Invoke) (any, error) {
	s.mu.Lock()
	entry := s.commands[invoke.Name]
	s.mu.Unlock()
	if entry == nil || entry.handler == nil {
		return nil, fmt.Errorf("command %q is not registered", invoke.Name)
	}
	args := ""
	if len(invoke.Input) > 0 {
		_ = json.Unmarshal(invoke.Input, &args)
	}
	return nil, entry.handler(commandContextFrom(invoke.Context), args)
}

func (s *server) shortcut(invoke plugin.Invoke) (any, error) {
	s.mu.Lock()
	handler := s.shortcutHandlers[invoke.Name]
	s.mu.Unlock()
	if handler == nil {
		return nil, fmt.Errorf("shortcut %q is not registered", invoke.Name)
	}
	return nil, handler(contextFrom(invoke.Context))
}

func (s *server) hostInvoke(invoke plugin.Invoke) (any, error) {
	switch invoke.Name {
	case "events.deliver":
		var payload struct {
			Event string `json:"event"`
			Data  any    `json:"data"`
		}
		if err := json.Unmarshal(invoke.Input, &payload); err != nil {
			return nil, err
		}
		s.mu.Lock()
		handlers := append([]func(any){}, s.busHandlers[payload.Event]...)
		s.mu.Unlock()
		for _, handler := range handlers {
			if handler != nil {
				handler(payload.Data)
			}
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown host invoke %q", invoke.Name)
	}
}
