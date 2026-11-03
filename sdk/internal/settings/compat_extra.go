package settings

import (
	"encoding/json"
	"math"
)

// GetSessionShareDestination returns Pi's /share destination, default "auto".
func (s Settings) GetSessionShareDestination() string {
	switch s.SessionShareDestination {
	case SessionShareDestinationGist, SessionShareDestinationRadius:
		return s.SessionShareDestination
	}
	return SessionShareDestinationAuto
}

// GetTuiMode returns "regular" only when authored, otherwise "fullscreen".
func (s Settings) GetTuiMode() string {
	if s.TuiMode == TuiModeRegular {
		return TuiModeRegular
	}
	return TuiModeFullscreen
}

// GetFullscreenExitOutput returns the validated fullscreen exit behavior.
func (s Settings) GetFullscreenExitOutput() string {
	if s.FullscreenExitOutput == FullscreenExitOutputResumeHint {
		return FullscreenExitOutputResumeHint
	}
	return FullscreenExitOutputTranscript
}

// GetFullscreenCopyOnSelect defaults to true.
func (s Settings) GetFullscreenCopyOnSelect() bool {
	return s.FullscreenCopyOnSelect == nil || *s.FullscreenCopyOnSelect
}

// GetFullscreenScrollbar returns "always", "hidden" or the "auto" default.
func (s Settings) GetFullscreenScrollbar() string {
	if s.FullscreenScrollbar == "always" || s.FullscreenScrollbar == "hidden" {
		return s.FullscreenScrollbar
	}
	return "auto"
}

// GetFullscreenWheelScrollLines returns a clamped line count or auto.
func (s Settings) GetFullscreenWheelScrollLines() WheelScrollLines {
	return parseWheelScrollLines(s.FullscreenWheelScrollLines)
}

// GetMaskSecretInput defaults to true.
func (s Settings) GetMaskSecretInput() bool {
	return s.MaskSecretInput == nil || *s.MaskSecretInput
}

// GetExtensionsAutoUpgrade reports the global-only extension upgrade flag.
func (s Settings) GetExtensionsAutoUpgrade() bool {
	return s.ExtensionsAutoUpgrade != nil && *s.ExtensionsAutoUpgrade
}

// GetExtensionsTypescriptRuntime returns the configured runtime, or "".
func (s Settings) GetExtensionsTypescriptRuntime() string {
	if s.ExtensionsTypescriptRuntime == nil {
		return ""
	}
	return *s.ExtensionsTypescriptRuntime
}

// GetSessionShareDestination returns the merged /share destination.
func (m *Manager) GetSessionShareDestination() string {
	return Settings{SessionShareDestination: m.mergedString("sessionShareDestination", "")}.GetSessionShareDestination()
}

// GetTuiMode returns the merged TUI mode.
func (m *Manager) GetTuiMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetTuiMode()
}

// GetFullscreenExitOutput returns the merged fullscreen exit behavior.
func (m *Manager) GetFullscreenExitOutput() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetFullscreenExitOutput()
}

// GetFullscreenCopyOnSelect returns the merged fullscreen copy preference.
func (m *Manager) GetFullscreenCopyOnSelect() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetFullscreenCopyOnSelect()
}

// GetFullscreenScrollbar returns the merged scrollbar mode.
func (m *Manager) GetFullscreenScrollbar() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetFullscreenScrollbar()
}

// GetFullscreenWheelScrollLines returns the merged wheel setting.
func (m *Manager) GetFullscreenWheelScrollLines() WheelScrollLines {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetFullscreenWheelScrollLines()
}

func parseWheelScrollLines(raw json.RawMessage) WheelScrollLines {
	if len(raw) == 0 || string(raw) == "null" {
		return WheelScrollLines{Auto: true}
	}
	var lines float64
	if json.Unmarshal(raw, &lines) != nil || math.IsNaN(lines) || math.IsInf(lines, 0) {
		return WheelScrollLines{Auto: true}
	}
	return WheelScrollLines{Lines: clampWheelScrollLines(lines)}
}

func clampWheelScrollLines(lines float64) float64 {
	return math.Max(1, math.Min(100, math.Floor(lines)))
}

// GetMaskSecretInput returns the merged login-input privacy setting.
func (m *Manager) GetMaskSecretInput() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.merged["maskSecretInput"]
	if !ok {
		return true
	}
	value, valid := decodeRawBool(raw)
	if !valid {
		return true
	}
	return value
}

// GetExtensionsAutoUpgrade reads the global-only setting.
func (m *Manager) GetExtensionsAutoUpgrade() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, valid := decodeRawBool(m.global["extensionsAutoUpgrade"])
	return valid && value
}

// GetExtensionsTypescriptRuntime reads the global-only runtime name.
func (m *Manager) GetExtensionsTypescriptRuntime() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, _ := decodeRawString(m.global["extensionsTypescriptRuntime"])
	return value
}

var _ = json.RawMessage(nil)

// GetTerminalCapabilityOverrides returns terminal capability overrides from
// the merged settings.
func (m *Manager) GetTerminalCapabilityOverrides() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return decodeSettings(m.merged).GetTerminalCapabilityOverrides()
}

// GetMermaidRenderingMode returns the markdown mermaid rendering mode.
func (s Settings) GetMermaidRenderingMode() string {
	if s.Markdown != nil && (s.Markdown.Mermaid == "off" || s.Markdown.Mermaid == "final") {
		return s.Markdown.Mermaid
	}
	return "streaming"
}
