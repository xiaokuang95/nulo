package sdk

import (
	"context"

	"github.com/xiaokuang95/nulo/sdk/internal/ai"
)

// ChatStream runs the OpenAI-compatible Chat Completions protocol. Provider
// plugins can use it directly when their supplier speaks that wire format.
func ChatStream(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
	return ai.StreamOpenAICompletions(ctx, model, tr, opts)
}

// ResponsesStream runs the OpenAI Responses protocol.
func ResponsesStream(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
	return ai.StreamOpenAIResponses(ctx, model, tr, opts)
}

// MessagesStream runs the Anthropic Messages protocol.
func MessagesStream(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
	return ai.StreamAnthropicMessages(ctx, model, tr, opts)
}
