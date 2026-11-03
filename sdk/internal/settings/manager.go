package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
)

// ProjectDir returns the project configuration directory nulo reads.
func ProjectDir(cwd string) string {
	return filepath.Join(cwd, ".nulo")
}

// Error is one settings load or write failure.
type Error struct {
	Scope string
	Path  string
	Err   error
}

func (e Error) Error() string {
	if e.Path == "" {
		return e.Scope + " settings: " + e.Err.Error()
	}
	return e.Scope + " settings (" + e.Path + "): " + e.Err.Error()
}

func (e Error) Unwrap() error { return e.Err }

// Options are the options of New.
type Options struct {
	// ProjectTrusted is whether project settings are read and written. Nil is
	// true, as Pi's SettingsManager.create defaults.
	ProjectTrusted *bool
}

// SettingsStorage is Pi's SettingsStorage: the backend of
// SettingsManager.fromStorage. WithLock runs fn on the current text of the
// scope's settings ("global" or "project", nil when there is none); fn returns
// the text to store, nil to leave it, or an error that WithLock returns.
type SettingsStorage interface {
	WithLock(scope string, fn func(current *string) (*string, error)) error
}

// Manager is Pi's SettingsManager: global and project settings layers and
// their merged view.
type Manager struct {
	mu sync.RWMutex

	cwd      string
	agentDir string
	trusted  bool

	global  map[string]json.RawMessage
	project map[string]json.RawMessage
	merged  map[string]json.RawMessage

	globalSettings Settings
	mergedSettings Settings

	// warningsValue backs the pointer GetWarnings returns without allocating.
	warningsValue bool
	// overrides are non-persistent settings applied on top of the merged view.
	overrides map[string]json.RawMessage

	globalErr  error
	projectErr error
	errors     []Error

	memory        bool
	memoryGlobal  map[string]json.RawMessage
	memoryProject map[string]json.RawMessage
	storage       SettingsStorage

	observers    map[uint64]func()
	nextObserver uint64

	generation    uint64
	extensionJSON json.RawMessage
	extensionGen  uint64
}

// New loads the settings of agentDir/settings.json and, when trusted,
// cwd/.nulo/settings.json.
func New(cwd, agentDir string) *Manager {
	return NewWithOptions(cwd, agentDir, Options{})
}

// NewWithOptions is New with options.
func NewWithOptions(cwd, agentDir string, options Options) *Manager {
	trusted := options.ProjectTrusted == nil || *options.ProjectTrusted
	m := &Manager{
		cwd:      cwd,
		agentDir: agentDir,
		trusted:  trusted,
		global:   map[string]json.RawMessage{},
		project:  map[string]json.RawMessage{},
	}
	m.reload()
	return m
}

// NewSettingsManagerFromStorage is upstream's SettingsManager.fromStorage.
func NewSettingsManagerFromStorage(storage SettingsStorage, options SettingsManagerCreateOptions) *SettingsManager {
	trusted := options.ProjectTrusted == nil || *options.ProjectTrusted
	m := &Manager{
		trusted: trusted,
		storage: storage,
		global:  map[string]json.RawMessage{},
		project: map[string]json.RawMessage{},
	}
	m.reload()
	return m
}

// NewInMemory returns an independent manager that never touches a file.
func NewInMemory(initial Settings) *Manager {
	global := settingsToRaw(initial)
	migrateSettings(global)
	m := &Manager{
		trusted:       true,
		global:        cloneRawMap(global),
		project:       map[string]json.RawMessage{},
		memory:        true,
		memoryGlobal:  cloneRawMap(global),
		memoryProject: map[string]json.RawMessage{},
	}
	m.remerge()
	return m
}

// AgentDir returns the directory of the global settings file.
func (m *Manager) AgentDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.agentDir
}

// CWD returns the project directory.
func (m *Manager) CWD() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cwd
}

// GlobalPath returns the global settings file path.
func (m *Manager) GlobalPath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.memory || m.storage != nil {
		return ""
	}
	return m.globalPath()
}

// GlobalSettingsPath is GlobalPath under Pi's name.
func (m *Manager) GlobalSettingsPath() string { return m.GlobalPath() }

// ProjectPath returns the project settings file path.
func (m *Manager) ProjectPath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.memory || m.storage != nil {
		return ""
	}
	return m.projectPath()
}

// ProjectSettingsPath is ProjectPath under the longer name.
func (m *Manager) ProjectSettingsPath() string { return m.ProjectPath() }

func (m *Manager) globalPath() string {
	if m.agentDir == "" {
		return "settings.json"
	}
	return filepath.Join(m.agentDir, "settings.json")
}

func (m *Manager) projectPath() string {
	return filepath.Join(ProjectDir(m.cwd), "settings.json")
}

// Load re-reads the layers, as Pi's loadSettings does.
func (m *Manager) Load() { m.Reload() }

// Reload reads the layers again and discards overrides.
func (m *Manager) Reload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadLocked()
}

func (m *Manager) reload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadLocked()
}

func (m *Manager) reloadLocked() {
	m.overrides = nil
	m.globalErr, m.projectErr = nil, nil
	if m.memory {
		m.global = cloneRawMap(m.memoryGlobal)
		m.project = map[string]json.RawMessage{}
		if m.trusted {
			m.project = cloneRawMap(m.memoryProject)
		}
		m.remerge()
		m.notifyLocked()
		return
	}
	if m.storage != nil {
		global, err := m.loadStorage("global")
		if err != nil {
			m.globalErr = err
			m.errors = append(m.errors, Error{Scope: "global", Err: err})
			global = cloneRawMap(m.global)
		}
		m.global = global
		if warnings := settingsTypeWarnings(global); len(warnings) > 0 {
			m.recordError("global", "", settingsTypeWarningError(warnings))
		}
		if !m.trusted {
			m.project = map[string]json.RawMessage{}
			m.remerge()
			m.notifyLocked()
			return
		}
		project, err := m.loadStorage("project")
		if err != nil {
			m.projectErr = err
			m.errors = append(m.errors, Error{Scope: "project", Err: err})
			project = cloneRawMap(m.project)
		}
		m.project = project
		if warnings := settingsTypeWarnings(project); len(warnings) > 0 {
			m.recordError("project", "", settingsTypeWarningError(warnings))
		}
		m.remerge()
		m.notifyLocked()
		return
	}
	global, err := readSettingsFile(m.globalPath())
	if err != nil {
		m.globalErr = err
		m.errors = append(m.errors, Error{Scope: "global", Path: m.globalPath(), Err: err})
		global = cloneRawMap(m.global)
	}
	m.global = global
	if warnings := settingsTypeWarnings(global); len(warnings) > 0 {
		m.recordError("global", m.globalPath(), settingsTypeWarningError(warnings))
	}
	if !m.trusted {
		m.project = map[string]json.RawMessage{}
		m.remerge()
		m.notifyLocked()
		return
	}
	project, err := readSettingsFile(m.projectPath())
	if err != nil {
		m.projectErr = err
		m.errors = append(m.errors, Error{Scope: "project", Path: m.projectPath(), Err: err})
		project = cloneRawMap(m.project)
	}
	m.project = project
	if warnings := settingsTypeWarnings(project); len(warnings) > 0 {
		m.recordError("project", m.projectPath(), settingsTypeWarningError(warnings))
	}
	m.remerge()
	m.notifyLocked()
}

func (m *Manager) remerge() {
	m.remergeLocked()
}

func (m *Manager) remergeLocked() {
	m.generation++
	base := mergeSettingsLayers(m.global, m.project, nil)
	if len(m.overrides) > 0 {
		base = mergeSettingsLayers(base, nil, m.overrides)
	}
	m.merged = base
	m.globalSettings = decodeSettings(m.global)
	m.mergedSettings = decodeSettings(base)
	value := true
	if warnings := m.mergedSettings.Warnings; warnings != nil && warnings.AnthropicExtraUsage != nil {
		value = *warnings.AnthropicExtraUsage
	}
	m.warningsValue = value
}

// Get returns the effective settings.
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return decodeSettings(m.merged)
}

// GetSettings is Get under Pi's extension name.
func (m *Manager) GetSettings() Settings { return m.Get() }

// GetGlobalSettings returns the persisted global layer.
func (m *Manager) GetGlobalSettings() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return decodeSettings(m.global)
}

// GetProjectSettings returns the persisted project layer (empty when
// untrusted).
func (m *Manager) GetProjectSettings() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.trusted {
		return Settings{}
	}
	return decodeSettings(m.project)
}

// IsProjectTrusted reports whether project settings are read and written.
func (m *Manager) IsProjectTrusted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trusted
}

// SetProjectTrusted changes project-settings access and reloads that layer.
func (m *Manager) SetProjectTrusted(trusted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.trusted == trusted {
		return
	}
	m.trusted = trusted
	m.project = map[string]json.RawMessage{}
	m.projectErr = nil
	if trusted {
		if m.memory {
			m.project = cloneRawMap(m.memoryProject)
		} else if m.storage != nil {
			if project, err := m.loadStorage("project"); err == nil {
				m.project = project
				if warnings := settingsTypeWarnings(project); len(warnings) > 0 {
					m.recordError("project", "", settingsTypeWarningError(warnings))
				}
			} else {
				m.projectErr = err
				m.errors = append(m.errors, Error{Scope: "project", Err: err})
			}
		} else if project, err := readSettingsFile(m.projectPath()); err == nil {
			m.project = project
			if warnings := settingsTypeWarnings(project); len(warnings) > 0 {
				m.recordError("project", m.projectPath(), settingsTypeWarningError(warnings))
			}
		} else {
			m.projectErr = err
			m.errors = append(m.errors, Error{Scope: "project", Path: m.projectPath(), Err: err})
		}
	}
	m.remergeLocked()
	m.notifyLocked()
}

// ApplyOverrides applies non-persistent overrides on top of the merged view.
func (m *Manager) ApplyOverrides(overrides Settings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := cloneRawMap(m.overrides)
	mergeLayerInto(next, settingsToRaw(overrides))
	m.overrides = next
	m.remergeLocked()
	m.notifyLocked()
}

// Flush is a no-op: nulo writes settings synchronously.
func (m *Manager) Flush() error { return nil }

// Diagnostics returns load/write diagnostics without clearing them.
func (m *Manager) Diagnostics() []Error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Error(nil), m.errors...)
}

// DrainErrors returns load/write diagnostics and clears them.
func (m *Manager) DrainErrors() []Error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.errors) == 0 {
		return []Error{}
	}
	out := append([]Error(nil), m.errors...)
	m.errors = nil
	return out
}

func (m *Manager) recordError(scope, path string, err error) {
	m.errors = append(m.errors, Error{Scope: scope, Path: path, Err: err})
}

// ObserveChange registers a callback for every change of the effective
// settings. The callback runs with the manager's lock held, as Pi's runtime
// push does, and the returned function stops it.
func (m *Manager) ObserveChange(observe func()) func() {
	if observe == nil {
		return func() {}
	}
	m.mu.Lock()
	if m.observers == nil {
		m.observers = map[uint64]func(){}
	}
	m.nextObserver++
	id := m.nextObserver
	m.observers[id] = observe
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.observers, id)
		m.mu.Unlock()
	}
}

func (m *Manager) notifyLocked() {
	for _, observer := range m.observers {
		observer()
	}
}

// UpdateGlobal applies fn to the global layer and persists it.
func (m *Manager) UpdateGlobal(fn func(*Settings)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current := decodeSettings(m.global)
	beforeSettings := settingsToRaw(current)
	before := cloneRawMap(m.global)
	fn(&current)
	applySettingsDiff(m.global, beforeSettings, settingsToRaw(current))
	m.remergeLocked()
	m.notifyLocked()
	if m.memory {
		m.memoryGlobal = cloneRawMap(m.global)
		return nil
	}
	if m.globalErr != nil {
		return fmt.Errorf("global settings file has parse errors: %w", m.globalErr)
	}
	return m.persistScope("global", before, m.global, nil)
}

// applySettingsDiff patches layer with the members that changed between before
// and after. Untouched and unknown members keep their original bytes, and an
// object that changed in place is patched member by member so nested unknown
// members survive, as Pi's patchSettingsObject does.
func applySettingsDiff(layer, before, after map[string]json.RawMessage) {
	for key := range before {
		if _, ok := after[key]; !ok {
			delete(layer, key)
		}
	}
	for key, value := range after {
		previous, existed := before[key]
		if existed && bytes.Equal(previous, value) {
			continue
		}
		var afterObject map[string]json.RawMessage
		if json.Unmarshal(value, &afterObject) == nil && afterObject != nil {
			if current, ok := layer[key]; ok {
				if tree, err := parseJSONValue(current); err == nil && tree.object != nil {
					var beforeObject map[string]json.RawMessage
					_ = json.Unmarshal(previous, &beforeObject)
					applyRawDiff(tree, beforeObject, afterObject)
					if encoded, err := tree.MarshalJSON(); err == nil {
						layer[key] = encoded
						continue
					}
				}
			}
		}
		layer[key] = value
	}
}

// UpdateProject applies fn to the project layer and persists it.
func (m *Manager) UpdateProject(fn func(*Settings)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.trusted {
		return errors.New("Project is not trusted; refusing to write project settings")
	}
	current := decodeSettings(m.project)
	beforeSettings := settingsToRaw(current)
	before := cloneRawMap(m.project)
	fn(&current)
	applySettingsDiff(m.project, beforeSettings, settingsToRaw(current))
	m.remergeLocked()
	m.notifyLocked()
	if m.memory {
		m.memoryProject = cloneRawMap(m.project)
		return nil
	}
	if m.projectErr != nil {
		return fmt.Errorf("project settings file has parse errors: %w", m.projectErr)
	}
	return m.persistScope("project", before, m.project, nil)
}

func (m *Manager) mutateGlobal(fn func(map[string]json.RawMessage)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := cloneRawMap(m.global)
	fn(m.global)
	m.remergeLocked()
	m.notifyLocked()
	if m.memory {
		m.memoryGlobal = cloneRawMap(m.global)
		return nil
	}
	if m.globalErr != nil {
		return fmt.Errorf("global settings file has parse errors: %w", m.globalErr)
	}
	return m.persistScope("global", before, m.global, nil)
}

func (m *Manager) mutateGlobalOrdered(order []string, fn func(map[string]json.RawMessage)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := cloneRawMap(m.global)
	fn(m.global)
	m.remergeLocked()
	m.notifyLocked()
	if m.memory {
		m.memoryGlobal = cloneRawMap(m.global)
		return nil
	}
	if m.globalErr != nil {
		return fmt.Errorf("global settings file has parse errors: %w", m.globalErr)
	}
	return m.persistScope("global", before, m.global, order)
}

func (m *Manager) mutateProject(fn func(map[string]json.RawMessage)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.trusted {
		return errors.New("Project is not trusted; refusing to write project settings")
	}
	before := cloneRawMap(m.project)
	fn(m.project)
	m.remergeLocked()
	m.notifyLocked()
	if m.memory {
		m.memoryProject = cloneRawMap(m.project)
		return nil
	}
	if m.projectErr != nil {
		return fmt.Errorf("project settings file has parse errors: %w", m.projectErr)
	}
	return m.persistScope("project", before, m.project, nil)
}

func (m *Manager) loadStorage(scope string) (map[string]json.RawMessage, error) {
	result := map[string]json.RawMessage{}
	err := m.storage.WithLock(scope, func(current *string) (*string, error) {
		if current == nil || *current == "" {
			return nil, nil
		}
		data := []byte(*current)
		originalLength := len(data)
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		if originalLength > 0 && len(data) == 0 {
			return nil, fmt.Errorf("unexpected end of JSON input")
		}
		parsed, parseErr := parseSettingsMap(data)
		if parseErr != nil {
			return nil, parseErr
		}
		result = parsed
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (m *Manager) persistScope(scope string, before, after map[string]json.RawMessage, order []string) error {
	if m.storage == nil {
		path := m.globalPath()
		if scope == "project" {
			path = m.projectPath()
		}
		if err := writeSettingsPatchOrdered(path, before, after, order); err != nil {
			m.recordError(scope, path, err)
			return err
		}
		return nil
	}
	err := m.storage.WithLock(scope, func(current *string) (*string, error) {
		data := []byte{}
		exists := false
		if current != nil {
			data = []byte(*current)
			exists = len(data) > 0
		}
		patched, patchErr := patchedSettingsBytes(data, exists, before, after, order)
		if patchErr != nil {
			return nil, patchErr
		}
		next := string(patched)
		return &next, nil
	})
	if err != nil {
		m.recordError(scope, "", err)
	}
	return err
}

// SettingsManager is Manager under Pi's type name.
type SettingsManager = Manager

// NewSettingsManager loads settings from cwd and agentDir, trusting project
// settings as Pi's SettingsManager.create does.
func NewSettingsManager(cwd, agentDir string) *SettingsManager {
	return New(cwd, agentDir)
}

// SettingsManagerCreateOptions is Options under Pi's name.
type SettingsManagerCreateOptions = Options

// NewSettingsManagerWithProjectTrust loads settings with an explicit project
// trust decision.
func NewSettingsManagerWithProjectTrust(cwd, agentDir string, projectTrusted bool) *SettingsManager {
	return NewWithOptions(cwd, agentDir, Options{ProjectTrusted: &projectTrusted})
}

// NewSettingsManagerWithOptions loads settings with explicit options.
func NewSettingsManagerWithOptions(cwd, agentDir string, options SettingsManagerCreateOptions) *SettingsManager {
	return NewWithOptions(cwd, agentDir, options)
}

// NewInMemorySettingsManager is NewInMemory under Pi's name.
func NewInMemorySettingsManager(settings Settings) *SettingsManager { return NewInMemory(settings) }

// NewManager is New under a shorter name.
func NewManager(cwd, agentDir string) *Manager { return New(cwd, agentDir) }

// ProjectConfigDir is ProjectDir under PiG's name.
func ProjectConfigDir(cwd string) string { return ProjectDir(cwd) }
