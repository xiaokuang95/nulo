package sdk

import (
	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

type Message = ai.Message
type Model = ai.Model
type Event = agent.Event
type Plan = agent.Plan
type PlanStep = agent.PlanStep
type ContentBlock = ai.ContentBlock
type AgentTool = agent.AgentTool
type ToolResult = agent.ToolResult
type StopReason = ai.StopReason
type ThinkingLevel = ai.ThinkingLevel
type Usage = ai.Usage
type SessionInfo = session.Info
type SessionTreeNode = session.TreeNode
type SessionEntry = session.Entry
type Cost = ai.Cost
type CostTier = ai.CostTier
type StreamFn = ai.StreamFn
type StreamOptions = ai.StreamOptions
type Transcript = ai.Transcript
type AssistantMessageEvent = ai.AssistantMessageEvent
type EventStream = ai.EventStream

// NewEventStream creates the channel-backed stream a Provider implementation
// returns. The provider pushes AssistantMessageEvent values and ends the stream
// with a final message.
func NewEventStream() *EventStream { return ai.NewEventStream() }

const (
	StopStop    = ai.StopStop
	StopError   = ai.StopError
	StopAborted = ai.StopAborted
	StopToolUse = ai.StopToolUse
	StopLength  = ai.StopLength
)

const (
	ThinkingOff     = ai.ThinkingOff
	ThinkingMinimal = ai.ThinkingMinimal
	ThinkingLow     = ai.ThinkingLow
	ThinkingMedium  = ai.ThinkingMedium
	ThinkingHigh    = ai.ThinkingHigh
	ThinkingXHigh   = ai.ThinkingXHigh
	ThinkingMax     = ai.ThinkingMax
)
