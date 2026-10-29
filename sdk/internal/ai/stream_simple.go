package ai

import "context"

func StreamSimple(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
	switch model.API {
	case "openai-completions":
		return StreamOpenAICompletions(ctx, model, tr, opts)
	case "openai-responses":
		return StreamOpenAIResponses(ctx, model, tr, opts)
	case "anthropic-messages":
		return StreamAnthropicMessages(ctx, model, tr, opts)
	case "faux", "":
		if model.API == "" && model.Provider == "faux" {
			p := NewFauxProvider()
			return p.StreamSimple(ctx, model, tr, opts)
		}
		if model.API == "faux" {
			p := NewFauxProvider()
			return p.StreamSimple(ctx, model, tr, opts)
		}
		return errorStream(model, "unknown api: "+model.API, StopError)
	default:
		return errorStream(model, "unknown api: "+model.API, StopError)
	}
}

func StreamForDir(agentDir string) StreamFn {
	return func(ctx context.Context, model Model, tr Transcript, opts StreamOptions) *EventStream {
		if opts.APIKey == "" {
			opts.APIKey = LookupAPIKey(model.Provider, agentDir, opts)
		}
		return StreamSimple(ctx, model, tr, opts)
	}
}
