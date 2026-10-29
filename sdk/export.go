package sdk

import (
	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

type Message = ai.Message
type Model = ai.Model
type Event = agent.Event
type ContentBlock = ai.ContentBlock
type Tool = agent.AgentTool
type StopReason = ai.StopReason

const (
	StopStop    = ai.StopStop
	StopError   = ai.StopError
	StopAborted = ai.StopAborted
	StopToolUse = ai.StopToolUse
	StopLength  = ai.StopLength
)
