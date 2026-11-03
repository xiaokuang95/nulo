package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

var stringArraySettings = []string{
	"extensions", "skills", "prompts", "themes", "enabledModels", "defaultTools", "npmCommand",
}

var boolSettings = []string{
	"hideThinkingBlock", "showCacheMissNotices", "enableSkillCommands",
	"enableInstallTelemetry", "enableAnalytics", "collapseChangelog", "showHardwareCursor",
}

// settingsTypeWarnings reports values Pi's typed accessors would ignore. One
// combined warning per file keeps the rest of the file usable.
func settingsTypeWarnings(object map[string]json.RawMessage) []string {
	var warnings []string
	for _, key := range stringArraySettings {
		raw, ok := object[key]
		if !ok || isJSONNull(raw) {
			continue
		}
		if key == "skills" && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
			continue
		}
		if !isStringArray(raw) {
			warnings = append(warnings, fmt.Sprintf("%q (expected an array of strings)", key))
		}
	}
	if raw, ok := object["packages"]; ok && !isJSONNull(raw) {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '[' {
			warnings = append(warnings, `"packages" (expected an array)`)
		} else {
			var items []json.RawMessage
			if json.Unmarshal(trimmed, &items) != nil {
				warnings = append(warnings, `"packages" (expected an array)`)
			} else {
				for _, item := range items {
					var source PackageSource
					if json.Unmarshal(item, &source) != nil || source.Source == "" && !source.WasObject && string(bytes.TrimSpace(item)) != `""` {
						warnings = append(warnings, `"packages" (an element of the wrong type)`)
						break
					}
				}
			}
		}
	}
	for _, key := range boolSettings {
		raw, ok := object[key]
		if !ok || isJSONNull(raw) {
			continue
		}
		var value bool
		if json.Unmarshal(raw, &value) != nil {
			warnings = append(warnings, fmt.Sprintf("%q (expected a boolean)", key))
		}
	}
	if raw, ok := object["retry"]; ok && !isJSONNull(raw) {
		var retry map[string]json.RawMessage
		if json.Unmarshal(raw, &retry) == nil {
			for _, key := range []string{"maxRetries", "baseDelayMs", "maxAgentDelayMs"} {
				if value, ok := retry[key]; ok && !isJSONNull(value) && !isJSONNumber(value) {
					warnings = append(warnings, fmt.Sprintf("%q (expected a number)", "retry."+key))
				}
			}
			if providerRaw, ok := retry["provider"]; ok && !isJSONNull(providerRaw) {
				var provider map[string]json.RawMessage
				if json.Unmarshal(providerRaw, &provider) == nil {
					for _, key := range []string{"timeoutMs", "maxRetries", "maxRetryDelayMs"} {
						if value, ok := provider[key]; ok && !isJSONNull(value) && !isJSONNumber(value) {
							warnings = append(warnings, fmt.Sprintf("%q (expected a number)", "retry.provider."+key))
						}
					}
				}
			}
		}
	}
	return warnings
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func isStringArray(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return false
	}
	var items []json.RawMessage
	if json.Unmarshal(trimmed, &items) != nil {
		return false
	}
	for _, item := range items {
		var text string
		if json.Unmarshal(item, &text) != nil {
			return false
		}
	}
	return true
}

func settingsTypeWarningError(warnings []string) error {
	return fmt.Errorf("ignored settings of the wrong type: %s", strings.Join(warnings, ", "))
}

func isJSONNumber(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if decoder.Decode(&number) != nil {
		return false
	}
	_, err := number.Float64()
	return err == nil
}

// AgentSessionRuntimeDiagnostic is one startup diagnostic. Type is "info",
// "warning", or "error".
type AgentSessionRuntimeDiagnostic struct {
	Type    string
	Message string
}

// SettingsError is Manager.Error under PiG's name.
type SettingsError = Error

// CollectSettingsDiagnostics drains the manager's settings errors as warnings
// naming the settings file, or the scope when no file backs the error.
func CollectSettingsDiagnostics(settingsManager *SettingsManager) []AgentSessionRuntimeDiagnostic {
	var diagnostics []AgentSessionRuntimeDiagnostic
	for _, settingsError := range settingsManager.DrainErrors() {
		message := fmt.Sprintf("Invalid %s settings: %v", settingsError.Scope, settingsError.Err)
		if settingsError.Path != "" {
			message = fmt.Sprintf("Invalid settings file %s: %v", settingsError.Path, settingsError.Err)
		}
		diagnostics = append(diagnostics, AgentSessionRuntimeDiagnostic{Type: "warning", Message: message})
	}
	return diagnostics
}

// DeduplicateDiagnostics removes duplicate type/message diagnostics while
// preserving their first occurrence.
func DeduplicateDiagnostics(diagnostics []AgentSessionRuntimeDiagnostic) []AgentSessionRuntimeDiagnostic {
	seen := make(map[AgentSessionRuntimeDiagnostic]bool, len(diagnostics))
	var out []AgentSessionRuntimeDiagnostic
	for _, diagnostic := range diagnostics {
		if seen[diagnostic] {
			continue
		}
		seen[diagnostic] = true
		out = append(out, diagnostic)
	}
	return out
}

// FormatReportedDiagnostic renders a diagnostic for non-interactive output.
func FormatReportedDiagnostic(diagnostic AgentSessionRuntimeDiagnostic) string {
	return formatReportedDiagnostic(diagnostic, false)
}

func formatReportedDiagnostic(diagnostic AgentSessionRuntimeDiagnostic, color bool) string {
	var prefix, open, closeSeq string
	switch diagnostic.Type {
	case "error":
		prefix, open, closeSeq = "Error: ", "[31m", "[39m"
	case "warning":
		prefix, open, closeSeq = "Warning: ", "[33m", "[39m"
	default:
		open, closeSeq = "[2m", "[22m"
	}
	if !color {
		return prefix + diagnostic.Message
	}
	return open + prefix + diagnostic.Message + closeSeq
}

// ReportDiagnostics writes diagnostics to stderr for non-interactive modes.
func ReportDiagnostics(diagnostics []AgentSessionRuntimeDiagnostic) {
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(os.Stderr, FormatReportedDiagnostic(diagnostic))
	}
}
