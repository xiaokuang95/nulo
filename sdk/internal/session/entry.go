package session

import (
	"encoding/json"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

const CurrentVersion = 3

type Header struct {
	Type          string `json:"type"`
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	ParentSession string `json:"parentSession,omitempty"`
}

type Entry struct {
	Type      string  `json:"type"`
	ID        string  `json:"id,omitempty"`
	ParentID  *string `json:"parentId"`
	Timestamp string  `json:"timestamp,omitempty"`

	Message *ai.Message `json:"message,omitempty"`

	Provider      string `json:"provider,omitempty"`
	ModelID       string `json:"modelId,omitempty"`
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
	Name          string `json:"name,omitempty"`

	Summary          string      `json:"summary,omitempty"`
	FirstKeptEntryID string      `json:"firstKeptEntryId,omitempty"`
	TokensBefore     int         `json:"tokensBefore,omitempty"`
	SystemMessage    *ai.Message `json:"systemMessage,omitempty"`
	FromID           string      `json:"fromId,omitempty"`

	CustomType string          `json:"customType,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	TargetID   string          `json:"targetId,omitempty"`
	Label      *string         `json:"label,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Model      string          `json:"model,omitempty"`
	Usage      *ai.Usage       `json:"usage,omitempty"`
	Note       string          `json:"note,omitempty"`
}

func parentPtr(id string) *string {
	if id == "" {
		return nil
	}
	p := id
	return &p
}

func parentID(e Entry) string {
	if e.ParentID == nil {
		return ""
	}
	return *e.ParentID
}
