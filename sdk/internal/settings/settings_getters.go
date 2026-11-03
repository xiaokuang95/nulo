package settings

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// OutputPad is Pi's 0|1 chat output padding. The alias keeps callers that use
// plain ints source-compatible.
type OutputPad = int

const (
	OutputPadNone OutputPad = 0
	OutputPadOne  OutputPad = 1
)

// GetShellPath normalizes shellPath like Pi's getShellPath.
func (s Settings) GetShellPath() (string, error) { return normalizeSettingsPath(s.ShellPath) }

// GetCommandPrefix returns the shell command prefix.
func (s Settings) GetCommandPrefix() string {
	if s.ShellCommandPrefix != "" {
		return s.ShellCommandPrefix
	}
	return s.CommandPrefix
}

// GetShellCommandPrefix is GetCommandPrefix under Pi's file-key name.
func (s Settings) GetShellCommandPrefix() string { return s.GetCommandPrefix() }

// GetSessionDir normalizes sessionDir.
func (s Settings) GetSessionDir() (string, error) { return normalizeSettingsPath(s.SessionDir) }

// GetShowImages reports terminal.showImages, default true.
func (s Settings) GetShowImages() bool {
	if s.Terminal != nil && s.Terminal.ShowImages != nil {
		return *s.Terminal.ShowImages
	}
	if s.ShowImages != nil {
		return *s.ShowImages
	}
	return DefaultShowImages
}

// GetImageWidthCells reports terminal.imageWidthCells, default 60 and at least 1.
func (s Settings) GetImageWidthCells() int {
	var value *int
	if s.Terminal != nil && s.Terminal.ImageWidthCells != nil {
		value = s.Terminal.ImageWidthCells
	} else {
		value = s.ImageWidthCells
	}
	if value == nil {
		return DefaultImageWidthCells
	}
	if *value < 1 {
		return 1
	}
	return *value
}

// GetImageAutoResize reports images.autoResize, default true.
func (s Settings) GetImageAutoResize() bool {
	if s.Images != nil && s.Images.AutoResize != nil {
		return *s.Images.AutoResize
	}
	if s.ImageAutoResize != nil {
		return *s.ImageAutoResize
	}
	return DefaultImageAutoResize
}

// GetBlockImages reports images.blockImages, default false.
func (s Settings) GetBlockImages() bool {
	if s.Images != nil && s.Images.BlockImages != nil {
		return *s.Images.BlockImages
	}
	if s.BlockImages != nil {
		return *s.BlockImages
	}
	return false
}

// GetClearOnShrink reports terminal.clearOnShrink, then PI_CLEAR_ON_SHRINK.
func (s Settings) GetClearOnShrink() bool {
	if s.Terminal != nil && s.Terminal.ClearOnShrink != nil {
		return *s.Terminal.ClearOnShrink
	}
	if s.ClearOnShrink != nil {
		return *s.ClearOnShrink
	}
	return clearOnShrinkEnv.isOne()
}

// GetShowTerminalProgress reports terminal.showTerminalProgress.
func (s Settings) GetShowTerminalProgress() bool {
	if s.Terminal != nil && s.Terminal.ShowTerminalProgress != nil {
		return *s.Terminal.ShowTerminalProgress
	}
	return s.ShowTerminalProgress != nil && *s.ShowTerminalProgress
}

// GetShowHardwareCursor reports showHardwareCursor or PI_HARDWARE_CURSOR=1.
func (s Settings) GetShowHardwareCursor() bool {
	if s.ShowHardwareCursor != nil {
		return *s.ShowHardwareCursor
	}
	return hardwareCursorEnv.isOne()
}

// GetEditorPaddingX returns editorPaddingX, default 0.
func (s Settings) GetEditorPaddingX() int {
	if s.EditorPaddingX == nil {
		return 0
	}
	return *s.EditorPaddingX
}

// GetOutputPad returns zero only for an explicit zero.
func (s Settings) GetOutputPad() OutputPad {
	if s.OutputPad != nil && *s.OutputPad == 0 {
		return OutputPadNone
	}
	return OutputPadOne
}

// GetAutocompleteMaxVisible returns autocompleteMaxVisible, default 5.
func (s Settings) GetAutocompleteMaxVisible() int {
	if s.AutocompleteMaxVisible == nil {
		return 5
	}
	return *s.AutocompleteMaxVisible
}

// GetCodeBlockIndent returns markdown.codeBlockIndent, default two spaces.
func (s Settings) GetCodeBlockIndent() string {
	if s.Markdown != nil && s.Markdown.CodeBlockIndent != nil {
		return *s.Markdown.CodeBlockIndent
	}
	return DefaultCodeBlockIndent
}

// GetHideThinkingBlock reports hideThinkingBlock.
func (s Settings) GetHideThinkingBlock() bool { return s.HideThinkingBlock }

// GetShowCacheMissNotices reports showCacheMissNotices.
func (s Settings) GetShowCacheMissNotices() bool { return s.ShowCacheMissNotices }

// GetQuietStartup parses quietStartup as Pi does.
func (s Settings) GetQuietStartup() QuietStartup { return s.QuietStartup }

// GetCollapseChangelog reports collapseChangelog.
func (s Settings) GetCollapseChangelog() bool { return s.CollapseChangelog }

// GetDefaultTools resolves defaultTools, or nil when unset.
func (s Settings) GetDefaultTools() []string {
	if s.DefaultTools == nil {
		return nil
	}
	return ResolveDefaultTools(s.DefaultTools)
}

// ResolvedDefaultTools resolves defaultTools, or Pi's built-in names.
func (s Settings) ResolvedDefaultTools() []string {
	if tools := s.GetDefaultTools(); tools != nil {
		return tools
	}
	return DefaultToolNames()
}

// GetDeviceID returns the global device ID on this settings value.
func (s Settings) GetDeviceID() string { return s.DeviceID }

// GetModelThinkingLevel returns one per-model thinking level.
func (s Settings) GetModelThinkingLevel(provider, modelID string) string {
	return s.ModelThinkingLevels[provider+"/"+modelID]
}

// GetWarnings returns warnings, defaulting anthropicExtraUsage to true.
func (s Settings) GetWarnings() WarningsSettings {
	value := true
	if s.Warnings != nil && s.Warnings.AnthropicExtraUsage != nil {
		value = *s.Warnings.AnthropicExtraUsage
	}
	return WarningsSettings{AnthropicExtraUsage: &value, anthropicExtraUsageSet: true}
}

// GetThinkingBudgets returns thinkingBudgets.
func (s Settings) GetThinkingBudgets() *ThinkingBudgetsSettings { return s.ThinkingBudgets }

// GetTerminalCapabilityOverrides returns terminal capability overrides in Pi's
// JSON shape, without importing the TUI package.
func (s Settings) GetTerminalCapabilityOverrides() map[string]any {
	result := map[string]any{}
	if s.Terminal == nil {
		return result
	}
	var images any
	if len(s.Terminal.Images) > 0 {
		_ = json.Unmarshal(s.Terminal.Images, &images)
	}
	switch value := images.(type) {
	case string:
		if value == "kitty" || value == "iterm2" {
			result["images"] = value
		}
	case bool:
		if !value {
			result["images"] = ""
		}
	}
	var trueColor any
	if len(s.Terminal.TrueColor) > 0 {
		_ = json.Unmarshal(s.Terminal.TrueColor, &trueColor)
	}
	if value, ok := trueColor.(bool); ok {
		result["trueColor"] = value
	}
	var hyperlinks any
	if len(s.Terminal.Hyperlinks) > 0 {
		_ = json.Unmarshal(s.Terminal.Hyperlinks, &hyperlinks)
	}
	if value, ok := hyperlinks.(bool); ok {
		result["hyperlinks"] = value
	}
	return result
}

// GetHttpIdleTimeoutMs resolves httpIdleTimeoutMs for this settings value.
func (s Settings) GetHttpIdleTimeoutMs() (int, error) {
	if len(s.httpIdleRaw) > 0 {
		if value, valid := parseTimeoutRaw(s.httpIdleRaw); valid {
			return value, nil
		}
		return 0, fmt.Errorf("Invalid httpIdleTimeoutMs setting: %s", jsSettingText(s.httpIdleRaw))
	}
	if s.HTTPIdleTimeoutMs == nil {
		return DefaultHTTPIdleTimeoutMs, nil
	}
	if *s.HTTPIdleTimeoutMs < 0 {
		return 0, fmt.Errorf("Invalid httpIdleTimeoutMs setting: %d", *s.HTTPIdleTimeoutMs)
	}
	return *s.HTTPIdleTimeoutMs, nil
}

// GetWebSocketConnectTimeoutMs resolves websocketConnectTimeoutMs for this
// settings value.
func (s Settings) GetWebSocketConnectTimeoutMs() (*int, error) {
	if len(s.webSocketRaw) > 0 {
		if value, valid := parseTimeoutRaw(s.webSocketRaw); valid {
			return &value, nil
		}
		return nil, fmt.Errorf("Invalid websocketConnectTimeoutMs setting: %s", jsSettingText(s.webSocketRaw))
	}
	if s.WebSocketConnectTimeoutMs == nil {
		return nil, nil
	}
	if *s.WebSocketConnectTimeoutMs < 0 {
		return nil, fmt.Errorf("Invalid websocketConnectTimeoutMs setting: %d", *s.WebSocketConnectTimeoutMs)
	}
	value := *s.WebSocketConnectTimeoutMs
	return &value, nil
}

// GetDefaultProvider returns defaultProvider.
func (s Settings) GetDefaultProvider() string { return s.DefaultProvider }

// GetDefaultModel returns defaultModel.
func (s Settings) GetDefaultModel() string { return s.DefaultModel }

// GetDefaultThinkingLevel returns defaultThinkingLevel.
func (s Settings) GetDefaultThinkingLevel() ai.ThinkingLevel {
	return ai.ThinkingLevel(s.DefaultThinkingLevel)
}

// GetThemeSetting returns the selected theme, preserving an authored empty
// theme; nil means the key is absent.
func (s Settings) GetThemeSetting() *string {
	if s.themeNull || s.themeNonString || (s.Theme == "" && !s.themePresent) {
		return nil
	}
	value := s.Theme
	return &value
}

// GetTheme returns a fixed theme name, or "" for a slash-separated theme path.
func (s Settings) GetTheme() string {
	if strings.Contains(s.Theme, "/") {
		return ""
	}
	return s.Theme
}

// GetTransport returns transport, default "auto".
func (s Settings) GetTransport() string {
	if s.Transport != "" {
		return s.Transport
	}
	return DefaultTransport
}

// GetSteeringMode returns steeringMode, default "one-at-a-time".
func (s Settings) GetSteeringMode() string {
	if s.SteeringMode != "" {
		return s.SteeringMode
	}
	return DefaultQueueMode
}

// GetFollowUpMode returns followUpMode, default "one-at-a-time".
func (s Settings) GetFollowUpMode() string {
	if s.FollowUpMode != "" {
		return s.FollowUpMode
	}
	return DefaultQueueMode
}

// GetDoubleEscapeAction returns doubleEscapeAction, default "tree".
func (s Settings) GetDoubleEscapeAction() string {
	if s.DoubleEscapeAction != "" {
		return s.DoubleEscapeAction
	}
	return DefaultDoubleEscapeAction
}

// GetTreeFilterMode validates treeFilterMode against Pi's closed set.
func (s Settings) GetTreeFilterMode() string {
	switch s.TreeFilterMode {
	case "default", "no-tools", "user-only", "labeled-only", "all":
		return s.TreeFilterMode
	}
	return DefaultTreeFilterMode
}

// GetDefaultProjectTrust validates the global fallback trust mode.
func (s Settings) GetDefaultProjectTrust() DefaultProjectTrust {
	switch DefaultProjectTrust(s.DefaultProjectTrust) {
	case DefaultProjectTrustAlways, DefaultProjectTrustNever:
		return DefaultProjectTrust(s.DefaultProjectTrust)
	}
	return DefaultProjectTrustAsk
}

// GetCacheWarmingMode reports cacheWarming, default "streaming".
func (s Settings) GetCacheWarmingMode() string {
	switch s.CacheWarming {
	case CacheWarmingOff, CacheWarmingStreaming, CacheWarmingIdle:
		return s.CacheWarming
	}
	return DefaultCacheWarming
}

// GetEnableSkillCommands reports enableSkillCommands, default true.
func (s Settings) GetEnableSkillCommands() bool {
	return s.EnableSkillCommands == nil || *s.EnableSkillCommands
}

// GetEnableInstallTelemetry reports enableInstallTelemetry, default true.
func (s Settings) GetEnableInstallTelemetry() bool {
	return s.EnableInstallTelemetry == nil || *s.EnableInstallTelemetry
}

// GetEnableAnalytics reports enableAnalytics, default false.
func (s Settings) GetEnableAnalytics() bool {
	return s.EnableAnalytics != nil && *s.EnableAnalytics
}

// GetTrackingID returns trackingId.
func (s Settings) GetTrackingID() string { return s.TrackingID }

// ThemeOverride returns ApplyOverrides input that selects theme, preserving an
// explicitly empty name.
func ThemeOverride(theme string) Settings {
	return Settings{Theme: theme, themePresent: true}
}
