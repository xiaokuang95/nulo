package ai

import (
	"maps"
	"slices"
	"strings"
)

// Thinking-level policy, ported from Pi's models.ts / simple-options.ts and the
// per-API thinking builders of openai-completions.ts, openai-responses.ts and
// anthropic-messages.ts.
const (
	minAnswerTokens        = 1024
	defaultThinkingMinimal = 1024
	defaultThinkingLow     = 2048
	defaultThinkingMedium  = 8192
	defaultThinkingHigh    = 16384
)

var allThinkingLevels = []ThinkingLevel{
	ThinkingOff,
	ThinkingMinimal,
	ThinkingLow,
	ThinkingMedium,
	ThinkingHigh,
	ThinkingXHigh,
	ThinkingMax,
}

// thinkingLevelSupported reports whether the model supports level. A model
// without reasoning supports only off; a level mapped to null is unsupported,
// and xhigh/max need an explicit non-null mapping.
func thinkingLevelSupported(model Model, level ThinkingLevel) bool {
	if !model.Reasoning {
		return level == ThinkingOff
	}
	mapped, ok := model.ThinkingLevelMap[level]
	if ok && mapped == nil {
		return false
	}
	if level == ThinkingXHigh || level == ThinkingMax {
		return ok && mapped != nil
	}
	return true
}

// SupportedThinkingLevels returns the levels the model offers, in order.
func SupportedThinkingLevels(model Model) []ThinkingLevel {
	levels := make([]ThinkingLevel, 0, len(allThinkingLevels))
	for _, level := range allThinkingLevels {
		if thinkingLevelSupported(model, level) {
			levels = append(levels, level)
		}
	}
	return levels
}

// ClampThinkingLevel returns the nearest supported level, as Pi clamps a
// requested level before sending it.
func ClampThinkingLevel(model Model, level ThinkingLevel) ThinkingLevel {
	available := SupportedThinkingLevels(model)
	if slices.Contains(available, level) {
		return level
	}
	requested := slices.Index(allThinkingLevels, level)
	if requested == -1 {
		if len(available) == 0 {
			return ThinkingOff
		}
		return available[0]
	}
	for i := requested; i < len(allThinkingLevels); i++ {
		if slices.Contains(available, allThinkingLevels[i]) {
			return allThinkingLevels[i]
		}
	}
	for i := requested - 1; i >= 0; i-- {
		if slices.Contains(available, allThinkingLevels[i]) {
			return allThinkingLevels[i]
		}
	}
	if len(available) == 0 {
		return ThinkingOff
	}
	return available[0]
}

// ThinkingLevelEffort is the provider value for a level: the model's mapping
// when present, otherwise the level itself.
func ThinkingLevelEffort(model Model, level ThinkingLevel) string {
	if mapped, ok := model.ThinkingLevelMap[level]; ok && mapped != nil {
		return *mapped
	}
	return string(level)
}

// ThinkingBudgetTokens is the token budget for a level. xhigh and max use the
// high budget; off and unknown levels have none.
func ThinkingBudgetTokens(level ThinkingLevel) int {
	switch level {
	case ThinkingMinimal:
		return defaultThinkingMinimal
	case ThinkingLow:
		return defaultThinkingLow
	case ThinkingMedium:
		return defaultThinkingMedium
	case ThinkingHigh, ThinkingXHigh, ThinkingMax:
		return defaultThinkingHigh
	}
	return 0
}

// clampBudgetToAnswerRoom caps a thinking budget so at least minAnswerTokens
// remain under a shared response ceiling.
func clampBudgetToAnswerRoom(budget, ceiling int) int {
	return min(budget, max(0, ceiling-minAnswerTokens))
}

// resolveChatTemplateValues fills a chat-template kwargs/args object: literals
// pass through, a "$var" placeholder reads thinking.enabled, thinking.effort or
// thinking.budget, and omitWhenOff drops the key while reasoning is off.
func resolveChatTemplateValues(config map[string]any, reasoningOn bool, effort func() (string, bool), budget int) map[string]any {
	if len(config) == 0 {
		return nil
	}
	resolved := make(map[string]any, len(config))
	for key, value := range config {
		variable, ok := value.(map[string]any)
		if _, isArray := value.([]any); isArray {
			variable, ok = nil, true
		}
		if !ok {
			resolved[key] = value
			continue
		}
		if omit, _ := variable["omitWhenOff"].(bool); omit && !reasoningOn {
			continue
		}
		switch variable["$var"] {
		case "thinking.enabled":
			resolved[key] = reasoningOn
		case "thinking.budget":
			if reasoningOn && budget > 0 {
				resolved[key] = budget
			}
		default:
			if effortValue, emit := effort(); emit {
				resolved[key] = effortValue
			}
		}
	}
	return resolved
}

// SamplingParamsFor merges the model's sampling defaults, its per-level
// overrides and the request's own params, in that order, as Pi does. It returns
// nil when none applies.
func SamplingParamsFor(model Model, level ThinkingLevel, request map[string]any) map[string]any {
	defaults := model.SamplingParams
	levelParams := model.SamplingParamsByThinkingLevel[ClampThinkingLevel(model, level)]
	if len(defaults) == 0 && len(levelParams) == 0 && len(request) == 0 {
		return nil
	}
	merged := make(map[string]any, len(defaults)+len(levelParams)+len(request))
	maps.Copy(merged, defaults)
	maps.Copy(merged, levelParams)
	maps.Copy(merged, request)
	return merged
}

// applyCompletionsThinking writes the thinking/reasoning fields of an
// openai-completions request, following Pi's thinkingFormat dispatch.
func applyCompletionsThinking(body map[string]any, model Model, opts StreamOptions) {
	if opts.ThinkingLevel == "" || !model.Reasoning {
		return
	}
	level := ClampThinkingLevel(model, opts.ThinkingLevel)
	reasoningOn := level != ThinkingOff
	format := thinkingFormatFor(model)
	supportsRE := supportsReasoningEffort(model)
	mapped := func(l ThinkingLevel) string { return ThinkingLevelEffort(model, l) }
	offEffort := func() (string, bool) {
		if value, ok := model.ThinkingLevelMap[ThinkingOff]; ok {
			if value == nil {
				return "", false
			}
			return *value, true
		}
		return "none", true
	}
	templateEffort := func() (string, bool) {
		l := level
		if !reasoningOn {
			l = ThinkingOff
		}
		value, present := model.ThinkingLevelMap[l]
		switch {
		case !present && reasoningOn:
			return string(level), true
		case value == nil:
			return "", false
		}
		return *value, true
	}
	ceiling := model.MaxOutputTokens(opts)
	budget := 0
	if reasoningOn {
		budget = ThinkingBudgetTokens(level)
		if ceiling > 0 {
			budget = clampBudgetToAnswerRoom(budget, ceiling)
		}
	}

	switch format {
	case "zai":
		if reasoningOn {
			body["thinking"] = map[string]any{"type": "enabled", "clear_thinking": false}
			if supportsRE {
				if value, exists := model.ThinkingLevelMap[level]; !exists || value != nil {
					body["reasoning_effort"] = mapped(level)
				}
			}
		} else {
			body["thinking"] = map[string]any{"type": "disabled"}
		}
	case "qwen":
		body["enable_thinking"] = reasoningOn
		if reasoningOn && supportsRE {
			body["reasoning_effort"] = mapped(level)
		}
	case "qwen-chat-template":
		body["chat_template_kwargs"] = map[string]any{"enable_thinking": reasoningOn, "preserve_thinking": true}
	case "chat-template":
		if values := resolveChatTemplateValues(model.Compat.ChatTemplateKwargs, reasoningOn, templateEffort, budget); values != nil {
			body["chat_template_kwargs"] = values
		}
	case "baseten":
		if values := resolveChatTemplateValues(model.Compat.ChatTemplateArgs, reasoningOn, templateEffort, budget); values != nil {
			body["chat_template_args"] = values
		}
		if value, emit := templateEffort(); supportsRE && emit {
			body["reasoning_effort"] = value
		}
	case "deepseek":
		if reasoningOn {
			body["thinking"] = map[string]any{"type": "enabled"}
			if supportsRE {
				body["reasoning_effort"] = mapped(level)
			}
		} else if _, emit := offEffort(); emit {
			body["thinking"] = map[string]any{"type": "disabled"}
		}
	case "openrouter":
		if reasoningOn {
			body["reasoning"] = map[string]any{"effort": mapped(level)}
		} else if value, emit := offEffort(); emit {
			body["reasoning"] = map[string]any{"effort": value}
		}
	case "ant-ling":
		if reasoningOn {
			if value, ok := model.ThinkingLevelMap[level]; ok && value != nil {
				body["reasoning"] = map[string]any{"effort": *value}
			}
		} else if value, ok := model.ThinkingLevelMap[ThinkingOff]; ok && value != nil && supportsRE {
			body["reasoning_effort"] = *value
		}
	case "together":
		body["reasoning"] = map[string]any{"enabled": reasoningOn}
		if reasoningOn && supportsRE {
			body["reasoning_effort"] = mapped(level)
		}
	case "string-thinking":
		if reasoningOn {
			body["thinking"] = mapped(level)
		} else if value, emit := offEffort(); emit {
			body["thinking"] = value
		}
	default: // "openai"
		if reasoningOn && supportsRE {
			body["reasoning_effort"] = mapped(level)
		} else if !reasoningOn && supportsRE {
			if value, ok := model.ThinkingLevelMap[ThinkingOff]; ok && value != nil {
				body["reasoning_effort"] = *value
			}
		}
	}

	budgetField := model.Compat.ThinkingTokenBudgetField
	if budgetField == "" && model.Compat.flag(model.Compat.SupportsThinkingTokenBudget, false) {
		budgetField = "thinking_token_budget"
	}
	if reasoningOn && budgetField != "" && budget > 0 {
		body[budgetField] = budget
	}
}

// thinkingFormatFor returns the configured format, or the one Pi detects from
// the provider name and base URL for its known OpenAI-compatible gateways.
func thinkingFormatFor(model Model) string {
	if model.Compat.ThinkingFormat != "" {
		return model.Compat.ThinkingFormat
	}
	provider := strings.ToLower(model.Provider)
	base := strings.ToLower(model.BaseURL)
	has := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(base, part) {
				return true
			}
		}
		return false
	}
	switch {
	case provider == "deepseek" || has("deepseek.com"):
		return "deepseek"
	case provider == "zai" || provider == "zai-coding-cn" || has("api.z.ai", "open.bigmodel.cn"):
		return "zai"
	case provider == "together" || has("api.together.ai", "api.together.xyz"):
		return "together"
	case provider == "ant-ling" || has("api.ant-ling.com"):
		return "ant-ling"
	case provider == "openrouter" || has("openrouter.ai"):
		return "openrouter"
	}
	return "openai"
}

// supportsReasoningEffort is the compat flag, or Pi's detection: the gateways
// that reject reasoning_effort do not get it.
func supportsReasoningEffort(model Model) bool {
	if model.Compat.SupportsReasoningEffort != nil {
		return *model.Compat.SupportsReasoningEffort
	}
	if model.Compat.ThinkingFormat != "" {
		return true
	}
	switch strings.ToLower(model.Provider) {
	case "xai", "zai", "zai-coding-cn", "moonshotai", "moonshotai-cn", "together",
		"cloudflare-ai-gateway", "nvidia", "ant-ling":
		return false
	}
	base := strings.ToLower(model.BaseURL)
	for _, host := range []string{"api.x.ai", "api.z.ai", "open.bigmodel.cn", "api.together.", "chutes.ai", "integrate.api.nvidia.com", "api.ant-ling.com", "gateway.ai.cloudflare.com", "api.moonshot."} {
		if strings.Contains(base, host) {
			return false
		}
	}
	return true
}

// applyResponsesThinking writes the reasoning member of an openai-responses
// request, as Pi's buildParams does.
func applyResponsesThinking(body map[string]any, model Model, opts StreamOptions) {
	if opts.ThinkingLevel == "" || !model.Reasoning {
		return
	}
	level := ClampThinkingLevel(model, opts.ThinkingLevel)
	switch {
	case level != ThinkingOff:
		body["reasoning"] = map[string]any{
			"effort":  ThinkingLevelEffort(model, level),
			"summary": "auto",
		}
		body["include"] = []string{"reasoning.encrypted_content"}
	default:
		value, ok := model.ThinkingLevelMap[ThinkingOff]
		if ok && value == nil {
			return
		}
		effort := "none"
		if value != nil {
			effort = *value
		}
		body["reasoning"] = map[string]any{"effort": effort}
	}
}

// applyAnthropicThinking writes the thinking and output_config members of an
// Anthropic request. A budget-based level also raises max_tokens to make room
// for the thinking, as Pi's adjustMaxTokensForThinking does.
func applyAnthropicThinking(body map[string]any, model Model, opts StreamOptions) {
	if opts.ThinkingLevel == "" {
		return
	}
	level := ClampThinkingLevel(model, opts.ThinkingLevel)
	if level == ThinkingOff {
		if model.Reasoning {
			if value, ok := model.ThinkingLevelMap[ThinkingOff]; !ok || value != nil {
				body["thinking"] = map[string]any{"type": "disabled"}
			}
		}
		return
	}
	if !model.Reasoning {
		return
	}
	if model.Compat.AdaptiveThinking() {
		body["thinking"] = map[string]any{"type": "adaptive", "display": "summarized"}
		body["output_config"] = map[string]any{"effort": anthropicEffort(model, level)}
		return
	}
	maxTokens, _ := body["max_tokens"].(int)
	budget := ThinkingBudgetTokens(level)
	ceiling := model.MaxTokens
	if ceiling == 0 {
		ceiling = maxTokens + budget
	}
	adjusted := min(maxTokens+budget, ceiling)
	if adjusted <= budget {
		budget = clampBudgetToAnswerRoom(budget, adjusted)
	}
	if budget == 0 {
		budget = 1024
	}
	body["max_tokens"] = adjusted
	body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget, "display": "summarized"}
}

// anthropicEffort maps a level to an Anthropic output_config effort.
func anthropicEffort(model Model, level ThinkingLevel) string {
	if value, ok := model.ThinkingLevelMap[level]; ok && value != nil {
		return *value
	}
	switch level {
	case ThinkingMinimal, ThinkingLow:
		return "low"
	case ThinkingMedium:
		return "medium"
	default:
		return "high"
	}
}

// applySamplingParams merges the model's sampling defaults and per-level
// overrides into the request body; the request's own params win.
func applySamplingParams(body map[string]any, model Model, opts StreamOptions) {
	for key, value := range SamplingParamsFor(model, opts.ThinkingLevel, opts.SamplingParams) {
		body[key] = value
	}
}
