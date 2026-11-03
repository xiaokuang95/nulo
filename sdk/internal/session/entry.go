package session

import (
	"bytes"
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

	Summary             string          `json:"summary,omitempty"`
	FirstKeptEntryID    string          `json:"firstKeptEntryId,omitempty"`
	FirstKeptEntryIndex json.RawMessage `json:"firstKeptEntryIndex,omitempty"`
	TokensBefore        int             `json:"tokensBefore,omitempty"`
	Details             json.RawMessage `json:"details,omitempty"`
	SystemMessage       *ai.Message     `json:"systemMessage,omitempty"`
	FromID              string          `json:"fromId,omitempty"`
	FromHook            *bool           `json:"fromHook,omitempty"`

	CustomType string          `json:"customType,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	Content    json.RawMessage `json:"content,omitempty"`
	Display    *bool           `json:"display,omitempty"`

	TargetID    string                  `json:"targetId,omitempty"`
	Label       *string                 `json:"label,omitempty"`
	Replacement *ContextEditReplacement `json:"replacement,omitempty"`

	Kind  string    `json:"kind,omitempty"`
	Model string    `json:"model,omitempty"`
	Usage *ai.Usage `json:"usage,omitempty"`
	Note  string    `json:"note,omitempty"`
}

// ContextEditReplacement is a context_edit's non-null replacement: the JSON
// string or content-block array that replaces the target's content.
type ContextEditReplacement struct {
	Content json.RawMessage `json:"content"`
}

// MarshalJSON keeps the context_edit replacement member present when it is
// null: null omits the target from model context, while an absent member is
// not a context_edit Pi accepts. JSON is written without HTML escaping, as
// Pi's JSON.stringify writes it.
func (e Entry) MarshalJSON() ([]byte, error) {
	type plain Entry
	switch e.Type {
	case "context_edit":
		return marshalSessionJSON(struct {
			plain
			Replacement *ContextEditReplacement `json:"replacement"`
		}{plain: plain(e), Replacement: e.Replacement})
	case "compaction":
		return marshalSessionJSON(struct {
			plain
			TokensBefore int `json:"tokensBefore"`
		}{plain: plain(e), TokensBefore: e.TokensBefore})
	case "session_info":
		// Pi always writes the name member, including an empty one that
		// clears a previously set name.
		return marshalSessionJSON(struct {
			plain
			Name string `json:"name"`
		}{plain: plain(e), Name: e.Name})
	}
	return marshalSessionJSON(plain(e))
}

// marshalSessionJSON encodes one session value the way JSON.stringify does:
// '<', '>' and '&' stay literal and the result has no trailing newline.
func marshalSessionJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
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
