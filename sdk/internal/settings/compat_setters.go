package settings

import (
	"encoding/json"
	"math"
)

// SetSessionShareDestination writes sessionShareDestination.
func (m *Manager) SetSessionShareDestination(destination string) error {
	return m.setGlobalValue("sessionShareDestination", destination)
}

// SetTuiMode writes tuiMode.
func (m *Manager) SetTuiMode(mode string) error {
	return m.setGlobalValue("tuiMode", mode)
}

// SetFullscreenExitOutput writes fullscreenExitOutput.
func (m *Manager) SetFullscreenExitOutput(output string) error {
	return m.setGlobalValue("fullscreenExitOutput", output)
}

// SetFullscreenCopyOnSelect writes fullscreenCopyOnSelect.
func (m *Manager) SetFullscreenCopyOnSelect(enabled bool) error {
	return m.setGlobalValue("fullscreenCopyOnSelect", enabled)
}

// SetFullscreenScrollbar writes fullscreenScrollbar.
func (m *Manager) SetFullscreenScrollbar(mode string) error {
	return m.setGlobalValue("fullscreenScrollbar", mode)
}

// SetFullscreenWheelScrollLines writes a clamped line count, auto, or null.
func (m *Manager) SetFullscreenWheelScrollLines(lines WheelScrollLines) error {
	var value json.RawMessage
	if lines.Auto {
		value = json.RawMessage(`"auto"`)
	} else {
		clamped := clampWheelScrollLines(lines.Lines)
		if math.IsNaN(clamped) {
			value = json.RawMessage(`null`)
		} else {
			encoded, _ := marshalJSONValue(clamped)
			value = encoded
		}
	}
	return m.setGlobalValue("fullscreenWheelScrollLines", value)
}

// SetMaskSecretInput writes maskSecretInput.
func (m *Manager) SetMaskSecretInput(enabled bool) error {
	return m.setGlobalValue("maskSecretInput", enabled)
}

// SetMermaidRenderingMode writes markdown.mermaid.
func (m *Manager) SetMermaidRenderingMode(mode string) error {
	return m.updateGlobalObject("markdown", func(markdown map[string]json.RawMessage) {
		encoded, _ := marshalJSONValue(mode)
		markdown["mermaid"] = encoded
	})
}
