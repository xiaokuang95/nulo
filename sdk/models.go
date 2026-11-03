package sdk

import "github.com/xiaokuang95/nulo/sdk/internal/ai"

func DefaultFauxModel() Model { return ai.DefaultFauxModel() }

// SupportedThinkingLevels lists the levels the model accepts, in order.
func SupportedThinkingLevels(model Model) []ThinkingLevel {
	return ai.SupportedThinkingLevels(model)
}

// ClampThinkingLevel returns the nearest level the model supports.
func ClampThinkingLevel(model Model, level ThinkingLevel) ThinkingLevel {
	return ai.ClampThinkingLevel(model, level)
}

// EstimateCost prices one usage against the model, pricing tiers included.
func EstimateCost(model Model, usage Usage) Cost {
	return ai.EstimateCost(model, usage)
}
