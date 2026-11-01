package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// modelFacingLimit is Pi's MCP_OUTPUT_MAX_BYTES: text beyond this is cut in the
// middle and the full text is saved to a temp file the model can read.
const modelFacingLimit = 20 * 1024

type callToolResult struct {
	IsError bool           `json:"isError"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Data        string `json:"data"`
	MIMEType    string `json:"mimeType"`
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Resource    *struct {
		URI      string `json:"uri"`
		MIMEType string `json:"mimeType"`
		Text     string `json:"text"`
		Blob     string `json:"blob"`
	} `json:"resource"`
}

// convertResult turns a CallToolResult into model-facing content. Each block
// keeps its own content block, the way Pi's convertMcpResult flattens them, and
// only an over-long text output is joined and cut.
func convertResult(raw json.RawMessage) ([]ai.ContentBlock, bool) {
	var res callToolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		// A server that answers with something other than a result object still
		// gets its payload to the model rather than an empty tool output.
		return []ai.ContentBlock{ai.TextBlock(string(raw))}, false
	}

	var content []ai.ContentBlock
	for _, block := range res.Content {
		switch block.Type {
		case "text":
			content = append(content, ai.TextBlock(block.Text))
		case "image":
			if block.Data != "" {
				content = append(content, ai.ImageBlock(block.Data, block.MIMEType))
			}
		case "resource_link":
			content = append(content, ai.TextBlock(formatResourceLink(block)))
		case "resource":
			if block.Resource == nil {
				continue
			}
			switch {
			case strings.HasPrefix(block.Resource.MIMEType, "image/") && block.Resource.Blob != "":
				content = append(content, ai.ImageBlock(block.Resource.Blob, block.Resource.MIMEType))
			case block.Resource.Text != "":
				content = append(content, ai.TextBlock(block.Resource.Text))
			case block.Resource.Blob != "":
				content = append(content, ai.TextBlock(fmt.Sprintf(
					"[Binary resource %s (%s) not shown]", block.Resource.URI, block.Resource.MIMEType)))
			}
		}
	}
	return limitContent(content), res.IsError
}

func formatResourceLink(block contentBlock) string {
	label := block.Title
	if label == "" {
		label = block.Name
	}
	details := block.MIMEType
	if block.Description != "" {
		if details != "" {
			details += ", "
		}
		details += block.Description
	}
	if details != "" {
		return fmt.Sprintf("[Resource %s %q (%s)]", block.URI, label, details)
	}
	return fmt.Sprintf("[Resource %s %q]", block.URI, label)
}

// limitContent keeps the text within modelFacingLimit. Beyond it the text blocks
// are joined, cut in the middle, and the full text is saved to a temp file, so
// the model can read the rest; images follow the truncated text.
func limitContent(content []ai.ContentBlock) []ai.ContentBlock {
	var texts []string
	var images []ai.ContentBlock
	for _, block := range content {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		} else {
			images = append(images, block)
		}
	}
	joined := strings.Join(texts, "\n")
	if len(joined) <= modelFacingLimit {
		if len(content) == 0 {
			return []ai.ContentBlock{ai.TextBlock("")}
		}
		return content
	}

	truncated, _ := truncateMiddle(joined, modelFacingLimit)
	where := ""
	if path, err := saveOutput(joined); err == nil {
		where = fmt.Sprintf("[Full output: %s (read it with offset/limit)]", path)
	} else {
		where = fmt.Sprintf("[Could not save the full output: %v]", err)
	}
	head := fmt.Sprintf(
		"Warning: truncated output (original token count: %d)\nTotal output lines: %d\n\n%s\n\n%s",
		(len(joined)+3)/4, strings.Count(joined, "\n")+1, truncated, where)

	out := make([]ai.ContentBlock, 0, 1+len(images))
	out = append(out, ai.TextBlock(head))
	return append(out, images...)
}

// truncateMiddle keeps the start and the end, cutting the middle out, exactly
// like Pi's truncateMiddle: half the budget for each side, never splitting a
// UTF-8 rune.
func truncateMiddle(s string, maxBytes int) (string, int) {
	if len(s) <= maxBytes {
		return s, 0
	}
	headEnd := maxBytes / 2
	for headEnd > 0 && !utf8.RuneStart(s[headEnd]) {
		headEnd--
	}
	tailStart := len(s) - (maxBytes - maxBytes/2)
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}
	removed := utf8.RuneCountInString(s[headEnd:tailStart])
	return s[:headEnd] + fmt.Sprintf("…%d chars truncated…", removed) + s[tailStart:], removed
}

func saveOutput(s string) (string, error) {
	f, err := os.CreateTemp("", "nulo-mcp-*.txt")
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
