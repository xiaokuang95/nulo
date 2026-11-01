package codemode

import (
	"fmt"
	"os"
	"strings"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// Defaults from Pi's codemode: output is budgeted in estimated tokens and every
// four characters count as one token.
const (
	defaultMaxOutputTokens = 10_000
	charsPerToken          = 4
)

// OutputKind is one kind of script output.
type OutputKind string

const (
	OutputText    OutputKind = "text"
	OutputConsole OutputKind = "console"
	OutputImage   OutputKind = "image"
)

// OutputItem is one piece of script output as the prelude reported it.
type OutputItem struct {
	Kind OutputKind
	Text string // text and console
	Data string // base64 for images
	MIME string
}

// formatOutput lays the script's output out so the model can tell items apart.
// With more than one text item each gets a `==> text N/M <==` line, and console
// lines follow everything else in one `<console_output>` block.
func formatOutput(items []OutputItem) []ai.ContentBlock {
	total := 0
	for _, item := range items {
		if item.Kind == OutputText {
			total++
		}
	}
	var out []ai.ContentBlock
	var console []string
	index := 0
	for _, item := range items {
		switch item.Kind {
		case OutputImage:
			out = append(out, ai.ImageBlock(item.Data, item.MIME))
		case OutputConsole:
			console = append(console, item.Text)
		default:
			index++
			text := item.Text
			if total > 1 {
				text = fmt.Sprintf("==> text %d/%d <==\n%s", index, total, text)
			}
			out = append(out, ai.TextBlock(text))
		}
	}
	if len(console) > 0 {
		out = append(out, ai.TextBlock("<console_output>\n"+strings.Join(console, "\n")+"\n</console_output>"))
	}
	return out
}

// joinAdjacentText merges neighbouring text blocks into one, each part starting
// on its own line, and moves images after the text.
func joinAdjacentText(items []ai.ContentBlock) []ai.ContentBlock {
	joined := make([]ai.ContentBlock, 0, len(items))
	var run strings.Builder
	inRun := false
	flush := func() {
		if inRun {
			joined = append(joined, ai.TextBlock(run.String()))
			run.Reset()
			inRun = false
		}
	}
	for _, item := range items {
		if item.Type != "text" {
			flush()
			joined = append(joined, item)
			continue
		}
		if !inRun {
			inRun = true
			run.WriteString(item.Text)
			continue
		}
		if all := run.String(); all != "" && !strings.HasSuffix(all, "\n") {
			run.WriteByte('\n')
		}
		run.WriteString(item.Text)
	}
	flush()
	return joined
}

// truncateOutput keeps the script output within maxTokens estimated tokens.
// Beyond it the text is cut in the middle and the full text is saved to a file
// the model can read; images follow the truncated text.
func truncateOutput(items []ai.ContentBlock, maxTokens int64) ([]ai.ContentBlock, string) {
	var texts []string
	for _, item := range items {
		if item.Type == "text" {
			texts = append(texts, item.Text)
		}
	}
	combined := strings.Join(texts, "\n")
	budget := int(maxTokens) * charsPerToken
	if budget < 0 {
		budget = 0
	}
	runes := []rune(combined)
	if len(texts) == 0 || len(runes) <= budget {
		return items, ""
	}

	headChars := budget / 2
	tailChars := budget - headChars
	removed := len(runes) - headChars - tailChars
	head := string(runes[:headChars])
	tail := ""
	if tailChars > 0 {
		tail = string(runes[len(runes)-tailChars:])
	}
	text := fmt.Sprintf(
		"Warning: truncated output (original token count: %d)\nTotal output lines: %d\n\n%s…%d tokens truncated…%s",
		ceilDiv(len(runes), charsPerToken), strings.Count(combined, "\n")+1,
		head, ceilDiv(removed, charsPerToken), tail)

	path, err := spillOutput(combined)
	if err == nil {
		text += "\n\n[Full output: " + path + " (read with offset/limit)]"
	} else {
		text += "\n\n[Could not save the full output: " + err.Error() + "]"
	}

	out := []ai.ContentBlock{ai.TextBlock(text)}
	for _, item := range items {
		if item.Type != "text" {
			out = append(out, item)
		}
	}
	return out, path
}

func ceilDiv(a, b int) int { return (a + b - 1) / b }

func spillOutput(s string) (string, error) {
	f, err := os.CreateTemp("", "nulo-codemode-*.txt")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(s); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}
