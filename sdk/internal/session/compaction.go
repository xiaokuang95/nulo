package session

import (
	"strings"
	"unicode/utf8"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

const (
	DefaultReserveTokens    = 16384
	DefaultKeepRecentTokens = 20000
	toolResultMaxChars      = 2000
)

func EstimateTokens(m ai.Message) int {
	n := utf8.RuneCountInString(ai.MessageText(m)) / 4
	if n < 1 && ai.MessageText(m) != "" {
		n = 1
	}
	return n
}

func ShouldCompact(tokens, contextWindow, reserve int) bool {
	if contextWindow <= 0 {
		return false
	}
	if reserve <= 0 {
		reserve = DefaultReserveTokens
	}
	return tokens > contextWindow-reserve
}

func FindCutIndex(path []Entry, keepRecent int) int {
	if keepRecent <= 0 {
		keepRecent = DefaultKeepRecentTokens
	}
	acc := 0
	cut := 0
	for i := len(path) - 1; i >= 0; i-- {
		msgs := entryMessages(path[i])
		n := 0
		for _, m := range msgs {
			n += EstimateTokens(m)
		}
		if n == 0 {
			continue
		}
		acc += n
		if acc >= keepRecent {
			cut = i
			break
		}
		cut = i
	}
	// don't cut toolResult
	for cut < len(path) {
		e := path[cut]
		if e.Type == "message" && e.Message != nil && e.Message.Role == "toolResult" {
			cut++
			continue
		}
		break
	}
	if cut >= len(path) {
		cut = len(path) - 1
	}
	return cut
}

func CanCompact(path []Entry) error {
	if len(path) == 0 {
		return errNothing
	}
	last := path[len(path)-1]
	if last.Type == "compaction" {
		return errAlready
	}
	cut := FindCutIndex(path, DefaultKeepRecentTokens)
	if cut <= 0 {
		return errNothing
	}
	return nil
}

var (
	errNothing = compactError("Nothing to compact (session too small)")
	errAlready = compactError("Already compacted")
)

type compactError string

func (e compactError) Error() string { return string(e) }

func SerializeConversation(msgs []ai.Message) string {
	var parts []string
	for _, m := range msgs {
		switch m.Role {
		case "user":
			if t := m.ContentString(); t != "" {
				parts = append(parts, "[User]: "+t)
			}
		case "assistant":
			var calls []string
			var texts []string
			for _, b := range m.ContentBlocks() {
				if b.Type == "toolCall" {
					calls = append(calls, b.Name)
				}
				if b.Type == "text" && b.Text != "" {
					texts = append(texts, b.Text)
				}
			}
			if len(texts) > 0 {
				parts = append(parts, "[Assistant]: "+strings.Join(texts, "\n"))
			}
			if len(calls) > 0 {
				parts = append(parts, "[Assistant tool calls]: "+strings.Join(calls, "; "))
			}
		case "toolResult":
			t := m.ContentString()
			if utf8.RuneCountInString(t) > toolResultMaxChars {
				t = string([]rune(t)[:toolResultMaxChars]) + "\n\n[truncated]"
			}
			if t != "" {
				parts = append(parts, "[Tool result]: "+t)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}
