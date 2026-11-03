package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

const CompactionSummaryPrefix = `The conversation history before this point was compacted into the following summary:

<summary>
`

const CompactionSummarySuffix = `
</summary>`

const BranchSummaryPrefix = `The following is a summary of a branch that this conversation came back from:

<summary>
`

const BranchSummarySuffix = `</summary>`

const SummarizationSystemPrompt = `You are a context summarization assistant. Your task is to read a conversation between a user and an AI assistant, then produce a structured summary following the exact format specified.

Do NOT continue the conversation. Do NOT respond to any questions in the conversation. ONLY output the structured summary.`

func ConvertToLlm(msgs []ai.Message) []ai.Message {
	var out []ai.Message
	for _, m := range msgs {
		switch m.Role {
		case "system", "user", "assistant", "toolResult":
			out = append(out, m)
		case "compactionSummary":
			text := CompactionSummaryPrefix + m.ContentString() + CompactionSummarySuffix
			out = append(out, ai.UserMessage(text, m.Timestamp))
		case "branchSummary":
			text := BranchSummaryPrefix + m.ContentString() + BranchSummarySuffix
			out = append(out, ai.UserMessage(text, m.Timestamp))
		case "custom":
			// Pi keeps a custom message's content blocks and gives them the
			// user role for the provider.
			content := m.Content
			if content == nil {
				content = ai.EncodeContentBlocks(nil)
			}
			out = append(out, ai.Message{Role: "user", Timestamp: m.Timestamp, Content: content})
		case "bashExecution":
			if m.ExcludeFromContext != nil && *m.ExcludeFromContext {
				continue
			}
			out = append(out, ai.UserMessage(bashExecutionText(m), m.Timestamp))
		}
	}
	return out
}

// bashExecutionText renders a user shell invocation as Pi's
// bashExecutionToText does.
func bashExecutionText(m ai.Message) string {
	var b strings.Builder
	b.WriteString("Ran `" + m.Command + "`\n")
	if m.Output != "" {
		b.WriteString("```\n" + m.Output + "\n```")
	} else {
		b.WriteString("(no output)")
	}
	if m.Cancelled {
		b.WriteString("\n\n(command cancelled)")
	} else if m.ExitCode != nil && *m.ExitCode != 0 {
		b.WriteString(fmt.Sprintf("\n\nCommand exited with code %d", *m.ExitCode))
	}
	if m.Truncated && m.FullOutputPath != "" {
		b.WriteString("\n\n[Output truncated. Full output: " + m.FullOutputPath + "]")
	}
	return b.String()
}

func compactionMessage(summary string, ts int64) ai.Message {
	return ai.Message{Role: "compactionSummary", Timestamp: ts, Content: ai.EncodeContentString(summary)}
}

func branchSummaryMessage(summary string, ts int64) ai.Message {
	return ai.Message{Role: "branchSummary", Timestamp: ts, Content: ai.EncodeContentString(summary)}
}

func isoTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func parseISO(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}
