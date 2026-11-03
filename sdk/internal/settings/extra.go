package settings

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func jsTrimmedEmpty(value string) bool {
	return strings.TrimFunc(value, func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', '\v', '\f', '\r', '\u00a0', '\u1680',
			'\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200a',
			'\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
			return true
		}
		return false
	}) == ""
}

// GetExternalEditorCommand returns the configured command, then VISUAL,
// EDITOR, or the platform default.
func (m *Manager) GetExternalEditorCommand() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return resolveExternalEditorCommand(m.mergedSettings.ExternalEditor, visualEnv.get(), editorEnv.get(), runtime.GOOS)
}

func resolveExternalEditorCommand(configured, visual, editor, goos string) string {
	if !jsTrimmedEmpty(configured) {
		return configured
	}
	if visual != "" {
		return visual
	}
	if editor != "" {
		return editor
	}
	if goos == "windows" {
		return "notepad"
	}
	return "nano"
}

// SetExternalEditor writes externalEditor.
func (m *Manager) SetExternalEditor(editor string) error {
	return m.setGlobalValue("externalEditor", editor)
}

// GetQuietStartup returns quietStartup, default false.
func (m *Manager) GetQuietStartup() QuietStartup {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.QuietStartup
}

// SetQuietStartup writes quietStartup as Pi's boolean | "header" value.
func (m *Manager) SetQuietStartup(quiet QuietStartup) error {
	var value json.RawMessage
	switch quiet {
	case QuietStartupTrue:
		value = json.RawMessage(`true`)
	case QuietStartupHeader:
		value = json.RawMessage(`"header"`)
	default:
		value = json.RawMessage(`false`)
	}
	return m.setGlobalValue("quietStartup", value)
}

// GetCollapseChangelog reports collapseChangelog, default false.
func (m *Manager) GetCollapseChangelog() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.CollapseChangelog
}

// SetCollapseChangelog writes collapseChangelog.
func (m *Manager) SetCollapseChangelog(collapse bool) error {
	return m.setGlobalValue("collapseChangelog", collapse)
}

// GetEnableInstallTelemetry reports enableInstallTelemetry, default true.
func (m *Manager) GetEnableInstallTelemetry() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if value := m.mergedSettings.EnableInstallTelemetry; value != nil {
		return *value
	}
	return true
}

// SetEnableInstallTelemetry writes enableInstallTelemetry.
func (m *Manager) SetEnableInstallTelemetry(enabled bool) error {
	return m.setGlobalValue("enableInstallTelemetry", enabled)
}

// GetEnableAnalytics reports enableAnalytics, default false.
func (m *Manager) GetEnableAnalytics() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.EnableAnalytics != nil && *m.mergedSettings.EnableAnalytics
}

// GetTrackingID returns trackingId.
func (m *Manager) GetTrackingID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.TrackingID
}

// SetEnableAnalytics writes enableAnalytics and generates a tracking ID on the
// first opt-in.
func (m *Manager) SetEnableAnalytics(enabled bool) error {
	return m.mutateGlobal(func(global map[string]json.RawMessage) {
		global["enableAnalytics"] = mustJSON(enabled)
		if enabled {
			existing, _ := decodeRawString(global["trackingId"])
			if existing == "" {
				global["trackingId"] = mustJSON(randomUUID())
			}
		}
	})
}

// GetEditorPaddingX returns editorPaddingX, default 0.
func (m *Manager) GetEditorPaddingX() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if value := m.mergedSettings.EditorPaddingX; value != nil {
		return *value
	}
	return 0
}

// SetEditorPaddingX writes editorPaddingX, clamped to 0..3.
func (m *Manager) SetEditorPaddingX(padding int) error {
	padding = max(0, min(3, padding))
	return m.setGlobalValue("editorPaddingX", padding)
}

// GetOutputPad returns outputPad: only an explicit 0 differs from 1.
func (m *Manager) GetOutputPad() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if value := m.mergedSettings.OutputPad; value != nil && *value == 0 {
		return 0
	}
	return 1
}

// SetOutputPad writes outputPad, clamped to 0..1.
func (m *Manager) SetOutputPad(padding int) error {
	padding = max(0, min(1, padding))
	return m.setGlobalValue("outputPad", padding)
}

// GetAutocompleteMaxVisible returns autocompleteMaxVisible, default 5.
func (m *Manager) GetAutocompleteMaxVisible() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if value := m.mergedSettings.AutocompleteMaxVisible; value != nil {
		return *value
	}
	return 5
}

// SetAutocompleteMaxVisible writes autocompleteMaxVisible, clamped to 3..20.
func (m *Manager) SetAutocompleteMaxVisible(visible int) error {
	visible = max(3, min(20, visible))
	return m.setGlobalValue("autocompleteMaxVisible", visible)
}

// GetShowHardwareCursor reports showHardwareCursor or PI_HARDWARE_CURSOR=1.
func (m *Manager) GetShowHardwareCursor() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if value := m.mergedSettings.ShowHardwareCursor; value != nil {
		return *value
	}
	return hardwareCursorEnv.isOne()
}

// SetShowHardwareCursor writes showHardwareCursor.
func (m *Manager) SetShowHardwareCursor(enabled bool) error {
	return m.setGlobalValue("showHardwareCursor", enabled)
}

// parseTimeoutRaw parses Pi's http/websocket timeout value.
func parseTimeoutRaw(raw json.RawMessage) (int, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return 0, false
	}
	if trimmed[0] == '"' {
		var text string
		if json.Unmarshal(trimmed, &text) != nil {
			return 0, false
		}
		text = strings.TrimSpace(text)
		if strings.EqualFold(text, "disabled") {
			return 0, true
		}
		if text == "" {
			return 0, false
		}
		number, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
			return 0, false
		}
		return timeoutInt(number), true
	}
	number, err := strconv.ParseFloat(string(trimmed), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return 0, false
	}
	return timeoutInt(number), true
}

func timeoutInt(number float64) int {
	floored := math.Floor(number)
	if floored >= float64(math.MaxInt) {
		return math.MaxInt
	}
	return int(floored)
}

// jsNumberText renders a number the way ECMAScript's String(number) does.
func jsNumberText(number float64) string {
	switch {
	case math.IsNaN(number):
		return "NaN"
	case math.IsInf(number, 1):
		return "Infinity"
	case math.IsInf(number, -1):
		return "-Infinity"
	case number == 0:
		return "0"
	case number == math.Trunc(number) && math.Abs(number) < 1e21:
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return normalizeJSONNumber(strconv.FormatFloat(number, 'g', -1, 64))
}

// GetHttpIdleTimeoutMs returns httpIdleTimeoutMs, default five minutes.
func (m *Manager) GetHttpIdleTimeoutMs() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, present := m.merged["httpIdleTimeoutMs"]
	if !present {
		return DefaultHTTPIdleTimeoutMs, nil
	}
	if value, valid := parseTimeoutRaw(raw); valid {
		return value, nil
	}
	return 0, fmt.Errorf("Invalid httpIdleTimeoutMs setting: %s", jsSettingText(raw))
}

// SetHttpIdleTimeoutMs writes a non-negative timeout, flooring fractions.
func (m *Manager) SetHttpIdleTimeoutMs(timeoutMs float64) error {
	if math.IsNaN(timeoutMs) || math.IsInf(timeoutMs, 0) || timeoutMs < 0 {
		return fmt.Errorf("Invalid httpIdleTimeoutMs setting: %s", jsNumberText(timeoutMs))
	}
	return m.setGlobalValue("httpIdleTimeoutMs", timeoutInt(timeoutMs))
}

// GetWebSocketConnectTimeoutMs returns websocketConnectTimeoutMs, or nil when
// unset. A present but invalid value is an error.
func (m *Manager) GetWebSocketConnectTimeoutMs() (*int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, present := m.merged["websocketConnectTimeoutMs"]
	if !present {
		return nil, nil
	}
	if value, valid := parseTimeoutRaw(raw); valid {
		return &value, nil
	}
	return nil, fmt.Errorf("Invalid websocketConnectTimeoutMs setting: %s", jsSettingText(raw))
}

// SetWebSocketConnectTimeoutMs writes websocketConnectTimeoutMs.
func (m *Manager) SetWebSocketConnectTimeoutMs(timeoutMs float64) error {
	if math.IsNaN(timeoutMs) || math.IsInf(timeoutMs, 0) || timeoutMs < 0 {
		return fmt.Errorf("Invalid websocketConnectTimeoutMs setting: %s", jsNumberText(timeoutMs))
	}
	return m.setGlobalValue("websocketConnectTimeoutMs", timeoutInt(timeoutMs))
}

// GetProviderRequestTimeoutMs returns retry.provider.timeoutMs when set, or
// the HTTP idle timeout otherwise.
func (m *Manager) GetProviderRequestTimeoutMs() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if retry := m.mergedSettings.Retry; retry != nil && retry.Provider != nil && retry.Provider.TimeoutMs != nil {
		return *retry.Provider.TimeoutMs, nil
	}
	return m.mergedSettings.GetHttpIdleTimeoutMs()
}

// GetOrCreateDeviceID returns the global deviceId, creating and saving it on
// first use.
func (m *Manager) GetOrCreateDeviceID() string {
	m.mu.RLock()
	existing, _ := decodeRawString(m.global["deviceId"])
	m.mu.RUnlock()
	if existing != "" {
		return existing
	}
	id := randomUUID()
	_ = m.mutateGlobal(func(global map[string]json.RawMessage) {
		if current, _ := decodeRawString(global["deviceId"]); current == "" {
			global["deviceId"] = mustJSON(id)
		} else {
			id = current
		}
	})
	return id
}

// GetOrCreateDeviceId is GetOrCreateDeviceID under Pi's spelling.
func (m *Manager) GetOrCreateDeviceId() string { return m.GetOrCreateDeviceID() }

// ExtensionSettingsJSON returns the merged settings as JSON. The encoding is
// reused until the effective settings change.
func (m *Manager) ExtensionSettingsJSON() json.RawMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.extensionJSON != nil && m.extensionGen == m.generation {
		return m.extensionJSON
	}
	var encoded []byte
	var err error
	if !m.memory && m.storage == nil {
		encoded, err = m.orderedExtensionJSON()
	}
	if encoded == nil || err != nil {
		encoded, err = marshalJSONValue(decodeSettings(m.merged))
		if err != nil {
			encoded = []byte(`{}`)
		}
	}
	m.extensionJSON = encoded
	m.extensionGen = m.generation
	return encoded
}

// ExtensionSettings returns a fresh map view of the merged settings.
func (m *Manager) ExtensionSettings() map[string]any {
	encoded := m.ExtensionSettingsJSON()
	var settings map[string]any
	if err := json.Unmarshal(encoded, &settings); err != nil || settings == nil {
		return map[string]any{}
	}
	return settings
}

// randomUUID returns a lower-case RFC 4122 v4 UUID.
func randomUUID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// GetPackages returns packages, or an empty slice.
func (m *Manager) GetPackages() []PackageSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items, _ := decodePackageSources(m.merged["packages"])
	if items == nil {
		return []PackageSource{}
	}
	return clonePackageSources(items)
}

// SetPackages writes packages; nil removes the key, an empty non-nil slice
// writes [].
func (m *Manager) SetPackages(packages []PackageSource) error {
	return m.setPackages("global", packages)
}

// SetProjectPackages writes packages to the project layer.
func (m *Manager) SetProjectPackages(packages []PackageSource) error {
	return m.setPackages("project", packages)
}

func (m *Manager) setPackages(scope string, packages []PackageSource) error {
	mutate := m.mutateGlobal
	if scope == "project" {
		mutate = m.mutateProject
	}
	return mutate(func(layer map[string]json.RawMessage) {
		if packages == nil {
			delete(layer, "packages")
			return
		}
		layer["packages"] = mustJSON(clonePackageSources(packages))
	})
}

// IsInstallTelemetryEnabled reports the PI_TELEMETRY env override first, then
// enableInstallTelemetry (default true).
func (m *Manager) IsInstallTelemetryEnabled() bool {
	if value, ok := os.LookupEnv("PI_TELEMETRY"); ok {
		trimmed := strings.TrimSpace(value)
		return trimmed == "1" || strings.EqualFold(trimmed, "true")
	}
	return m.GetEnableInstallTelemetry()
}
