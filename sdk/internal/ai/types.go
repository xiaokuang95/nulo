package ai

import (
	"bytes"
	"encoding/json"
	"strings"
)

type StopReason string

const (
	StopPending  StopReason = "pending"
	StopStop     StopReason = "stop"
	StopLength   StopReason = "length"
	StopToolUse  StopReason = "toolUse"
	StopError    StopReason = "error"
	StopAborted  StopReason = "aborted"
	StopDeferred StopReason = "deferred"
)

type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
	ThinkingMax     ThinkingLevel = "max"
)

type Cost struct {
	Input             float64 `json:"input"`
	Output            float64 `json:"output"`
	CacheRead         float64 `json:"cacheRead"`
	CacheWrite        float64 `json:"cacheWrite"`
	Total             float64 `json:"total"`
	TotalFromProvider bool    `json:"totalFromProvider,omitempty"`
	// Tiers are the request-wide pricing tiers from models.json. The highest
	// threshold the whole prompt exceeds replaces every base rate.
	Tiers []CostTier `json:"tiers,omitempty"`
}

// CostTier is one context-length pricing tier. Rates are USD per million
// tokens, as Pi's ModelCost.tiers.
type CostTier struct {
	InputTokensAbove float64 `json:"inputTokensAbove"`
	Input            float64 `json:"input"`
	Output           float64 `json:"output"`
	CacheRead        float64 `json:"cacheRead"`
	CacheWrite       float64 `json:"cacheWrite"`
}

type Usage struct {
	Input        int  `json:"input"`
	Output       int  `json:"output"`
	CacheRead    int  `json:"cacheRead"`
	CacheWrite   int  `json:"cacheWrite"`
	CacheWrite1h *int `json:"cacheWrite1h,omitempty"`
	Reasoning    *int `json:"reasoning,omitempty"`
	TotalTokens  int  `json:"totalTokens"`
	Cost         Cost `json:"cost"`
}

func ZeroUsage() Usage {
	return Usage{Cost: Cost{}}
}

type ContentBlock struct {
	Type              string         `json:"type"`
	Text              string         `json:"text,omitempty"`
	TextSignature     string         `json:"textSignature,omitempty"`
	Thinking          string         `json:"thinking,omitempty"`
	ThinkingSignature string         `json:"thinkingSignature,omitempty"`
	Redacted          bool           `json:"redacted,omitempty"`
	Data              string         `json:"data,omitempty"`
	MIMEType          string         `json:"mimeType,omitempty"`
	ID                string         `json:"id,omitempty"`
	Name              string         `json:"name,omitempty"`
	Arguments         map[string]any `json:"arguments,omitempty"`
	ThoughtSignature  string         `json:"thoughtSignature,omitempty"`
	Namespace         string         `json:"namespace,omitempty"`
	ArgumentsError    string         `json:"argumentsError,omitempty"`
}

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

func ThinkingBlock(thinking string) ContentBlock {
	return ContentBlock{Type: "thinking", Thinking: thinking}
}

func ToolCallBlock(id, name string, args map[string]any) ContentBlock {
	if args == nil {
		args = map[string]any{}
	}
	return ContentBlock{Type: "toolCall", ID: id, Name: name, Arguments: args}
}

func ImageBlock(data, mime string) ContentBlock {
	return ContentBlock{Type: "image", Data: data, MIMEType: mime}
}

type Tool struct {
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	Parameters          json.RawMessage `json:"parameters"`
	ConstrainedSampling any             `json:"constrainedSampling,omitempty"`
}

type ToolRef struct {
	Name string `json:"name"`
}

type Model struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	API           string   `json:"api"`
	Provider      string   `json:"provider"`
	BaseURL       string   `json:"baseUrl"`
	Reasoning     bool     `json:"reasoning"`
	Input         []string `json:"input,omitempty"`
	ContextWindow int      `json:"contextWindow"`
	MaxTokens     int      `json:"maxTokens"`
	// ThinkingLevelMap names the provider value sent for each thinking level; a
	// nil value marks the level unsupported, and an absent xhigh or max is
	// unsupported too.
	ThinkingLevelMap map[ThinkingLevel]*string `json:"thinkingLevelMap,omitempty"`
	// SamplingParams are request-body fields merged into every OpenAI-compatible
	// request; SamplingParamsByThinkingLevel overrides them per level.
	SamplingParams                map[string]any                   `json:"samplingParams,omitempty"`
	SamplingParamsByThinkingLevel map[ThinkingLevel]map[string]any `json:"samplingParamsByThinkingLevel,omitempty"`
	// InputLimits describes provider image limits and the resize profile applied
	// before an image is sent.
	InputLimits *InputLimits `json:"inputLimits,omitempty"`
	// PromptCache is the provider's best-effort prompt-cache lifetime in seconds
	// for the short and long retention tiers.
	PromptCache map[string]float64 `json:"promptCache,omitempty"`
	// Headers are the provider/model header templates from models.json;
	// RequestHeaders resolves them for each request.
	Headers map[string]string `json:"headers,omitempty"`
	Compat  Compat            `json:"compat,omitempty"`
	Cost    Cost              `json:"cost"`
	// AuthHeader sends the resolved key as Authorization: Bearer as well, as
	// models.json's authHeader does.
	AuthHeader bool `json:"authHeader,omitempty"`
	// OAuth names the provider's OAuth flow ("radius"); its credential comes
	// from auth.json and is refreshed at the gateway when it expires.
	OAuth string `json:"oauth,omitempty"`

	// APIKey is the provider's apiKey template from models.json; LookupAPIKey
	// resolves it per request. It never reaches disk.
	APIKey string `json:"-"`
}

// InputLimits carries a model's provider input limits.
type InputLimits struct {
	// MaxRequestBytes is the maximum serialized provider request size.
	MaxRequestBytes int               `json:"maxRequestBytes,omitempty"`
	Images          *ImageInputLimits `json:"images,omitempty"`
}

// ImageInputLimits describes provider image limits and the cache-safe resize
// profile applied before an image enters a request.
type ImageInputLimits struct {
	Resize        *ImageResizeOptions `json:"resize,omitempty"`
	MaxPerMessage int                 `json:"maxPerMessage,omitempty"`
	MaxPerRequest int                 `json:"maxPerRequest,omitempty"`
}

// ImageResizeOptions is the resize profile for a model's images. Zero values
// mean the field is omitted and Pi's conservative default applies.
type ImageResizeOptions struct {
	MaxWidth    int `json:"maxWidth,omitempty"`
	MaxHeight   int `json:"maxHeight,omitempty"`
	MaxBytes    int `json:"maxBytes,omitempty"`
	JPEGQuality int `json:"jpegQuality,omitempty"`
}

// MaxOutputTokens is the request's output-token cap. The caller's value wins,
// then the model's own, matching Pi's fallback chain.
func (m Model) MaxOutputTokens(opts StreamOptions) int {
	if opts.MaxTokens > 0 {
		return opts.MaxTokens
	}
	return m.MaxTokens
}

// Compat describes verified differences from an API's standard request behavior,
// mirroring the fields of Pi's models.json "compat" block that nulo acts on. Each
// flag is a pointer so an unset value is distinguishable from an explicit false,
// and the accessors below supply Pi's defaults. A model-level value overrides the
// provider-level one field by field.
type Compat struct {
	// MaxTokensField selects max_tokens or max_completion_tokens for
	// openai-completions. Defaults to "max_tokens".
	MaxTokensField string `json:"maxTokensField,omitempty"`
	// SupportsStore sends store:false when the provider accepts the field.
	SupportsStore *bool `json:"supportsStore,omitempty"`
	// SupportsDeveloperRole sends the developer role instead of system.
	SupportsDeveloperRole *bool `json:"supportsDeveloperRole,omitempty"`
	// SupportsUsageInStreaming requests stream_options.include_usage.
	SupportsUsageInStreaming *bool `json:"supportsUsageInStreaming,omitempty"`
	// RequiresToolResultName adds the name field to tool result messages.
	RequiresToolResultName *bool `json:"requiresToolResultName,omitempty"`
	// SupportsMaxOutputTokens sends max_output_tokens (openai-responses).
	SupportsMaxOutputTokens *bool `json:"supportsMaxOutputTokens,omitempty"`
	// SupportsReasoningEffort sends reasoning_effort. Defaults to true.
	SupportsReasoningEffort *bool `json:"supportsReasoningEffort,omitempty"`
	// ThinkingFormat selects how thinking is requested on openai-completions.
	// Empty means "openai" (reasoning_effort).
	ThinkingFormat string `json:"thinkingFormat,omitempty"`
	// ThinkingTokenBudgetField names the top-level field that caps reasoning
	// tokens: "thinking_token_budget", "thinking_budget" or
	// "thinking_budget_tokens".
	ThinkingTokenBudgetField string `json:"thinkingTokenBudgetField,omitempty"`
	// SupportsThinkingTokenBudget is the vLLM alias of ThinkingTokenBudgetField.
	SupportsThinkingTokenBudget *bool `json:"supportsThinkingTokenBudget,omitempty"`
	// ChatTemplateKwargs is sent as chat_template_kwargs with the "chat-template"
	// thinking format; ChatTemplateArgs as chat_template_args with "baseten".
	ChatTemplateKwargs map[string]any `json:"chatTemplateKwargs,omitempty"`
	ChatTemplateArgs   map[string]any `json:"chatTemplateArgs,omitempty"`
	// ForceAdaptiveThinking asks Anthropic for adaptive thinking plus
	// output_config.effort instead of a token budget.
	ForceAdaptiveThinking *bool `json:"forceAdaptiveThinking,omitempty"`
}

func (c Compat) flag(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// TokensField is the body field carrying the output-token cap.
func (c Compat) TokensField() string {
	if c.MaxTokensField == "max_completion_tokens" {
		return "max_completion_tokens"
	}
	return "max_tokens"
}

func (c Compat) Store() bool            { return c.flag(c.SupportsStore, false) }
func (c Compat) UsageInStreaming() bool { return c.flag(c.SupportsUsageInStreaming, true) }
func (c Compat) ToolResultName() bool   { return c.flag(c.RequiresToolResultName, false) }
func (c Compat) MaxOutputTokens() bool  { return c.flag(c.SupportsMaxOutputTokens, true) }
func (c Compat) ReasoningEffort() bool  { return c.flag(c.SupportsReasoningEffort, true) }
func (c Compat) AdaptiveThinking() bool { return c.flag(c.ForceAdaptiveThinking, false) }

// DeveloperRole reports whether to use the developer role. The default differs
// per API: openai-responses supports it, most openai-completions endpoints do not.
func (c Compat) DeveloperRole(def bool) bool { return c.flag(c.SupportsDeveloperRole, def) }

// Override merges other over c, field by field; a field set in other wins.
func (c Compat) Override(other Compat) Compat {
	if other.MaxTokensField != "" {
		c.MaxTokensField = other.MaxTokensField
	}
	if other.SupportsStore != nil {
		c.SupportsStore = other.SupportsStore
	}
	if other.SupportsDeveloperRole != nil {
		c.SupportsDeveloperRole = other.SupportsDeveloperRole
	}
	if other.SupportsUsageInStreaming != nil {
		c.SupportsUsageInStreaming = other.SupportsUsageInStreaming
	}
	if other.RequiresToolResultName != nil {
		c.RequiresToolResultName = other.RequiresToolResultName
	}
	if other.SupportsMaxOutputTokens != nil {
		c.SupportsMaxOutputTokens = other.SupportsMaxOutputTokens
	}
	if other.SupportsReasoningEffort != nil {
		c.SupportsReasoningEffort = other.SupportsReasoningEffort
	}
	if other.ThinkingFormat != "" {
		c.ThinkingFormat = other.ThinkingFormat
	}
	if other.ThinkingTokenBudgetField != "" {
		c.ThinkingTokenBudgetField = other.ThinkingTokenBudgetField
	}
	if other.SupportsThinkingTokenBudget != nil {
		c.SupportsThinkingTokenBudget = other.SupportsThinkingTokenBudget
	}
	if other.ChatTemplateKwargs != nil {
		c.ChatTemplateKwargs = other.ChatTemplateKwargs
	}
	if other.ChatTemplateArgs != nil {
		c.ChatTemplateArgs = other.ChatTemplateArgs
	}
	if other.ForceAdaptiveThinking != nil {
		c.ForceAdaptiveThinking = other.ForceAdaptiveThinking
	}
	return c
}

type Message struct {
	Role      string `json:"role"`
	Timestamp int64  `json:"timestamp"`

	Content      json.RawMessage    `json:"content,omitempty"`
	Sections     map[string]*string `json:"sections,omitempty"`
	ToolsAdded   []Tool             `json:"toolsAdded,omitempty"`
	ToolsRemoved []ToolRef          `json:"toolsRemoved,omitempty"`

	API           string     `json:"api,omitempty"`
	Provider      string     `json:"provider,omitempty"`
	Model         string     `json:"model,omitempty"`
	ThinkingLevel string     `json:"thinkingLevel,omitempty"`
	Usage         *Usage     `json:"usage,omitempty"`
	StopReason    StopReason `json:"stopReason,omitempty"`
	ErrorMessage  string     `json:"errorMessage,omitempty"`
	DurationMs    *int       `json:"durationMs,omitempty"`

	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	IsError    bool   `json:"isError,omitempty"`

	// Projected custom_message entries (role "custom") carry their type and
	// whether the host should render them.
	CustomType string          `json:"customType,omitempty"`
	Display    bool            `json:"display,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`

	// Bash execution messages (role "bashExecution") persist a user shell
	// invocation as Pi does. Other roles leave these empty.
	Command            string `json:"command,omitempty"`
	Output             string `json:"output,omitempty"`
	ExitCode           *int   `json:"exitCode,omitempty"`
	Cancelled          bool   `json:"cancelled,omitempty"`
	Truncated          bool   `json:"truncated,omitempty"`
	FullOutputPath     string `json:"fullOutputPath,omitempty"`
	ExcludeFromContext *bool  `json:"excludeFromContext,omitempty"`
}

func (m Message) ContentString() string {
	return contentRawText(m.Content)
}

func (m Message) ContentBlocks() []ContentBlock {
	return contentRawBlocks(m.Content)
}

func EncodeContentString(s string) json.RawMessage {
	return encodeJSON(s)
}

func EncodeContentBlocks(blocks []ContentBlock) json.RawMessage {
	if blocks == nil {
		blocks = []ContentBlock{}
	}
	return encodeJSON(blocks)
}

// encodeJSON marshals v without HTML escaping, as Pi's JSON.stringify writes
// content: a session holding "<tag>" keeps the literal text.
func encodeJSON(value any) json.RawMessage {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'})
}

func contentRawText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	blocks := contentRawBlocks(raw)
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func contentRawBlocks(raw json.RawMessage) []ContentBlock {
	if len(raw) == 0 {
		return nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return []ContentBlock{TextBlock(s)}
	}
	return nil
}

func UserMessage(text string, ts int64) Message {
	return Message{Role: "user", Timestamp: ts, Content: EncodeContentString(text)}
}

func AssistantMessage(blocks []ContentBlock, model Model, reason StopReason, ts int64) Message {
	u := ZeroUsage()
	return Message{
		Role:       "assistant",
		Timestamp:  ts,
		Content:    EncodeContentBlocks(blocks),
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		Usage:      &u,
		StopReason: reason,
	}
}

func ToolResultMessage(callID, name string, blocks []ContentBlock, isError bool, ts int64) Message {
	return Message{
		Role:       "toolResult",
		Timestamp:  ts,
		ToolCallID: callID,
		ToolName:   name,
		Content:    EncodeContentBlocks(blocks),
		IsError:    isError,
	}
}

func SystemMessage(content string, ts int64) Message {
	return Message{Role: "system", Timestamp: ts, Content: EncodeContentString(content)}
}
