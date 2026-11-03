package sdk

import (
	settingspkg "github.com/xiaokuang95/nulo/sdk/internal/settings"
)

const (
	DefaultCompactionReserveTokens    = settingspkg.DefaultCompactionReserveTokens
	DefaultCompactionKeepRecentTokens = settingspkg.DefaultCompactionKeepRecentTokens
	DefaultBranchSummaryReserveTokens = settingspkg.DefaultBranchSummaryReserveTokens
	DefaultRetryMaxRetries            = settingspkg.DefaultRetryMaxRetries
	DefaultRetryBaseDelayMs           = settingspkg.DefaultRetryBaseDelayMs
	DefaultMaxAgentDelayMs            = settingspkg.DefaultMaxAgentDelayMs
	DefaultProviderRetryMaxDelayMs    = settingspkg.DefaultProviderRetryMaxDelayMs
	DefaultCacheWarming               = settingspkg.DefaultCacheWarming
	DefaultTransport                  = settingspkg.DefaultTransport
	DefaultQueueMode                  = settingspkg.DefaultQueueMode
	DefaultEnableSkillCommands        = settingspkg.DefaultEnableSkillCommands
	DefaultShowImages                 = settingspkg.DefaultShowImages
	DefaultImageWidthCells            = settingspkg.DefaultImageWidthCells
	DefaultImageAutoResize            = settingspkg.DefaultImageAutoResize
	DefaultDoubleEscapeAction         = settingspkg.DefaultDoubleEscapeAction
	DefaultTreeFilterMode             = settingspkg.DefaultTreeFilterMode
	DefaultCodeBlockIndent            = settingspkg.DefaultCodeBlockIndent
	DefaultCodemodeMode               = settingspkg.DefaultCodemodeMode
	DefaultCodemodeInlineBudget       = settingspkg.DefaultCodemodeInlineBudget
	DefaultHTTPIdleTimeoutMs          = settingspkg.DefaultHTTPIdleTimeoutMs

	DefaultProjectTrustAsk    = settingspkg.DefaultProjectTrustAsk
	DefaultProjectTrustAlways = settingspkg.DefaultProjectTrustAlways
	DefaultProjectTrustNever  = settingspkg.DefaultProjectTrustNever
	CacheWarmingOff           = settingspkg.CacheWarmingOff
	CacheWarmingStreaming     = settingspkg.CacheWarmingStreaming
	CacheWarmingIdle          = settingspkg.CacheWarmingIdle
	CodemodeModeOn            = settingspkg.CodemodeModeOn
	CodemodeModeOnly          = settingspkg.CodemodeModeOnly

	SessionShareDestinationAuto    = settingspkg.SessionShareDestinationAuto
	SessionShareDestinationGist    = settingspkg.SessionShareDestinationGist
	SessionShareDestinationRadius  = settingspkg.SessionShareDestinationRadius
	FullscreenExitOutputTranscript = settingspkg.FullscreenExitOutputTranscript
	FullscreenExitOutputResumeHint = settingspkg.FullscreenExitOutputResumeHint
	TuiModeRegular                 = settingspkg.TuiModeRegular
	TuiModeFullscreen              = settingspkg.TuiModeFullscreen
	MermaidRenderingStreaming      = settingspkg.MermaidRenderingStreaming
	MermaidRenderingOff            = settingspkg.MermaidRenderingOff
	MermaidRenderingFinal          = settingspkg.MermaidRenderingFinal

	QuietStartupFalse  = settingspkg.QuietStartupFalse
	QuietStartupTrue   = settingspkg.QuietStartupTrue
	QuietStartupHeader = settingspkg.QuietStartupHeader
	OutputPadNone      = settingspkg.OutputPadNone
	OutputPadOne       = settingspkg.OutputPadOne
)

// DefaultCompactionConfig is the resolved compaction defaults.
var DefaultCompactionConfig = settingspkg.DefaultCompactionConfig

// DefaultBranchSummaryConfig is the resolved branch-summary defaults.
var DefaultBranchSummaryConfig = settingspkg.DefaultBranchSummaryConfig

// DefaultNpmCommand mirrors Pi's npm command fallback.
var DefaultNpmCommand = append([]string(nil), settingspkg.DefaultNpmCommand...)

// CacheWarmingModes lists the valid cache-warming modes in Pi's order.
var CacheWarmingModes = append([]string(nil), settingspkg.CacheWarmingModes...)

// DefaultToolNames is Pi's built-in default tool selection.
func DefaultToolNames() []string { return settingspkg.DefaultToolNames() }

// Settings mirrors Pi's settings.json.
type Settings = settingspkg.Settings

// SettingsManager is the global and project settings view.
type SettingsManager = settingspkg.Manager

// SettingsOptions are the options of NewSettingsManagerWithOptions.
type SettingsOptions = settingspkg.Options

// SettingsManagerCreateOptions is Pi's name for SettingsOptions.
type SettingsManagerCreateOptions = settingspkg.SettingsManagerCreateOptions

// SettingsManagerOptions is another name for SettingsOptions.
type SettingsManagerOptions = settingspkg.Options

// SettingsError is one settings load or write failure.
type SettingsError = settingspkg.Error

// CompactionSettings, BranchSummarySettings and the other nested settings
// types mirror Pi's JSON shape.
type (
	CompactionSettings      = settingspkg.CompactionSettings
	CompactionSettingsJSON  = settingspkg.CompactionSettingsJSON
	CompactionModelOverride = settingspkg.CompactionModelOverride
	BranchSummarySettings   = settingspkg.BranchSummarySettings
	RetrySettings           = settingspkg.RetrySettings
	RetrySettingsJSON       = settingspkg.RetrySettingsJSON
	ProviderRetrySettings   = settingspkg.ProviderRetrySettings
	CodemodeSettings        = settingspkg.CodemodeSettings
	ThinkingBudgetsSettings = settingspkg.ThinkingBudgetsSettings
	MarkdownSettings        = settingspkg.MarkdownSettings
	WarningsSettings        = settingspkg.WarningsSettings
	WarningSettings         = settingspkg.WarningSettings
	TerminalSettings        = settingspkg.TerminalSettings
	ImageSettings           = settingspkg.ImageSettings
	CompactionConfig        = settingspkg.CompactionConfig
	BranchSummaryConfig     = settingspkg.BranchSummaryConfig
	RetryConfig             = settingspkg.RetryConfig
	ProviderRetryConfig     = settingspkg.ProviderRetryConfig
	QuietStartup            = settingspkg.QuietStartup
	OutputPad               = settingspkg.OutputPad
	DefaultProjectTrust     = settingspkg.DefaultProjectTrust
	SettingsStorage         = settingspkg.SettingsStorage
	PackageSource           = settingspkg.PackageSource
	CacheWarmingMode        = settingspkg.CacheWarmingMode
	CodemodeMode            = settingspkg.CodemodeMode
	TransportSetting        = settingspkg.TransportSetting
	SessionShareDestination = settingspkg.SessionShareDestination
	FullscreenExitOutput    = settingspkg.FullscreenExitOutput
	TuiMode                 = settingspkg.TuiMode
	MermaidRenderingMode    = settingspkg.MermaidRenderingMode
	WheelScrollLines        = settingspkg.WheelScrollLines
)

// NewSettingsManager loads global and project settings. Project settings are
// trusted by default, as Pi's SettingsManager.create does; use
// NewSettingsManagerWithProjectTrust to pass the host's decision.
func NewSettingsManager(cwd, agentDir string) *SettingsManager {
	return settingspkg.New(cwd, agentDir)
}

// CreateSettingsManager is NewSettingsManager under Pi's create name.
func CreateSettingsManager(cwd, agentDir string) *SettingsManager {
	return settingspkg.New(cwd, agentDir)
}

// NewSettingsManagerWithOptions loads settings with explicit options.
func NewSettingsManagerWithOptions(cwd, agentDir string, options SettingsOptions) *SettingsManager {
	return settingspkg.NewWithOptions(cwd, agentDir, options)
}

// NewSettingsManagerWithProjectTrust loads settings and reads the project
// layer only when trusted.
func NewSettingsManagerWithProjectTrust(cwd, agentDir string, trusted bool) *SettingsManager {
	return settingspkg.NewWithOptions(cwd, agentDir, settingspkg.Options{ProjectTrusted: &trusted})
}

// NewInMemorySettingsManager returns settings that never touch a file.
func NewInMemorySettingsManager(settings Settings) *SettingsManager {
	return settingspkg.NewInMemory(settings)
}

// NewSettingsManagerFromStorage creates a manager backed by Pi's
// SettingsStorage instead of settings.json files.
func NewSettingsManagerFromStorage(storage SettingsStorage, options SettingsManagerCreateOptions) *SettingsManager {
	return settingspkg.NewSettingsManagerFromStorage(storage, options)
}

// ApplyToolModifiers applies +name/-name entries to a tool list.
func ApplyToolModifiers(base, entries []string) []string {
	return settingspkg.ApplyToolModifiers(base, entries)
}

// ResolveDefaultTools resolves a defaultTools list.
func ResolveDefaultTools(entries []string) []string {
	return settingspkg.ResolveDefaultTools(entries)
}

// ThemeOverride returns ApplyOverrides input that selects theme, preserving an
// explicitly empty name.
func ThemeOverride(theme string) Settings { return settingspkg.ThemeOverride(theme) }

// ProjectDir returns the project configuration directory (.nulo).
func ProjectDir(cwd string) string { return settingspkg.ProjectDir(cwd) }

// ProjectConfigDir is ProjectDir under PiG's name.
func ProjectConfigDir(cwd string) string { return settingspkg.ProjectConfigDir(cwd) }
