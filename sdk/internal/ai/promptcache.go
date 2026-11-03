package ai

import "time"

// PromptCacheTTL is the model's best-effort prompt-cache lifetime for the
// retention tier ("short" or "long"), as models.json's promptCache declares it.
func PromptCacheTTL(model Model, retention string) (time.Duration, bool) {
	seconds, ok := model.PromptCache[retention]
	if !ok || seconds <= 0 {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

// CacheWarmingDelay is when to refresh a cache that expires after ttl: 90% of
// the lifetime, but with at least ten seconds of margin. Lifetimes too short to
// refresh safely are unusable.
func CacheWarmingDelay(ttl time.Duration) (time.Duration, bool) {
	if ttl <= 10*time.Second {
		return 0, false
	}
	delay := time.Duration(float64(ttl) * 0.9)
	if margin := ttl - 10*time.Second; delay > margin {
		delay = margin
	}
	if delay < time.Second {
		delay = time.Second
	}
	return delay, true
}

// EstimateCost prices one usage against the model, tiers included.
func EstimateCost(model Model, usage Usage) Cost {
	priced := usage
	calculateCost(model, &priced)
	return priced.Cost
}
