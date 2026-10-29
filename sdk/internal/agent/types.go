package agent

import (
	"context"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

type Event struct {
	Type                  string                    `json:"type"`
	Message               *ai.Message               `json:"message,omitempty"`
	Messages              []ai.Message              `json:"messages,omitempty"`
	AssistantMessageEvent *ai.AssistantMessageEvent `json:"assistantMessageEvent,omitempty"`
	ToolResults           []ai.Message              `json:"toolResults,omitempty"`
	ToolCallID            string                    `json:"toolCallId,omitempty"`
	ToolName              string                    `json:"toolName,omitempty"`
	Args                  map[string]any            `json:"args,omitempty"`
	PartialResult         any                       `json:"partialResult,omitempty"`
	Result                any                       `json:"result,omitempty"`
	IsError               bool                      `json:"isError,omitempty"`
	DurationMs            *int                      `json:"durationMs,omitempty"`
}

type ToolResult struct {
	Content   []ai.ContentBlock
	Details   any
	IsError   bool
	Terminate bool
}

type AgentTool struct {
	ai.Tool
	Label         string
	ExecutionMode string
	Execute       func(ctx context.Context, toolCallID string, args map[string]any) (ToolResult, error)
}

type Context struct {
	Messages []ai.Message
	Tools    []AgentTool
}

type BeforeToolCallResult struct {
	Block     bool
	Reason    string
	Terminate bool
}

type AfterToolCallResult struct {
	Content   []ai.ContentBlock
	Details   any
	IsError   *bool
	Terminate *bool
}

type TurnDecision struct {
	Action string // "", "continue", "end"
}

type TurnContext struct {
	Message     ai.Message
	ToolResults []ai.Message
	Context     Context
	NewMessages []ai.Message
}

type Config struct {
	Model           ai.Model
	ThinkingLevel   ai.ThinkingLevel
	ToolExecution   string // "parallel" (default) or "sequential"
	ConvertToLlm    func([]ai.Message) []ai.Message
	StreamFn        ai.StreamFn
	GetAPIKey       func(provider string) string
	BeforeToolCall  func(ctx context.Context, assistant ai.Message, call ai.ContentBlock, args map[string]any) (*BeforeToolCallResult, error)
	AfterToolCall   func(ctx context.Context, assistant ai.Message, call ai.ContentBlock, args map[string]any, result ToolResult, isError bool) (*AfterToolCallResult, error)
	FinishTurn      func(ctx context.Context, turn TurnContext) *TurnDecision
	PrepareRequest  func(ctx context.Context, c Context, model ai.Model) Context
	PrepareNextTurn func(ctx context.Context, turn TurnContext) *TurnUpdate
	GetSteering     func() []ai.Message
	GetFollowUp     func() []ai.Message
	StreamOptions   ai.StreamOptions
}

type TurnUpdate struct {
	Context       *Context
	Messages      []ai.Message
	Model         *ai.Model
	ThinkingLevel *ai.ThinkingLevel
}

type EventSink func(Event)
