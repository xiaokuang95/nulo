package sdk

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xiaokuang95/nulo/plugin"
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
	defer func() {
		if recovered := recover(); recovered != nil {
			reg := plugin.Registration{
				APIVersion: plugin.APIVersion,
				Name:       current.name,
				Version:    current.version,
				Error:      fmt.Sprint(recovered),
			}
			raw, _ := json.Marshal(reg)
			result = string(raw)
		}
	}()
	current.dispatchMu.Lock()
	defer current.dispatchMu.Unlock()
	current.hostCallPtr = hostCall
	_ = json.Unmarshal([]byte(hostInfo), &current.hostInfo)
	current.mu.Lock()
	factory := current.factory
	current.ready = false
	current.handlers = map[string][]Handler{}
	current.busHandlers = map[string][]func(any){}
	current.tools = map[string]*toolEntry{}
	current.commands = map[string]*commandEntry{}
	current.shortcuts = nil
	current.shortcutHandlers = map[string]func(*Context) error{}
	current.flags = map[string]plugin.FlagSpec{}
	current.flagValues = map[string]any{}
	current.renderers = nil
	current.mu.Unlock()
	api := newAPI(current)
	if factory != nil {
		factory(api)
	}
	current.mu.Lock()
	current.ready = true
	current.mu.Unlock()
	reg := api.registration()
	if reg.Name == "" {
		reg.Name = "unnamed"
	}
	if reg.Version == "" {
		reg.Version = current.version
	}
	current.mu.Lock()
	current.name = reg.Name
	if current.version == "" {
		current.version = reg.Version
	}
	current.mu.Unlock()
	raw, err := json.Marshal(reg)
	if err != nil {
		fallback, _ := json.Marshal(plugin.Registration{APIVersion: plugin.APIVersion, Name: reg.Name, Error: err.Error()})
		return string(fallback)
	}
	return string(raw)
}

// Call is the Go half of NuloPluginCall.
func Call(request string) (result string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			raw, _ := json.Marshal(plugin.InvokeResult{Error: fmt.Sprint(recovered)})
			result = string(raw)
		}
	}()
	// A handler may call back into the host, and the host may emit another
	// event before the first call returns. Reentrant calls therefore must not
	// take dispatchMu; Init still does to keep reloads atomic.
	var invoke plugin.Invoke
	if err := json.Unmarshal([]byte(request), &invoke); err != nil {
		raw, _ := json.Marshal(plugin.InvokeResult{Error: "invalid invoke: " + err.Error()})
		return string(raw)
	}
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
		raw, _ := json.Marshal(plugin.InvokeResult{Error: err.Error()})
		return string(raw)
	}
	raw, merr := json.Marshal(value)
	if merr != nil {
		raw, _ = json.Marshal(plugin.InvokeResult{Error: merr.Error()})
		return string(raw)
	}
	envelope, _ := json.Marshal(plugin.InvokeResult{Result: raw})
	return string(envelope)
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
