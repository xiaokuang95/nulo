package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

func (m *Manager) mergedRaw() map[string]json.RawMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.merged
}

func (m *Manager) mergedString(key, fallback string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.merged[key]
	if !ok {
		return fallback
	}
	value, ok := decodeRawString(raw)
	if !ok {
		return fallback
	}
	return value
}

func (m *Manager) mergedBool(key string, fallback bool) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.merged[key]
	if !ok {
		return fallback
	}
	value, ok := decodeRawBool(raw)
	if !ok {
		return fallback
	}
	return value
}

func (m *Manager) mergedStrings(key string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.merged[key]
	if !ok {
		return nil
	}
	value, ok := decodeRawStrings(raw)
	if !ok {
		return nil
	}
	return value
}

// GetDefaultProvider returns defaultProvider, or "".
func (m *Manager) GetDefaultProvider() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.DefaultProvider
}

// GetDefaultModel returns defaultModel, or "".
func (m *Manager) GetDefaultModel() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.DefaultModel
}

// GetDefaultThinkingLevel returns defaultThinkingLevel, or "".
func (m *Manager) GetDefaultThinkingLevel() ai.ThinkingLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ai.ThinkingLevel(m.mergedSettings.DefaultThinkingLevel)
}

// GetModelThinkingLevel returns the per-model default, or "".
func (m *Manager) GetModelThinkingLevel(provider, modelID string) ai.ThinkingLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ai.ThinkingLevel(m.mergedSettings.ModelThinkingLevels[provider+"/"+modelID])
}

// GetAllModelThinkingLevels returns a copy of modelThinkingLevels.
func (m *Manager) GetAllModelThinkingLevels() map[string]ai.ThinkingLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	levels, _ := decodeRawObject(m.merged["modelThinkingLevels"])
	out := make(map[string]ai.ThinkingLevel, len(levels))
	for key, raw := range levels {
		if value, ok := decodeRawString(raw); ok {
			out[key] = ai.ThinkingLevel(value)
		}
	}
	return out
}

// GetCompactionSettings resolves compaction settings for an optional model.
func (m *Manager) GetCompactionSettings(model ...any) (CompactionConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	compaction, _ := decodeRawObject(m.merged["compaction"])
	provider, modelID := compactionModelKey(model)
	enabled := true
	if raw, ok := rawMember(compaction, "enabled"); ok {
		if value, ok := decodeRawBool(raw); ok {
			enabled = value
		}
	}
	reserve, err := compactionTokens(compaction, "reserveTokens", provider, modelID)
	if err != nil {
		return CompactionConfig{}, err
	}
	keep, err := compactionTokens(compaction, "keepRecentTokens", provider, modelID)
	if err != nil {
		return CompactionConfig{}, err
	}
	return CompactionConfig{Enabled: enabled, ReserveTokens: reserve, KeepRecentTokens: keep}, nil
}

func compactionModelKey(model []any) (provider, modelID string) {
	if len(model) == 0 {
		return "", ""
	}
	switch value := model[0].(type) {
	case ai.Model:
		return value.Provider, value.ID
	case *ai.Model:
		if value != nil {
			return value.Provider, value.ID
		}
	case string:
		provider, modelID, _ = strings.Cut(value, "/")
		if len(model) > 1 {
			if second, ok := model[1].(string); ok {
				return value, second
			}
		}
		return provider, modelID
	}
	return "", ""
}

func compactionTokens(compaction map[string]json.RawMessage, field, provider, modelID string) (int, error) {
	fallback := DefaultCompactionReserveTokens
	if field == "keepRecentTokens" {
		fallback = DefaultCompactionKeepRecentTokens
	}
	ordinary, ordinaryPresent := compaction[field]
	var ordinaryValue int
	if ordinaryPresent {
		value, err := decodeRawInt(ordinary)
		if err != nil {
			return 0, fmt.Errorf("Invalid compaction.%s setting: %s. Expected a non-negative safe integer.", field, jsSettingText(ordinary))
		}
		ordinaryValue = value
	}
	modelKey := ""
	if provider != "" || modelID != "" {
		modelKey = provider + "/" + modelID
	}
	if modelKey != "" {
		overrides, _ := decodeRawObject(compaction["modelOverrides"])
		if entry, ok := overrides[modelKey]; ok {
			trimmed := bytes.TrimSpace(entry)
			if len(trimmed) == 0 || trimmed[0] != '{' {
				return 0, fmt.Errorf("Invalid compaction.modelOverrides[%q] setting: %s. Expected an object.", modelKey, jsSettingText(entry))
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(entry, &object); err != nil || object == nil {
				return 0, fmt.Errorf("Invalid compaction.modelOverrides[%q] setting: %s. Expected an object.", modelKey, jsSettingText(entry))
			}
			if raw, present := object[field]; present {
				value, err := decodeRawInt(raw)
				if err != nil {
					return 0, fmt.Errorf("Invalid compaction.modelOverrides[%q].%s setting: %s. Expected a non-negative safe integer.", modelKey, field, jsSettingText(raw))
				}
				return value, nil
			}
		}
	}
	if ordinaryPresent {
		return ordinaryValue, nil
	}
	return fallback, nil
}

// jsSettingText renders a raw setting the way String(value) does in Pi's
// validation errors.
func jsSettingText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "null"
	}
	switch trimmed[0] {
	case '"':
		var text string
		if json.Unmarshal(trimmed, &text) == nil {
			return text
		}
	case 't', 'f':
		if bytes.Equal(trimmed, []byte("true")) {
			return "true"
		}
		if bytes.Equal(trimmed, []byte("false")) {
			return "false"
		}
	case '[':
		var items []json.RawMessage
		if json.Unmarshal(trimmed, &items) == nil {
			parts := make([]string, len(items))
			for i, item := range items {
				parts[i] = jsSettingText(item)
			}
			return strings.Join(parts, ",")
		}
	case '{':
		return "[object Object]"
	}
	if number, err := strconv.ParseFloat(string(trimmed), 64); err == nil {
		if number == 0 {
			return "0"
		}
		if number == math.Trunc(number) && math.Abs(number) < 1e21 {
			return strconv.FormatFloat(number, 'f', -1, 64)
		}
		return normalizeJSONNumber(string(trimmed))
	}
	return string(trimmed)
}

// GetCompactionEnabled returns the compaction toggle independently of token
// validation.
func (m *Manager) GetCompactionEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	compaction, _ := decodeRawObject(m.merged["compaction"])
	if raw, ok := rawMember(compaction, "enabled"); ok {
		if value, ok := decodeRawBool(raw); ok {
			return value
		}
	}
	return DefaultCompactionEnabled
}

// GetCompactionReserveTokens resolves reserveTokens independently of
// keepRecentTokens validation.
func (m *Manager) GetCompactionReserveTokens(model ...any) (int, error) {
	return m.compactionToken("reserveTokens", model...)
}

// GetCompactionKeepRecentTokens resolves keepRecentTokens independently of
// reserveTokens validation.
func (m *Manager) GetCompactionKeepRecentTokens(model ...any) (int, error) {
	return m.compactionToken("keepRecentTokens", model...)
}

func (m *Manager) compactionToken(field string, model ...any) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	compaction, _ := decodeRawObject(m.merged["compaction"])
	provider, modelID := compactionModelKey(model)
	return compactionTokens(compaction, field, provider, modelID)
}

// GetBranchSummarySettings resolves branch summary settings.
func (m *Manager) GetBranchSummarySettings() BranchSummaryConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := BranchSummaryConfig{ReserveTokens: DefaultBranchSummaryReserveTokens}
	if branch := m.mergedSettings.BranchSummary; branch != nil {
		if branch.ReserveTokens != nil {
			result.ReserveTokens = *branch.ReserveTokens
		}
		if branch.SkipPrompt != nil {
			result.SkipPrompt = *branch.SkipPrompt
		}
	}
	return result
}

// GetBranchSummarySkipPrompt reports skipPrompt.
func (m *Manager) GetBranchSummarySkipPrompt() bool {
	return m.GetBranchSummarySettings().SkipPrompt
}

// GetRetrySettings resolves retry settings.
func (m *Manager) GetRetrySettings() RetryConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := RetryConfig{
		Enabled:         true,
		MaxRetries:      DefaultRetryMaxRetries,
		BaseDelayMs:     DefaultRetryBaseDelayMs,
		MaxAgentDelayMs: DefaultMaxAgentDelayMs,
	}
	if retry := m.mergedSettings.Retry; retry != nil {
		if retry.Enabled != nil {
			result.Enabled = *retry.Enabled
		}
		if retry.MaxRetries != nil {
			result.MaxRetries = *retry.MaxRetries
		}
		if retry.BaseDelayMs != nil {
			result.BaseDelayMs = *retry.BaseDelayMs
		}
		if retry.MaxAgentDelayMs != nil {
			result.MaxAgentDelayMs = *retry.MaxAgentDelayMs
		}
	}
	return result
}

// GetRetryEnabled reports the agent retry toggle.
func (m *Manager) GetRetryEnabled() bool { return m.GetRetrySettings().Enabled }

// GetProviderRetrySettings resolves retry.provider.
func (m *Manager) GetProviderRetrySettings() ProviderRetryConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := ProviderRetryConfig{MaxRetryDelayMs: DefaultProviderRetryMaxDelayMs}
	if retry := m.mergedSettings.Retry; retry != nil && retry.Provider != nil {
		if retry.Provider.TimeoutMs != nil {
			value := *retry.Provider.TimeoutMs
			result.TimeoutMs = &value
		}
		if retry.Provider.MaxRetries != nil {
			value := *retry.Provider.MaxRetries
			result.MaxRetries = &value
		}
		if retry.Provider.MaxRetryDelayMs != nil {
			result.MaxRetryDelayMs = *retry.Provider.MaxRetryDelayMs
		}
	}
	return result
}

// GetCacheWarmingMode returns the global cache-warming mode.
func (m *Manager) GetCacheWarmingMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	switch mode := m.globalSettings.CacheWarming; mode {
	case "off", "streaming", "idle":
		return mode
	}
	return DefaultCacheWarming
}

// GetDefaultTools returns the resolved defaultTools selection, or nil when no
// settings layer sets it.
func (m *Manager) GetDefaultTools() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.merged["defaultTools"]
	if !ok {
		return nil
	}
	entries, valid := decodeRawStrings(raw)
	if !valid || entries == nil {
		return nil
	}
	return ResolveDefaultTools(entries)
}

// ResolvedDefaultTools returns GetDefaultTools, or Pi's built-in names when
// the setting is absent.
func (m *Manager) ResolvedDefaultTools() []string {
	if tools := m.GetDefaultTools(); tools != nil {
		return tools
	}
	return DefaultToolNames()
}

// GetEnabledModels returns enabledModels, or nil.
func (m *Manager) GetEnabledModels() []string { return m.mergedStrings("enabledModels") }

// GetSkillPaths returns the skills paths, accepting the legacy object form.
func (m *Manager) GetSkillPaths() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.merged["skills"]
	if !ok {
		return []string{}
	}
	if list, valid := decodeRawStrings(raw); valid {
		if list == nil {
			return []string{}
		}
		return list
	}
	if object, valid := decodeRawObject(raw); valid {
		directories, _ := decodeRawStrings(object["customDirectories"])
		if directories == nil {
			return []string{}
		}
		return directories
	}
	return []string{}
}

// GetPromptTemplatePaths returns prompts, or an empty list.
func (m *Manager) GetPromptTemplatePaths() []string {
	return nonNilStrings(m.mergedStrings("prompts"))
}

// GetThemePaths returns themes, or an empty list.
func (m *Manager) GetThemePaths() []string {
	return nonNilStrings(m.mergedStrings("themes"))
}

// GetExtensionPaths returns extensions, or an empty list.
func (m *Manager) GetExtensionPaths() []string {
	return nonNilStrings(m.mergedStrings("extensions"))
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// GetSessionDir returns the configured session directory, expanding ~ and
// file URLs.
func (m *Manager) GetSessionDir() (string, error) {
	return normalizeSettingsPath(m.mergedString("sessionDir", ""))
}

// GetShellPath returns shellPath, expanding ~ and file URLs.
func (m *Manager) GetShellPath() (string, error) {
	return normalizeSettingsPath(m.mergedString("shellPath", ""))
}

// GetShellCommandPrefix returns shellCommandPrefix.
func (m *Manager) GetShellCommandPrefix() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetShellCommandPrefix()
}

// GetNpmCommand returns npmCommand, or nil.
func (m *Manager) GetNpmCommand() []string {
	return m.mergedStrings("npmCommand")
}

// GetHTTPProxy returns the global httpProxy. Project settings cannot change
// the proxy used for every session.
func (m *Manager) GetHTTPProxy() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, _ := decodeRawString(m.global["httpProxy"])
	return value
}

// GetTransport returns transport, default "auto".
func (m *Manager) GetTransport() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetTransport()
}

// GetSteeringMode returns steeringMode, default "one-at-a-time".
func (m *Manager) GetSteeringMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetSteeringMode()
}

// GetFollowUpMode returns followUpMode, default "one-at-a-time".
func (m *Manager) GetFollowUpMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetFollowUpMode()
}

// GetTheme returns theme, or "" for a theme path (which contains "/").
func (m *Manager) GetTheme() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetTheme()
}

// GetThemeSetting returns theme as authored, or nil when it is unset.
func (m *Manager) GetThemeSetting() *string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.merged["theme"]
	if !ok {
		return nil
	}
	value, valid := decodeRawString(raw)
	if !valid {
		return nil
	}
	return &value
}

// GetHideThinkingBlock reports hideThinkingBlock.
func (m *Manager) GetHideThinkingBlock() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.HideThinkingBlock
}

// GetShowCacheMissNotices reports showCacheMissNotices.
func (m *Manager) GetShowCacheMissNotices() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.ShowCacheMissNotices
}

// GetShowImages reports terminal.showImages (or legacy showImages), default true.
func (m *Manager) GetShowImages() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetShowImages()
}

// GetImageWidthCells returns terminal.imageWidthCells (or legacy
// imageWidthCells), default 60.
func (m *Manager) GetImageWidthCells() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetImageWidthCells()
}

// GetClearOnShrink reports terminal.clearOnShrink.
func (m *Manager) GetClearOnShrink() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetClearOnShrink()
}

// GetShowTerminalProgress reports terminal.showTerminalProgress.
func (m *Manager) GetShowTerminalProgress() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetShowTerminalProgress()
}

// GetImageAutoResize reports images.autoResize (or legacy imageAutoResize),
// default true.
func (m *Manager) GetImageAutoResize() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetImageAutoResize()
}

// GetBlockImages reports images.blockImages (or legacy blockImages), default
// false.
func (m *Manager) GetBlockImages() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetBlockImages()
}

func decodeBoolSetting(raw map[string]json.RawMessage, key string, fallback bool) bool {
	if value, ok := decodeRawBool(raw[key]); ok {
		return value
	}
	return fallback
}

func decodeIntSetting(raw map[string]json.RawMessage, key string, fallback int) int {
	if value, err := decodeRawInt(raw[key]); err == nil {
		return value
	}
	return fallback
}

// GetEnableSkillCommands reports enableSkillCommands, default true.
func (m *Manager) GetEnableSkillCommands() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if value := m.mergedSettings.EnableSkillCommands; value != nil {
		return *value
	}
	return DefaultEnableSkillCommands
}

// GetDoubleEscapeAction returns doubleEscapeAction, default "tree".
func (m *Manager) GetDoubleEscapeAction() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if value := m.mergedSettings.DoubleEscapeAction; value != "" {
		return value
	}
	return DefaultDoubleEscapeAction
}

// GetTreeFilterMode returns treeFilterMode, default "default".
func (m *Manager) GetTreeFilterMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	switch value := m.mergedSettings.TreeFilterMode; value {
	case "default", "no-tools", "user-only", "labeled-only", "all":
		return value
	}
	return DefaultTreeFilterMode
}

// GetDefaultProjectTrust returns the global defaultProjectTrust fallback,
// default "ask". Project settings cannot change it.
func (m *Manager) GetDefaultProjectTrust() DefaultProjectTrust {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value := DefaultProjectTrust(m.globalSettings.DefaultProjectTrust)
	switch value {
	case DefaultProjectTrustAsk, DefaultProjectTrustAlways, DefaultProjectTrustNever:
		return value
	}
	return DefaultProjectTrustAsk
}

// GetLastChangelogVersion returns lastChangelogVersion.
func (m *Manager) GetLastChangelogVersion() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.LastChangelogVersion
}

// GetDeviceID returns the global deviceId.
func (m *Manager) GetDeviceID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, _ := decodeRawString(m.global["deviceId"])
	return value
}

// GetMermaidRenderingMode returns markdown.mermaid when it is "off" or
// "final", otherwise "streaming".
func (m *Manager) GetMermaidRenderingMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if markdown := m.mergedSettings.Markdown; markdown != nil {
		if markdown.Mermaid == "off" || markdown.Mermaid == "final" {
			return markdown.Mermaid
		}
	}
	return "streaming"
}

// GetCodeBlockIndent returns markdown.codeBlockIndent, default two spaces.
func (m *Manager) GetCodeBlockIndent() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mergedSettings.GetCodeBlockIndent()
}

// GetCodemodeMode returns codemode.mode, default "on".
func (m *Manager) GetCodemodeMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	codemode, _ := decodeRawObject(m.merged["codemode"])
	if raw, ok := rawMember(codemode, "mode"); ok {
		if value, ok := decodeRawString(raw); ok && (value == "on" || value == "only") {
			return value
		}
	}
	return DefaultCodemodeMode
}

// GetCodemodeInlineBudget returns codemode.inlineBudget, default 3000.
func (m *Manager) GetCodemodeInlineBudget() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	codemode, _ := decodeRawObject(m.merged["codemode"])
	if raw, ok := rawMember(codemode, "inlineBudget"); ok {
		if value, err := decodeRawInt(raw); err == nil {
			return value
		}
	}
	return DefaultCodemodeInlineBudget
}

// GetWarnings returns warnings.anthropicExtraUsage, default true.
func (m *Manager) GetWarnings() WarningsSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return WarningsSettings{AnthropicExtraUsage: &m.warningsValue, anthropicExtraUsageSet: true}
}

// GetThinkingBudgets returns thinkingBudgets, or nil.
func (m *Manager) GetThinkingBudgets() *ThinkingBudgetsSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.merged["thinkingBudgets"]
	if !ok {
		return nil
	}
	var budgets ThinkingBudgetsSettings
	if err := json.Unmarshal(raw, &budgets); err != nil {
		return nil
	}
	return &budgets
}

func (m *Manager) mergedInt(key string, fallback int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.merged[key]
	if !ok {
		return fallback
	}
	value, err := decodeRawInt(raw)
	if err != nil {
		return fallback
	}
	return value
}

// GetCommandPrefix is GetShellCommandPrefix under Settings' name.
func (m *Manager) GetCommandPrefix() string { return m.GetShellCommandPrefix() }
