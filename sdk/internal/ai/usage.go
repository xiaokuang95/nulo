package ai

import "encoding/json"

// Token accounting for the three streaming APIs, ported from Pi's
// calculateCost, parseChunkUsage (openai-completions), the message_start /
// message_delta usage handling (anthropic-messages) and finalizeResponse
// (openai-responses).
//
// Providers disagree about where cache tokens are reported and whether they are
// already included in the prompt count, so each API has its own mapping below.

// calculateCost fills usage.Cost from the model's per-million-token rates.
// Model.Cost holds rates; Usage.Cost holds the amounts computed from them. The
// highest tier whose threshold the whole prompt exceeds replaces every base
// rate, as Pi's calculateCost.
func calculateCost(model Model, u *Usage) {
	rates := model.Cost
	inputRate, outputRate, cacheReadRate, cacheWriteRate := rates.Input, rates.Output, rates.CacheRead, rates.CacheWrite
	inputTokens := u.Input + u.CacheRead + u.CacheWrite
	matched := -1.0
	for _, tier := range rates.Tiers {
		if float64(inputTokens) > tier.InputTokensAbove && tier.InputTokensAbove > matched {
			inputRate, outputRate = tier.Input, tier.Output
			cacheReadRate, cacheWriteRate = tier.CacheRead, tier.CacheWrite
			matched = tier.InputTokensAbove
		}
	}
	// Anthropic charges double the base input rate for 1h cache writes.
	longWrite := 0
	if u.CacheWrite1h != nil {
		longWrite = *u.CacheWrite1h
	}
	shortWrite := u.CacheWrite - longWrite

	u.Cost.Input = inputRate / 1e6 * float64(u.Input)
	u.Cost.Output = outputRate / 1e6 * float64(u.Output)
	u.Cost.CacheRead = cacheReadRate / 1e6 * float64(u.CacheRead)
	u.Cost.CacheWrite = (cacheWriteRate*float64(shortWrite) + inputRate*2*float64(longWrite)) / 1e6
	u.Cost.Total = u.Cost.Input + u.Cost.Output + u.Cost.CacheRead + u.Cost.CacheWrite
}

// decodeAs re-decodes a value the SSE reader already turned into any, so usage
// objects can be typed without re-parsing the whole payload. It reports false
// for an absent object, which is how the callers detect "no usage reported".
func decodeAs(v any, out any) bool {
	if v == nil {
		return false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, out) == nil
}

// oaiUsageFields is the usage object of a chat-completions chunk. Cache tokens
// arrive under several provider-specific names, hence the pointers: absent must
// stay distinguishable from an explicit zero.
type oaiUsageFields struct {
	PromptTokens         int  `json:"prompt_tokens"`
	CompletionTokens     int  `json:"completion_tokens"`
	PromptCacheHitTokens *int `json:"prompt_cache_hit_tokens"`
	CachedTokens         *int `json:"cached_tokens"`
	PromptTokensDetails  *struct {
		CachedTokens     *int `json:"cached_tokens"`
		CacheWriteTokens *int `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens *int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (r oaiUsageFields) cacheRead() int {
	// OpenAI/OpenRouter report prompt_tokens_details.cached_tokens, DeepSeek
	// prompt_cache_hit_tokens, and Kimi a top-level cached_tokens.
	switch {
	case r.PromptTokensDetails != nil && r.PromptTokensDetails.CachedTokens != nil:
		return *r.PromptTokensDetails.CachedTokens
	case r.PromptCacheHitTokens != nil:
		return *r.PromptCacheHitTokens
	case r.CachedTokens != nil:
		return *r.CachedTokens
	}
	return 0
}

func (r oaiUsageFields) cacheWrite() int {
	if r.PromptTokensDetails != nil && r.PromptTokensDetails.CacheWriteTokens != nil {
		return *r.PromptTokensDetails.CacheWriteTokens
	}
	return 0
}

// parseChunkUsage mirrors Pi's parseChunkUsage. Cache reads and writes are
// counted separately and subtracted from the prompt count, because
// spec-compliant providers include them there; subtracting writes from
// cached_tokens would instead under-report them.
func parseChunkUsage(raw oaiUsageFields, model Model) *Usage {
	cacheRead := raw.cacheRead()
	cacheWrite := raw.cacheWrite()
	input := raw.PromptTokens - cacheRead - cacheWrite
	if input < 0 {
		input = 0
	}
	// completion_tokens already includes reasoning tokens.
	reasoning := 0
	if raw.CompletionTokensDetails != nil && raw.CompletionTokensDetails.ReasoningTokens != nil {
		reasoning = *raw.CompletionTokensDetails.ReasoningTokens
	}
	u := &Usage{
		Input:       input,
		Output:      raw.CompletionTokens,
		CacheRead:   cacheRead,
		CacheWrite:  cacheWrite,
		Reasoning:   &reasoning,
		TotalTokens: input + raw.CompletionTokens + cacheRead + cacheWrite,
	}
	calculateCost(model, u)
	return u
}

// anthropicUsageFields is Anthropic's usage object. Every field is a pointer so
// a message_delta reports only the counters it actually revised.
type anthropicUsageFields struct {
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheCreation            *struct {
		Ephemeral1hInputTokens *int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	OutputTokensDetails *struct {
		ThinkingTokens *int `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
}

// apply merges one Anthropic usage object into u, leaving fields the object
// omits untouched, then recomputes totals and cost as Pi does.
func (f anthropicUsageFields) apply(u *Usage, model Model) {
	if f.InputTokens != nil {
		u.Input = *f.InputTokens
	}
	if f.OutputTokens != nil {
		u.Output = *f.OutputTokens
	}
	if f.CacheReadInputTokens != nil {
		u.CacheRead = *f.CacheReadInputTokens
	}
	if f.CacheCreationInputTokens != nil {
		u.CacheWrite = *f.CacheCreationInputTokens
	}
	if f.CacheCreation != nil && f.CacheCreation.Ephemeral1hInputTokens != nil {
		u.CacheWrite1h = f.CacheCreation.Ephemeral1hInputTokens
	}
	if f.OutputTokensDetails != nil && f.OutputTokensDetails.ThinkingTokens != nil {
		u.Reasoning = f.OutputTokensDetails.ThinkingTokens
	}
	u.TotalTokens = u.Input + u.Output + u.CacheRead + u.CacheWrite
	calculateCost(model, u)
}

// responsesUsageFields is the usage object of a completed Responses payload.
// Unlike chat completions, input_tokens includes cached and cache-write tokens.
type responsesUsageFields struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails *struct {
		CachedTokens     *int `json:"cached_tokens"`
		CacheWriteTokens *int `json:"cache_write_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails *struct {
		ReasoningTokens *int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// toUsage mirrors Pi's finalizeResponse mapping. The provider's own
// total_tokens is kept when it sends one, so the value is not recomputed.
func (r responsesUsageFields) toUsage(model Model) *Usage {
	cacheRead, cacheWrite := 0, 0
	if r.InputTokensDetails != nil {
		if r.InputTokensDetails.CachedTokens != nil {
			cacheRead = *r.InputTokensDetails.CachedTokens
		}
		if r.InputTokensDetails.CacheWriteTokens != nil {
			cacheWrite = *r.InputTokensDetails.CacheWriteTokens
		}
	}
	input := r.InputTokens - cacheRead - cacheWrite
	if input < 0 {
		input = 0
	}
	reasoning := 0
	if r.OutputTokensDetails != nil && r.OutputTokensDetails.ReasoningTokens != nil {
		reasoning = *r.OutputTokensDetails.ReasoningTokens
	}
	u := &Usage{
		Input:       input,
		Output:      r.OutputTokens,
		CacheRead:   cacheRead,
		CacheWrite:  cacheWrite,
		Reasoning:   &reasoning,
		TotalTokens: r.TotalTokens,
	}
	calculateCost(model, u)
	return u
}
