package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

// HostCallHandler executes one plugin-initiated API call and returns a value
// that can be marshalled to JSON. A nil Handler answers every call with an
// error, which lets a plugin load in a host that only wants registrations.
type HostCallHandler func(ctx context.Context, method string, params json.RawMessage) (any, error)

// Diagnostic is a non-fatal plugin loading or runtime problem.
type Diagnostic struct {
	Path    string
	Plugin  string
	Message string
}

func (d Diagnostic) Error() string {
	if d.Plugin != "" {
		return d.Plugin + ": " + d.Message
	}
	if d.Path != "" {
		return d.Path + ": " + d.Message
	}
	return d.Message
}

// HostOptions configures a plugin host.
type HostOptions struct {
	Info         HostInfo
	Handler      HostCallHandler
	OnDiagnostic func(Diagnostic)
}

// Instance is one successfully loaded native plugin.
type Instance struct {
	Path         string
	Registration Registration
	lib          nativeLibrary
	host         *Host
	mu           sync.Mutex
	closed       bool
}

// Host loads native plugins and dispatches calls to them. It is safe for
// concurrent use. A plugin library is loaded at most once per Host.
type Host struct {
	mu       sync.RWMutex
	info     HostInfo
	handler  HostCallHandler
	onDiag   func(Diagnostic)
	callback uintptr
	byPath   map[string]*Instance
	order    []*Instance
	closed   bool
}

// NewHost creates an empty host. The callback pointer passed to plugins is
// created on first use and remains valid for the lifetime of the process.
func NewHost(opts HostOptions) *Host {
	h := &Host{
		info:    opts.Info,
		handler: opts.Handler,
		onDiag:  opts.OnDiagnostic,
		byPath:  map[string]*Instance{},
	}
	return h
}

// SetInfo updates the snapshot returned to plugins loaded later.
func (h *Host) SetInfo(info HostInfo) {
	h.mu.Lock()
	h.info = info
	h.mu.Unlock()
}

// SetHandler replaces the host-call handler. Existing plugins use it for their
// next call, which is how a host binds a plugin to a newly created session.
func (h *Host) SetHandler(fn HostCallHandler) {
	h.mu.Lock()
	h.handler = fn
	h.mu.Unlock()
}

// Instances returns the loaded plugins in load order.
func (h *Host) Instances() []*Instance {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]*Instance(nil), h.order...)
}

// Load opens one shared library and calls its Init entry point.
func (h *Host) Load(path string) (*Instance, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, fmt.Errorf("plugin host is closed")
	}
	if existing := h.byPath[abs]; existing != nil {
		h.mu.Unlock()
		return existing, nil
	}
	if h.callback == 0 {
		h.callback = h.newCallback()
	}
	info := h.info
	if info.APIVersion == 0 {
		info.APIVersion = APIVersion
	}
	if info.Host == "" {
		info.Host = "nulo"
	}
	cb := h.callback
	h.mu.Unlock()

	lib, err := openNativeLibrary(abs)
	if err != nil {
		return nil, err
	}
	inst := &Instance{Path: abs, host: h, lib: lib}
	raw, err := inst.init(info, cb)
	if err != nil {
		// A c-shared Go library starts a runtime in its init path. Unloading it
		// after that is unsafe on Windows and several Unix linkers, so a failed
		// load is leaked rather than risking the host process.
		return nil, err
	}
	reg, err := DecodeRegistration(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", abs, err)
	}
	inst.Registration = reg

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, fmt.Errorf("plugin host is closed")
	}
	if existing := h.byPath[abs]; existing != nil {
		h.mu.Unlock()
		return existing, nil
	}
	h.byPath[abs] = inst
	h.order = append(h.order, inst)
	h.mu.Unlock()
	return inst, nil
}

func (i *Instance) init(info HostInfo, callback uintptr) ([]byte, error) {
	rawInfo, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	infoPtr, infoLen := stringData(string(rawInfo))
	res, err := i.lib.call(InitSymbol, infoPtr, uintptr(infoLen), callback)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", InitSymbol, err)
	}
	if res == 0 {
		return nil, fmt.Errorf("%s returned no registration", InitSymbol)
	}
	text := readCString(res, 64<<20)
	i.free(res)
	return []byte(text), nil
}

// Call sends one Invoke to the plugin and decodes its result.
func (i *Instance) Call(ctx context.Context, invoke Invoke) (json.RawMessage, error) {
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return nil, fmt.Errorf("plugin %s is closed", i.Registration.Name)
	}
	i.mu.Unlock()
	raw, err := json.Marshal(invoke)
	if err != nil {
		return nil, err
	}
	ptr, n := stringData(string(raw))
	res, err := i.lib.call(CallSymbol, ptr, uintptr(n))
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", i.Registration.Name, err)
	}
	if res == 0 {
		return nil, fmt.Errorf("plugin %s returned no result", i.Registration.Name)
	}
	text := readCString(res, 64<<20)
	i.free(res)
	decoded, err := DecodeInvokeResult([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", i.Registration.Name, err)
	}
	if decoded.Error != "" {
		return nil, fmt.Errorf("plugin %s: %s", i.Registration.Name, decoded.Error)
	}
	if len(decoded.Result) == 0 {
		return json.RawMessage("null"), nil
	}
	return decoded.Result, nil
}

// Emit sends an event to the plugin if it subscribed to it.
func (i *Instance) Emit(ctx context.Context, event string, data any, ctxData any) (json.RawMessage, bool, error) {
	if !hasEvent(i.Registration.Events, event) {
		return nil, false, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, true, err
	}
	rawCtx, err := json.Marshal(ctxData)
	if err != nil {
		return nil, true, err
	}
	out, err := i.Call(ctx, Invoke{Type: "event", Event: event, Data: raw, Context: rawCtx})
	return out, true, err
}

func hasEvent(events []string, event string) bool {
	for _, name := range events {
		if name == event {
			return true
		}
	}
	return false
}

// ExecuteTool calls a plugin tool.
func (i *Instance) ExecuteTool(ctx context.Context, callID, name string, args map[string]any, ctxData any) (ToolResult, error) {
	rawArgs, err := json.Marshal(args)
	if err != nil {
		return ToolResult{}, err
	}
	rawCtx, err := json.Marshal(ctxData)
	if err != nil {
		return ToolResult{}, err
	}
	out, err := i.Call(ctx, Invoke{Type: "tool", Name: name, ToolCallID: callID, Input: rawArgs, Context: rawCtx})
	if err != nil {
		return ToolResult{}, err
	}
	var result ToolResult
	if err := json.Unmarshal(out, &result); err != nil {
		return ToolResult{}, fmt.Errorf("plugin %s: tool %s result: %w", i.Registration.Name, name, err)
	}
	return result, nil
}

// ExecuteCommand calls a plugin command with its raw argument string.
// ExecuteShortcut calls a shortcut handler registered by the plugin.
func (i *Instance) ExecuteShortcut(ctx context.Context, key string, ctxData any) error {
	rawCtx, err := json.Marshal(ctxData)
	if err != nil {
		return err
	}
	_, err = i.Call(ctx, Invoke{Type: "shortcut", Name: key, Context: rawCtx})
	return err
}

func (i *Instance) ExecuteCommand(ctx context.Context, name, args string, ctxData any) error {
	rawArgs, err := json.Marshal(args)
	if err != nil {
		return err
	}
	rawCtx, err := json.Marshal(ctxData)
	if err != nil {
		return err
	}
	_, err = i.Call(ctx, Invoke{Type: "command", Name: name, Input: rawArgs, Context: rawCtx})
	return err
}

func (i *Instance) free(ptr uintptr) {
	if ptr == 0 {
		return
	}
	_, _ = i.lib.call(FreeSymbol, ptr)
}

func (h *Host) newCallback() uintptr {
	return purego.NewCallback(func(_ purego.CDecl, req uintptr, reqLen uintptr, out uintptr, outCap uintptr, outLen uintptr) uintptr {
		if reqLen > 64<<20 {
			return 0
		}
		request := append([]byte(nil), bytesAt(req, int(reqLen))...)
		response := h.answer(request)
		if outLen != 0 {
			*(*int32)(ptrToUnsafe(outLen)) = int32(len(response))
		}
		if out == 0 || outCap == 0 || int(outCap) < len(response) {
			return 0
		}
		copy(bytesAt(out, len(response)), response)
		return 1
	})
}

type pluginNameKey struct{}

// NameFromContext returns the plugin that made the current host call.
func NameFromContext(ctx context.Context) string {
	name, _ := ctx.Value(pluginNameKey{}).(string)
	return name
}

func (h *Host) answer(request []byte) []byte {
	var call HostCall
	if err := json.Unmarshal(request, &call); err != nil {
		raw, _ := json.Marshal(HostReply{Error: &APIError{Message: "invalid host call: " + err.Error()}})
		return raw
	}
	h.mu.RLock()
	handler := h.handler
	h.mu.RUnlock()
	var result any
	var apiErr *APIError
	if handler == nil {
		apiErr = &APIError{Message: "host call " + call.Method + " is unavailable"}
	} else {
		ctx := context.WithValue(context.Background(), pluginNameKey{}, call.Plugin)
		value, err := handler(ctx, call.Method, call.Params)
		if err != nil {
			apiErr = &APIError{Message: err.Error()}
		} else {
			result = value
		}
	}
	reply := HostReply{ID: call.ID, Result: result, Error: apiErr}
	raw, err := json.Marshal(reply)
	if err != nil {
		raw, _ = json.Marshal(HostReply{ID: call.ID, Error: &APIError{Message: err.Error()}})
	}
	if len(raw) > 8<<20 {
		raw, _ = json.Marshal(HostReply{ID: call.ID, Error: &APIError{Message: "host response is larger than the plugin bridge allows"}})
	}
	return raw
}

// Close unloads every plugin. Unloading a Go shared library that started its
// own runtime is not always safe on every platform, so failures are reported
// as diagnostics and ignored; the process is still free to exit.
func (h *Host) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	instances := append([]*Instance(nil), h.order...)
	h.order = nil
	h.byPath = map[string]*Instance{}
	h.mu.Unlock()
	for _, inst := range instances {
		inst.mu.Lock()
		inst.closed = true
		inst.mu.Unlock()
		// Do not dlclose/FreeLibrary a library that may contain a Go runtime.
		// The OS reclaims it at process exit.
	}
	return nil
}

func (h *Host) diagnose(d Diagnostic) {
	if h.onDiag != nil {
		h.onDiag(d)
	}
}

// Manifest is the optional plugin.json beside a shared library. A manifest is
// how a plugin gives the loader a stable name before the library is loaded.
type Manifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	APIVersion  int               `json:"apiVersion,omitempty"`
	Library     string            `json:"library,omitempty"`
	Platforms   map[string]string `json:"platforms,omitempty"`
}

// DiscoverPath turns a file, a manifest, or a directory into library paths.
// A directory is scanned one level deep in lexical order. Directories with a
// plugin.json use its library/platform field; otherwise every shared library in
// the directory is loaded.
func DiscoverPath(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Base(path), "plugin.json") {
			return discoverManifest(path)
		}
		if isSharedLibrary(path) {
			return []string{path}, nil
		}
		return nil, fmt.Errorf("%s is not a shared library or plugin.json", path)
	}
	return discoverDir(path)
}

func discoverDir(dir string) ([]string, error) {
	if manifest := filepath.Join(dir, "plugin.json"); pathIsFile(manifest) {
		return discoverManifest(manifest)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		full := filepath.Join(dir, name)
		if entry.IsDir() {
			manifest := filepath.Join(full, "plugin.json")
			if _, err := os.Stat(manifest); err == nil {
				libs, err := discoverManifest(manifest)
				if err != nil {
					return nil, err
				}
				out = append(out, libs...)
				continue
			}
			libs, err := discoverDir(full)
			if err != nil {
				return nil, err
			}
			out = append(out, libs...)
			continue
		}
		if strings.EqualFold(name, "plugin.json") {
			libs, err := discoverManifest(full)
			if err != nil {
				return nil, err
			}
			out = append(out, libs...)
			continue
		}
		if isSharedLibrary(name) {
			out = append(out, full)
		}
	}
	return out, nil
}

func discoverManifest(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if manifest.APIVersion != 0 && manifest.APIVersion != APIVersion {
		return nil, fmt.Errorf("%s: plugin API version %d is not supported", path, manifest.APIVersion)
	}
	dir := filepath.Dir(path)
	library := manifest.Library
	if platform := platformKey(); platform != "" {
		if specific := manifest.Platforms[platform]; specific != "" {
			library = specific
		}
	}
	if library == "" {
		libs, err := discoverDirLibraries(dir)
		if err != nil {
			return nil, err
		}
		if len(libs) == 0 {
			return nil, fmt.Errorf("%s: no shared library found", path)
		}
		return libs, nil
	}
	full := library
	if !filepath.IsAbs(full) {
		full = filepath.Join(dir, library)
	}
	if !isSharedLibrary(full) {
		return nil, fmt.Errorf("%s: %s is not a shared library", path, library)
	}
	return []string{full}, nil
}

func discoverDirLibraries(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if isSharedLibrary(entry.Name()) {
			out = append(out, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func pathIsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isSharedLibrary(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".dll", ".so", ".dylib":
		return true
	}
	// Versioned Unix libraries: libfoo.so.1
	return strings.Contains(strings.ToLower(filepath.Base(path)), ".so.")
}

func platformKey() string { return runtime.GOOS }

// AddTool records a tool registered after Init. The next call to Call can
// execute it, and the host's session adds it to the catalogue.
func (i *Instance) AddTool(spec ToolSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("plugin %s: tool has no name", i.Registration.Name)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, existing := range i.Registration.Tools {
		if existing.Name == spec.Name {
			return fmt.Errorf("plugin %s: duplicate tool %q", i.Registration.Name, spec.Name)
		}
	}
	if len(spec.Parameters) == 0 {
		spec.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	if spec.ExecutionMode == "" {
		spec.ExecutionMode = "parallel"
	}
	i.Registration.Tools = append(i.Registration.Tools, spec)
	return nil
}

// AddEvent records an event registered after Init, so later emits reach it.
func (i *Instance) AddEvent(event string) error {
	if !IsEvent(event) {
		return fmt.Errorf("plugin %s: unknown event %q", i.Registration.Name, event)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, existing := range i.Registration.Events {
		if existing == event {
			return nil
		}
	}
	i.Registration.Events = append(i.Registration.Events, event)
	sort.Strings(i.Registration.Events)
	return nil
}

// AddCommand records a command registered after Init.
func (i *Instance) AddCommand(spec CommandSpec) error {
	if spec.Name == "" {
		return fmt.Errorf("plugin %s: command has no name", i.Registration.Name)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, existing := range i.Registration.Commands {
		if existing.Name == spec.Name {
			return fmt.Errorf("plugin %s: duplicate command %q", i.Registration.Name, spec.Name)
		}
	}
	i.Registration.Commands = append(i.Registration.Commands, spec)
	return nil
}

// AddFlag records a flag registered after Init.
func (i *Instance) AddFlag(spec FlagSpec) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, existing := range i.Registration.Flags {
		if existing.Name == spec.Name {
			return fmt.Errorf("plugin %s: duplicate flag %q", i.Registration.Name, spec.Name)
		}
	}
	i.Registration.Flags = append(i.Registration.Flags, spec)
	return nil
}

// AddShortcut records a shortcut registered after Init.
func (i *Instance) AddShortcut(spec ShortcutSpec) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.Registration.Shortcuts = append(i.Registration.Shortcuts, spec)
	return nil
}

// AddRenderer records a renderer registered after Init.
func (i *Instance) AddRenderer(spec RendererSpec) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.Registration.Renderers = append(i.Registration.Renderers, spec)
	return nil
}

// Snapshot returns a copy of the current registration. Registrations can grow
// after Init, so callers must not read the public field concurrently.
func (i *Instance) Snapshot() Registration {
	i.mu.Lock()
	defer i.mu.Unlock()
	reg := i.Registration
	reg.Tools = append([]ToolSpec(nil), reg.Tools...)
	reg.Commands = append([]CommandSpec(nil), reg.Commands...)
	reg.Flags = append([]FlagSpec(nil), reg.Flags...)
	reg.Shortcuts = append([]ShortcutSpec(nil), reg.Shortcuts...)
	reg.Renderers = append([]RendererSpec(nil), reg.Renderers...)
	reg.Events = append([]string(nil), reg.Events...)
	return reg
}
