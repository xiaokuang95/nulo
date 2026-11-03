// Package settings is Pi's settings.json model: a global layer in the agent
// directory, an optional trusted project layer, and a merged view with the
// accessors the session uses.
package settings

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
)

// Defaults Pi applies when a settings key is absent.
const (
	DefaultCompactionEnabled          = true
	DefaultCompactionReserveTokens    = 16384
	DefaultCompactionKeepRecentTokens = 20000
	DefaultBranchSummaryReserveTokens = 16384
	DefaultRetryMaxRetries            = 3
	DefaultRetryBaseDelayMs           = 2000
	DefaultMaxAgentDelayMs            = 60000
	DefaultProviderRetryMaxDelayMs    = 60000
	DefaultCacheWarming               = "streaming"
	DefaultTransport                  = "auto"
	DefaultQueueMode                  = "one-at-a-time"
	DefaultEnableSkillCommands        = true
	DefaultShowImages                 = true
	DefaultImageWidthCells            = 60
	DefaultImageAutoResize            = true
	DefaultDoubleEscapeAction         = "tree"
	DefaultTreeFilterMode             = "default"
	DefaultCodeBlockIndent            = "  "
	DefaultCodemodeMode               = "on"
	DefaultCodemodeInlineBudget       = 3000
	DefaultHTTPIdleTimeoutMs          = 300000
)

// DefaultProjectTrust is the project-trust mode string.
type DefaultProjectTrust string

// The valid DefaultProjectTrust values.
const (
	DefaultProjectTrustAsk    DefaultProjectTrust = "ask"
	DefaultProjectTrustAlways DefaultProjectTrust = "always"
	DefaultProjectTrustNever  DefaultProjectTrust = "never"
)

// DefaultNpmCommand mirrors Pi's npm command fallback.
var DefaultNpmCommand = []string{"npm"}

// QuietStartup is Pi's boolean | "header" setting.
type QuietStartup uint8

const (
	QuietStartupFalse QuietStartup = iota
	QuietStartupTrue
	QuietStartupHeader
)

func (q QuietStartup) String() string {
	switch q {
	case QuietStartupTrue:
		return "true"
	case QuietStartupHeader:
		return "header"
	default:
		return "false"
	}
}

func (q QuietStartup) MarshalJSON() ([]byte, error) {
	if q == QuietStartupHeader {
		return []byte(`"header"`), nil
	}
	if q == QuietStartupTrue {
		return []byte(`true`), nil
	}
	return []byte(`false`), nil
}

func (q *QuietStartup) UnmarshalJSON(data []byte) error {
	switch strings.TrimSpace(string(data)) {
	case "true":
		*q = QuietStartupTrue
	case `"header"`:
		*q = QuietStartupHeader
	default:
		*q = QuietStartupFalse
	}
	return nil
}

// Settings mirrors Pi's settings.json. Unknown keys are kept in Extra so a
// settings file written by a newer Pi survives a read/write cycle.
type Settings struct {
	DefaultProvider      string            `json:"defaultProvider,omitempty"`
	DefaultModel         string            `json:"defaultModel,omitempty"`
	DefaultThinkingLevel string            `json:"defaultThinkingLevel,omitempty"`
	ModelThinkingLevels  map[string]string `json:"modelThinkingLevels,omitempty"`
	EnabledModels        []string          `json:"enabledModels,omitzero"`
	DefaultTools         []string          `json:"defaultTools,omitzero"`

	Compaction    *CompactionSettings    `json:"compaction,omitempty"`
	BranchSummary *BranchSummarySettings `json:"branchSummary,omitempty"`
	Retry         *RetrySettings         `json:"retry,omitempty"`

	CacheWarming              string `json:"cacheWarming,omitempty"`
	SessionDir                string `json:"sessionDir,omitempty"`
	ExternalEditor            string `json:"externalEditor,omitempty"`
	CollapseChangelog         bool   `json:"collapseChangelog,omitempty"`
	EnableInstallTelemetry    *bool  `json:"enableInstallTelemetry,omitempty"`
	EnableAnalytics           *bool  `json:"enableAnalytics,omitempty"`
	TrackingID                string `json:"trackingId,omitempty"`
	EditorPaddingX            *int   `json:"editorPaddingX,omitempty"`
	OutputPad                 *int   `json:"outputPad,omitempty"`
	AutocompleteMaxVisible    *int   `json:"autocompleteMaxVisible,omitempty"`
	ShowHardwareCursor        *bool  `json:"showHardwareCursor,omitempty"`
	HTTPIdleTimeoutMs         *int   `json:"-"`
	WebSocketConnectTimeoutMs *int   `json:"-"`
	ShellPath                 string `json:"shellPath,omitempty"`
	ShellCommandPrefix        string `json:"shellCommandPrefix,omitempty"`
	// CommandPrefix is PiG's name for ShellCommandPrefix. It is not a
	// separate on-disk key; MarshalJSON folds it into shellCommandPrefix.
	CommandPrefix string   `json:"-"`
	NpmCommand    []string `json:"npmCommand,omitzero"`
	HTTPProxy     string   `json:"httpProxy,omitempty"`

	Packages   []PackageSource `json:"packages,omitzero"`
	Extensions []string        `json:"extensions,omitzero"`
	Skills     []string        `json:"skills,omitzero"`
	Prompts    []string        `json:"prompts,omitzero"`
	Themes     []string        `json:"themes,omitzero"`

	Codemode *CodemodeSettings `json:"codemode,omitempty"`

	Transport               string `json:"transport,omitempty"`
	SteeringMode            string `json:"steeringMode,omitempty"`
	FollowUpMode            string `json:"followUpMode,omitempty"`
	SessionShareDestination string `json:"sessionShareDestination,omitempty"`
	TuiMode                 string `json:"tuiMode,omitempty"`
	FullscreenExitOutput    string `json:"fullscreenExitOutput,omitempty"`
	FullscreenScrollbar     string `json:"fullscreenScrollbar,omitempty"`
	FullscreenCopyOnSelect  *bool  `json:"fullscreenCopyOnSelect,omitempty"`
	// FullscreenWheelScrollLines is a line count or "auto"; nil is unset.
	FullscreenWheelScrollLines  json.RawMessage `json:"fullscreenWheelScrollLines,omitempty"`
	MaskSecretInput             *bool           `json:"maskSecretInput,omitempty"`
	ExtensionsAutoUpgrade       *bool           `json:"extensionsAutoUpgrade,omitempty"`
	ExtensionsTypescriptRuntime *string         `json:"extensionsTypescriptRuntime,omitempty"`
	Theme                       string          `json:"theme,omitempty"`
	HideThinkingBlock           bool            `json:"hideThinkingBlock,omitempty"`

	ShowCacheMissNotices bool              `json:"showCacheMissNotices,omitempty"`
	ShowImages           *bool             `json:"showImages,omitempty"`
	ImageWidthCells      *int              `json:"imageWidthCells,omitempty"`
	ImageAutoResize      *bool             `json:"imageAutoResize,omitempty"`
	BlockImages          *bool             `json:"blockImages,omitempty"`
	ClearOnShrink        *bool             `json:"clearOnShrink,omitempty"`
	ShowTerminalProgress *bool             `json:"showTerminalProgress,omitempty"`
	EnableSkillCommands  *bool             `json:"enableSkillCommands,omitempty"`
	Terminal             *TerminalSettings `json:"terminal,omitempty"`
	Images               *ImageSettings    `json:"images,omitempty"`

	LastChangelogVersion string                   `json:"lastChangelogVersion,omitempty"`
	DefaultProjectTrust  string                   `json:"defaultProjectTrust,omitempty"`
	DoubleEscapeAction   string                   `json:"doubleEscapeAction,omitempty"`
	TreeFilterMode       string                   `json:"treeFilterMode,omitempty"`
	DeviceID             string                   `json:"deviceId,omitempty"`
	QuietStartup         QuietStartup             `json:"-"`
	ThinkingBudgets      *ThinkingBudgetsSettings `json:"thinkingBudgets,omitempty"`
	Markdown             *MarkdownSettings        `json:"markdown,omitempty"`
	Warnings             *WarningsSettings        `json:"warnings,omitempty"`

	// Extra holds top-level members this package does not model.
	Extra map[string]json.RawMessage `json:"-"`

	// Presence flags keep values that omitempty would otherwise drop: an
	// authored empty string, an empty theme, and an explicit false.
	emptyStringFields map[string]bool
	themePresent      bool
	themeNonString    bool
	themeNull         bool
	presentBools      map[string]bool
	httpIdleRaw       json.RawMessage
	webSocketRaw      json.RawMessage
	quietStartupRaw   json.RawMessage
	quietStartupSet   bool
	failedFields      map[string]json.RawMessage
}

// CompactionSettings is compaction's JSON object.
type CompactionSettings struct {
	Enabled          *bool                              `json:"enabled,omitempty"`
	ReserveTokens    *int                               `json:"reserveTokens,omitempty"`
	KeepRecentTokens *int                               `json:"keepRecentTokens,omitempty"`
	ModelOverrides   map[string]CompactionModelOverride `json:"modelOverrides,omitempty"`
}

// CompactionModelOverride overrides token settings for one exact
// "provider/modelId".
type CompactionModelOverride struct {
	ReserveTokens    *int `json:"reserveTokens,omitempty"`
	KeepRecentTokens *int `json:"keepRecentTokens,omitempty"`
}

// BranchSummarySettings is branchSummary's JSON object.
type BranchSummarySettings struct {
	ReserveTokens *int  `json:"reserveTokens,omitempty"`
	SkipPrompt    *bool `json:"skipPrompt,omitempty"`
}

// RetrySettings is retry's JSON object.
type RetrySettings struct {
	Enabled         *bool                  `json:"enabled,omitempty"`
	MaxRetries      *int                   `json:"maxRetries,omitempty"`
	BaseDelayMs     *int                   `json:"baseDelayMs,omitempty"`
	MaxAgentDelayMs *int                   `json:"maxAgentDelayMs,omitempty"`
	Provider        *ProviderRetrySettings `json:"provider,omitempty"`
}

// ProviderRetrySettings is retry.provider.
type ProviderRetrySettings struct {
	TimeoutMs       *int `json:"timeoutMs,omitempty"`
	MaxRetries      *int `json:"maxRetries,omitempty"`
	MaxRetryDelayMs *int `json:"maxRetryDelayMs,omitempty"`
}

// CodemodeSettings is codemode's JSON object.
type CodemodeSettings struct {
	Mode         string `json:"mode,omitempty"`
	InlineBudget *int   `json:"inlineBudget,omitempty"`
}

// TerminalSettings is terminal's JSON object.
type TerminalSettings struct {
	ShowImages           *bool           `json:"showImages,omitempty"`
	ImageWidthCells      *int            `json:"imageWidthCells,omitempty"`
	ClearOnShrink        *bool           `json:"clearOnShrink,omitempty"`
	ShowTerminalProgress *bool           `json:"showTerminalProgress,omitempty"`
	Hyperlinks           json.RawMessage `json:"hyperlinks,omitempty"`
	Images               json.RawMessage `json:"images,omitempty"`
	TrueColor            json.RawMessage `json:"trueColor,omitempty"`
}

// ImageSettings is images' JSON object.
type ImageSettings struct {
	AutoResize  *bool `json:"autoResize,omitempty"`
	BlockImages *bool `json:"blockImages,omitempty"`
}

// ThinkingBudgetsSettings maps thinking levels to token budgets.
type ThinkingBudgetsSettings struct {
	Minimal *int `json:"minimal,omitempty"`
	Low     *int `json:"low,omitempty"`
	Medium  *int `json:"medium,omitempty"`
	High    *int `json:"high,omitempty"`
}

// MarkdownSettings is markdown's JSON object.
type MarkdownSettings struct {
	CodeBlockIndent *string `json:"codeBlockIndent,omitempty"`
	Mermaid         string  `json:"mermaid,omitempty"`

	codeBlockIndentNull bool
}

type markdownSettingsWire struct {
	CodeBlockIndent *string `json:"codeBlockIndent,omitempty"`
	Mermaid         string  `json:"mermaid,omitempty"`
}

// UnmarshalJSON tracks an authored null codeBlockIndent. A null reads as the
// default but must survive a load/write round trip.
func (m *MarkdownSettings) UnmarshalJSON(data []byte) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return err
	}
	var wire markdownSettingsWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*m = MarkdownSettings{CodeBlockIndent: wire.CodeBlockIndent, Mermaid: wire.Mermaid}
	if raw, ok := members["codeBlockIndent"]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		m.codeBlockIndentNull = true
	}
	return nil
}

// MarshalJSON preserves an authored null codeBlockIndent.
func (m MarkdownSettings) MarshalJSON() ([]byte, error) {
	encoded, err := marshalJSONValue(markdownSettingsWire{
		CodeBlockIndent: m.CodeBlockIndent,
		Mermaid:         m.Mermaid,
	})
	if err != nil {
		return nil, err
	}
	if !m.codeBlockIndentNull {
		return encoded, nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil {
		return nil, err
	}
	if _, ok := members["codeBlockIndent"]; !ok {
		members["codeBlockIndent"] = json.RawMessage("null")
	}
	return marshalJSONValue(members)
}

// WarningsSettings is warnings' JSON object.
type WarningsSettings struct {
	AnthropicExtraUsage    *bool `json:"anthropicExtraUsage,omitempty"`
	anthropicExtraUsageSet bool
}

// CompactionConfig is the resolved compaction configuration.
type CompactionConfig struct {
	Enabled          bool
	ReserveTokens    int
	KeepRecentTokens int
}

// BranchSummaryConfig is the resolved branch summary configuration.
type BranchSummaryConfig struct {
	ReserveTokens int
	SkipPrompt    bool
}

// RetryConfig is the resolved agent retry configuration.
type RetryConfig struct {
	Enabled         bool
	MaxRetries      int
	BaseDelayMs     int
	MaxAgentDelayMs int
}

// ProviderRetryConfig is the resolved provider retry configuration.
type ProviderRetryConfig struct {
	TimeoutMs       *int
	MaxRetries      *int
	MaxRetryDelayMs int
}

// plainSettings is Settings without its custom JSON methods, used to decode
// one top-level member at a time so one wrong-typed value does not discard the
// rest of the file.
type plainSettings Settings

// terminalWire folds the legacy flat terminal/image fields into Pi's nested
// terminal and images objects, and removes the top-level duplicates.
func (s *Settings) terminalWire() {
	if s.Terminal == nil && (s.ShowImages != nil || s.ImageWidthCells != nil || s.ClearOnShrink != nil || s.ShowTerminalProgress != nil) {
		s.Terminal = &TerminalSettings{}
	}
	if s.Terminal != nil {
		terminal := *s.Terminal
		if terminal.ShowImages == nil {
			terminal.ShowImages = s.ShowImages
		}
		if terminal.ImageWidthCells == nil {
			terminal.ImageWidthCells = s.ImageWidthCells
		}
		if terminal.ClearOnShrink == nil {
			terminal.ClearOnShrink = s.ClearOnShrink
		}
		if terminal.ShowTerminalProgress == nil {
			terminal.ShowTerminalProgress = s.ShowTerminalProgress
		}
		s.Terminal = &terminal
		s.ShowImages = nil
		s.ImageWidthCells = nil
		s.ClearOnShrink = nil
		s.ShowTerminalProgress = nil
	}
	if s.Images == nil && (s.ImageAutoResize != nil || s.BlockImages != nil) {
		s.Images = &ImageSettings{}
	}
	if s.Images != nil {
		images := *s.Images
		if images.AutoResize == nil {
			images.AutoResize = s.ImageAutoResize
		}
		if images.BlockImages == nil {
			images.BlockImages = s.BlockImages
		}
		s.Images = &images
		s.ImageAutoResize = nil
		s.BlockImages = nil
	}
}

// MarshalJSON writes known settings plus the unknown members in Extra.
func (s Settings) MarshalJSON() ([]byte, error) {
	if s.ShellCommandPrefix == "" && s.CommandPrefix != "" {
		s.ShellCommandPrefix = s.CommandPrefix
	}
	s.terminalWire()
	encoded, err := marshalJSONValue(plainSettings(s))
	if err != nil {
		return nil, err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil {
		return nil, err
	}
	// Re-add values omitted only because they were empty or false.
	for key, present := range s.emptyStringFields {
		if present {
			if _, exists := members[key]; !exists {
				members[key] = json.RawMessage(`""`)
			}
		}
	}
	if s.themeNull {
		if _, exists := members["theme"]; !exists {
			members["theme"] = json.RawMessage(`null`)
		}
	} else if s.themePresent && s.Theme == "" && !s.themeNonString {
		if _, exists := members["theme"]; !exists {
			members["theme"] = json.RawMessage(`""`)
		}
	}
	for key, present := range s.presentBools {
		if present {
			if _, exists := members[key]; !exists {
				members[key] = json.RawMessage(`false`)
			}
		}
	}
	if raw := quietStartupWire(s.QuietStartup, s.quietStartupRaw, s.quietStartupSet); len(raw) > 0 {
		members["quietStartup"] = raw
	}
	if raw := timeoutWire(s.HTTPIdleTimeoutMs, s.httpIdleRaw, DefaultHTTPIdleTimeoutMs); len(raw) > 0 {
		members["httpIdleTimeoutMs"] = raw
	}
	if raw := timeoutWire(s.WebSocketConnectTimeoutMs, s.webSocketRaw, 0); len(raw) > 0 {
		members["websocketConnectTimeoutMs"] = raw
	}
	for key, value := range s.failedFields {
		if _, exists := members[key]; !exists {
			members[key] = value
		}
	}
	for key, value := range s.Extra {
		if _, exists := members[key]; !exists {
			members[key] = value
		}
	}
	return marshalSettingsMembers(members)
}

// marshalSettingsMembers writes settings in Pi's schema order, appending
// unknown members in lexical order.
func marshalSettingsMembers(members map[string]json.RawMessage) ([]byte, error) {
	keys := make([]string, 0, len(members))
	for key := range members {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		left, right := settingsKeyRank(keys[i]), settingsKeyRank(keys[j])
		if left != right {
			return left < right
		}
		return keys[i] < keys[j]
	})
	tree := &jsonValue{object: []jsonMember{}}
	for _, key := range keys {
		value, err := parseJSONValue(members[key])
		if err != nil {
			value = &jsonValue{raw: members[key]}
		}
		tree.set(key, value)
	}
	return tree.MarshalJSON()
}

// UnmarshalJSON keeps members this package does not model in Extra.
func (s *Settings) UnmarshalJSON(data []byte) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return err
	}
	migrateSettings(members)
	// Pi has no top-level commandPrefix, showImages, imageWidthCells,
	// clearOnShrink, imageAutoResize or blockImages. They are unknown members
	// that survive a save but never feed the terminal/images getters.
	flatUnknown := map[string]json.RawMessage{}
	for _, key := range []string{"commandPrefix", "showImages", "imageWidthCells", "clearOnShrink", "imageAutoResize", "blockImages", "showTerminalProgress"} {
		if raw, ok := members[key]; ok {
			flatUnknown[key] = append(json.RawMessage(nil), raw...)
			delete(members, key)
		}
	}
	var decoded plainSettings
	var failedFields map[string]json.RawMessage
	plainType := reflect.TypeOf(decoded)
	decodedValue := reflect.ValueOf(&decoded).Elem()
	for i := 0; i < plainType.NumField(); i++ {
		field := plainType.Field(i)
		tag := field.Tag.Get("json")
		key, _, _ := strings.Cut(tag, ",")
		if key == "" || key == "-" {
			continue
		}
		raw, ok := members[key]
		if !ok {
			continue
		}
		single := append([]byte(`{"`+key+`":`), raw...)
		single = append(single, '}')
		holder := reflect.New(plainType).Elem()
		if err := json.Unmarshal(single, holder.Addr().Interface()); err != nil {
			if isStructuredSettingKey(key) {
				if failedFields == nil {
					failedFields = map[string]json.RawMessage{}
				}
				failedFields[key] = append(json.RawMessage(nil), raw...)
			}
			continue
		}
		decodedValue.Field(i).Set(holder.Field(i))
		if key == "compaction" && compactionNeedsRaw(raw) {
			decodedValue.Field(i).Set(reflect.Zero(field.Type))
			if failedFields == nil {
				failedFields = map[string]json.RawMessage{}
			}
			failedFields["compaction"] = append(json.RawMessage(nil), raw...)
		}
	}
	if raw, ok := members["packages"]; ok {
		if items, valid := decodePackageSources(raw); valid {
			decoded.Packages = items
		}
	}
	if decoded.Terminal != nil {
		if decoded.ShowImages == nil {
			decoded.ShowImages = decoded.Terminal.ShowImages
		}
		if decoded.ImageWidthCells == nil {
			decoded.ImageWidthCells = decoded.Terminal.ImageWidthCells
		}
		if decoded.ClearOnShrink == nil {
			decoded.ClearOnShrink = decoded.Terminal.ClearOnShrink
		}
		if decoded.ShowTerminalProgress == nil {
			decoded.ShowTerminalProgress = decoded.Terminal.ShowTerminalProgress
		}
	}
	if decoded.Images != nil {
		if decoded.ImageAutoResize == nil {
			decoded.ImageAutoResize = decoded.Images.AutoResize
		}
		if decoded.BlockImages == nil {
			decoded.BlockImages = decoded.Images.BlockImages
		}
	}
	*s = Settings(decoded)
	s.failedFields = failedFields
	if s.ShellCommandPrefix != "" {
		s.CommandPrefix = s.ShellCommandPrefix
	}
	for key := range emptyStringOverrides {
		raw, ok := members[key]
		if !ok {
			continue
		}
		var text string
		if json.Unmarshal(raw, &text) == nil {
			if s.emptyStringFields == nil {
				s.emptyStringFields = map[string]bool{}
			}
			s.emptyStringFields[key] = true
		}
	}
	if raw, ok := members["theme"]; ok {
		s.themePresent = true
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			s.themeNull = true
		} else {
			var text string
			if json.Unmarshal(raw, &text) != nil {
				s.themeNonString = true
			}
		}
	}
	for key, raw := range members {
		if !isBoolSettingKey(key) {
			continue
		}
		var value bool
		if json.Unmarshal(raw, &value) == nil {
			if s.presentBools == nil {
				s.presentBools = map[string]bool{}
			}
			s.presentBools[key] = true
		}
	}
	if raw, ok := members["quietStartup"]; ok {
		s.quietStartupRaw = append(json.RawMessage(nil), raw...)
		s.quietStartupSet = true
		_ = json.Unmarshal(raw, &s.QuietStartup)
	}
	if raw, ok := members["httpIdleTimeoutMs"]; ok {
		s.httpIdleRaw = append(json.RawMessage(nil), raw...)
		if parsed, valid := parseTimeoutRaw(raw); valid {
			s.HTTPIdleTimeoutMs = &parsed
		}
	}
	if raw, ok := members["websocketConnectTimeoutMs"]; ok {
		s.webSocketRaw = append(json.RawMessage(nil), raw...)
		if parsed, valid := parseTimeoutRaw(raw); valid {
			s.WebSocketConnectTimeoutMs = &parsed
		}
	}
	delete(members, "quietStartup")
	delete(members, "httpIdleTimeoutMs")
	delete(members, "websocketConnectTimeoutMs")
	for _, key := range settingKeys() {
		delete(members, key)
	}
	for key, raw := range flatUnknown {
		members[key] = raw
	}
	if len(members) > 0 {
		s.Extra = members
	}
	return nil
}

var boolSettingKeys = map[string]bool{
	"hideThinkingBlock":    true,
	"showCacheMissNotices": true,
	"collapseChangelog":    true,
	"enableSkillCommands":  true,
	"showImages":           true,
	"imageAutoResize":      true,
	"blockImages":          true,
	"clearOnShrink":        true,
	"showTerminalProgress": true,
	"quietStartup":         true,
}

func isBoolSettingKey(key string) bool { return boolSettingKeys[key] }

var settingKeyList = func() []string {
	var keys []string
	t := reflect.TypeOf(Settings{})
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}()

func settingKeys() []string { return settingKeyList }

func settingsToRaw(s Settings) map[string]json.RawMessage {
	encoded, err := marshalJSONValue(s)
	if err != nil {
		return map[string]json.RawMessage{}
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil || members == nil {
		return map[string]json.RawMessage{}
	}
	// JSON omitempty cannot tell an assigned empty slice from an absent one.
	// Settings setters and UpdateGlobal do: preserve explicit empty arrays.
	emptyArrays := []struct {
		key    string
		values int
		set    bool
	}{
		{"packages", len(s.Packages), s.Packages != nil},
		{"extensions", len(s.Extensions), s.Extensions != nil},
		{"skills", len(s.Skills), s.Skills != nil},
		{"prompts", len(s.Prompts), s.Prompts != nil},
		{"themes", len(s.Themes), s.Themes != nil},
		{"enabledModels", len(s.EnabledModels), s.EnabledModels != nil},
		{"defaultTools", len(s.DefaultTools), s.DefaultTools != nil},
		{"npmCommand", len(s.NpmCommand), s.NpmCommand != nil},
	}
	for _, item := range emptyArrays {
		if item.set && item.values == 0 {
			members[item.key] = json.RawMessage(`[]`)
		}
	}
	if s.ModelThinkingLevels != nil && len(s.ModelThinkingLevels) == 0 {
		members["modelThinkingLevels"] = json.RawMessage(`{}`)
	}
	return members
}

func decodeSettings(raw map[string]json.RawMessage) Settings {
	encoded, err := marshalJSONValue(raw)
	if err != nil {
		return Settings{}
	}
	var s Settings
	if err := json.Unmarshal(encoded, &s); err != nil {
		return Settings{}
	}
	return s
}

// DefaultToolNames is Pi's DEFAULT_TOOL_NAMES.
func DefaultToolNames() []string {
	return []string{"read", "bash", "edit", "write"}
}

// IsToolModifier reports whether entry is a +name or -name tool modifier.
func IsToolModifier(entry string) bool {
	return len(entry) > 0 && (entry[0] == '+' || entry[0] == '-')
}

// MergeDefaultTools merges two defaultTools layers: a list with a plain tool
// replaces the inherited list, while a list of only modifiers appends.
func MergeDefaultTools(base, overrides []string) []string {
	if overrides == nil {
		return cloneStrings(base)
	}
	if base == nil {
		return cloneStrings(overrides)
	}
	for _, entry := range overrides {
		if !IsToolModifier(entry) {
			return cloneStrings(overrides)
		}
	}
	return append(cloneStrings(base), overrides...)
}

// ResolveDefaultTools applies plain replacements and +name/-name modifiers.
func ResolveDefaultTools(entries []string) []string {
	plain := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !IsToolModifier(entry) {
			plain = append(plain, entry)
		}
	}
	base := plain
	if len(plain) == 0 && len(entries) != 0 {
		base = DefaultToolNames()
	}
	return ApplyToolModifiers(base, entries)
}

// ApplyToolModifiers applies +name/-name entries to base in order.
func ApplyToolModifiers(base, entries []string) []string {
	tools := cloneStrings(base)
	if tools == nil {
		tools = []string{}
	}
	for _, entry := range entries {
		if !IsToolModifier(entry) {
			continue
		}
		name := entry[1:]
		index := -1
		for i, tool := range tools {
			if tool == name {
				index = i
				break
			}
		}
		switch {
		case entry[0] == '+' && index == -1 && name != "":
			tools = append(tools, name)
		case entry[0] == '-' && index >= 0:
			tools = append(tools[:index], tools[index+1:]...)
		}
	}
	return tools
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// PiG's JSON type names.
type (
	CompactionSettingsJSON  = CompactionSettings
	RetrySettingsJSON       = RetrySettings
	CacheWarmingMode        = string
	CodemodeMode            = string
	TransportSetting        = string
	WarningSettings         = WarningsSettings
	SessionShareDestination = string
	FullscreenExitOutput    = string
	TuiMode                 = string
	MermaidRenderingMode    = string
)

// Pi's enum values.
const (
	CacheWarmingOff       = "off"
	CacheWarmingStreaming = "streaming"
	CacheWarmingIdle      = "idle"

	CodemodeModeOn   = "on"
	CodemodeModeOnly = "only"

	SessionShareDestinationAuto    = "auto"
	SessionShareDestinationGist    = "gist"
	SessionShareDestinationRadius  = "radius"
	FullscreenExitOutputTranscript = "transcript"
	FullscreenExitOutputResumeHint = "resume-hint"
	TuiModeRegular                 = "regular"
	TuiModeFullscreen              = "fullscreen"
	MermaidRenderingStreaming      = "streaming"
	MermaidRenderingOff            = "off"
	MermaidRenderingFinal          = "final"
)

// WheelScrollLines is a line count, or Auto for velocity-based scrolling.
type WheelScrollLines struct {
	Auto  bool
	Lines float64
}

// timeoutWire picks the authored timeout spelling when it still matches the
// typed value, otherwise the typed value; an invalid authored value survives.
func timeoutWire(value *int, raw json.RawMessage, _ int) json.RawMessage {
	if len(raw) > 0 {
		if parsed, valid := parseTimeoutRaw(raw); valid {
			if value == nil || *value == parsed {
				return append(json.RawMessage(nil), raw...)
			}
		} else if value == nil {
			return append(json.RawMessage(nil), raw...)
		}
	}
	if value == nil {
		return nil
	}
	encoded, _ := marshalJSONValue(*value)
	return encoded
}

// quietStartupWire preserves an authored false or an unusual authored value;
// otherwise it writes the typed value or omits it.
func quietStartupWire(value QuietStartup, raw json.RawMessage, present bool) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 {
		switch string(trimmed) {
		case "true", "false", `"header"`, "null":
			if value == QuietStartupFalse && string(trimmed) == "null" {
				return append(json.RawMessage(nil), trimmed...)
			}
			return quietStartupJSON(value)
		default:
			if value == QuietStartupFalse {
				return append(json.RawMessage(nil), trimmed...)
			}
		}
	}
	if value == QuietStartupFalse {
		if present {
			return json.RawMessage("false")
		}
		return nil
	}
	return quietStartupJSON(value)
}

func quietStartupJSON(value QuietStartup) json.RawMessage {
	switch value {
	case QuietStartupTrue:
		return json.RawMessage("true")
	case QuietStartupHeader:
		return json.RawMessage(`"header"`)
	default:
		return json.RawMessage("false")
	}
}

// isStructuredSettingKey lists settings whose typed decode failure should keep
// the authored raw value, so runtime getters can still validate it.
func isStructuredSettingKey(key string) bool {
	switch key {
	case "compaction", "branchSummary", "retry", "terminal", "images",
		"codemode", "markdown", "warnings", "thinkingBudgets", "packages":
		return true
	}
	return false
}

// compactionNeedsRaw keeps authored nulls (which Pi's getter rejects) and
// malformed model-override entries that a typed struct cannot represent.
func compactionNeedsRaw(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return true
	}
	for _, key := range []string{"reserveTokens", "keepRecentTokens"} {
		if value, ok := object[key]; ok && isJSONNull(value) {
			return true
		}
	}
	if overrides, ok := object["modelOverrides"]; ok && !isJSONNull(overrides) {
		var entries map[string]json.RawMessage
		if json.Unmarshal(overrides, &entries) != nil {
			return true
		}
		for _, entry := range entries {
			trimmed := bytes.TrimSpace(entry)
			if len(trimmed) == 0 || trimmed[0] != '{' {
				return true
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(entry, &fields) != nil {
				return true
			}
			for _, key := range []string{"reserveTokens", "keepRecentTokens"} {
				if value, ok := fields[key]; ok && isJSONNull(value) {
					return true
				}
			}
		}
	}
	return false
}

// DefaultCompactionConfig is the resolved compaction defaults.
var DefaultCompactionConfig = CompactionConfig{
	Enabled:          DefaultCompactionEnabled,
	ReserveTokens:    DefaultCompactionReserveTokens,
	KeepRecentTokens: DefaultCompactionKeepRecentTokens,
}

// DefaultBranchSummaryConfig is the resolved branch-summary defaults.
var DefaultBranchSummaryConfig = BranchSummaryConfig{
	ReserveTokens: DefaultBranchSummaryReserveTokens,
}

// GetToolListError validates a --tools list: either an allowlist of plain
// names and patterns, or only +name/-name entries with exact names. It returns
// the problem, or "" when the list is valid.
func GetToolListError(entries []string) string {
	modifiers := 0
	for _, entry := range entries {
		if IsToolModifier(entry) {
			modifiers++
		}
	}
	if modifiers == 0 {
		return ""
	}
	if modifiers < len(entries) {
		return "tool names cannot be mixed with +name or -name entries"
	}
	for _, entry := range entries {
		if strings.Contains(entry, "*") {
			return "+name and -name entries take exact tool names, not patterns: " + entry
		}
	}
	return ""
}

// CacheWarmingModes lists the valid cache-warming modes in Pi's order.
var CacheWarmingModes = []string{CacheWarmingOff, CacheWarmingStreaming, CacheWarmingIdle}
