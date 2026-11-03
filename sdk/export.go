package sdk

import (
	"github.com/xiaokuang95/nulo/sdk/internal/agent"
	"github.com/xiaokuang95/nulo/sdk/internal/ai"
	"github.com/xiaokuang95/nulo/sdk/internal/session"
)

type Message = ai.Message
type Model = ai.Model
type Event = agent.Event
type ContentBlock = ai.ContentBlock
type Tool = agent.AgentTool
type StopReason = ai.StopReason
type ThinkingLevel = ai.ThinkingLevel
type Usage = ai.Usage
type SessionInfo = session.Info
type SessionTreeNode = session.TreeNode
type Cost = ai.Cost
type CostTier = ai.CostTier

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
