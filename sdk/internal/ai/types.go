package ai

import (
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
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	API           string            `json:"api"`
	Provider      string            `json:"provider"`
	BaseURL       string            `json:"baseUrl"`
	Reasoning     bool              `json:"reasoning"`
	Input         []string          `json:"input,omitempty"`
	ContextWindow int               `json:"contextWindow"`
	MaxTokens     int               `json:"maxTokens"`
	Headers       map[string]string `json:"headers,omitempty"`
	Compat        Compat            `json:"compat,omitempty"`
	Cost          Cost              `json:"cost"`

	// APIKey is resolved when the model is built, from the provider's apiKey in
	// models.json, then auth.json, then the environment. It never reaches disk.
	APIKey string `json:"-"`
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
//
// Pi also documents reasoning-related compat (thinkingFormat,
// supportsReasoningEffort) and more; nulo does not send reasoning parameters yet,
// so they are deliberately absent rather than parsed-but-ignored.
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
}

func (m Message) ContentString() string {
	return contentRawText(m.Content)
}

func (m Message) ContentBlocks() []ContentBlock {
	return contentRawBlocks(m.Content)
}

func EncodeContentString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func EncodeContentBlocks(blocks []ContentBlock) json.RawMessage {
	if blocks == nil {
		blocks = []ContentBlock{}
	}
	b, _ := json.Marshal(blocks)
	return b
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
