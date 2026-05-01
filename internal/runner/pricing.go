package runner

import (
	"log/slog"
	"strings"
)

// modelPricing holds per-million-token USD prices for a Claude model.
type modelPricing struct {
	Input         float64
	Output        float64
	CacheRead     float64
	CacheCreation float64
}

const tokensPerMillion = 1_000_000.0

// modelPrices maps Claude model identifiers to their per-million-token USD pricing.
// Keys can be either canonical IDs (e.g. "claude-sonnet-4-5") or short aliases
// ("sonnet"). Lookup tries an exact match first, then falls back to prefix
// matching on the canonical family ("claude-opus-*", "claude-sonnet-*",
// "claude-haiku-*").
//
// Prices below are list rates published by Anthropic for the Claude 4.x family.
// Update the table when new models or pricing tiers are introduced.
var modelPrices = map[string]modelPricing{
	"opus":   {Input: 15.0, Output: 75.0, CacheRead: 1.50, CacheCreation: 18.75},
	"sonnet": {Input: 3.0, Output: 15.0, CacheRead: 0.30, CacheCreation: 3.75},
	"haiku":  {Input: 1.0, Output: 5.0, CacheRead: 0.10, CacheCreation: 1.25},
}

// lookupPricing returns the pricing for the given model name, plus true if
// found. It accepts both canonical model IDs (e.g. "claude-sonnet-4-5-20250929")
// and aliases ("sonnet", "opus", "haiku") and matches case-insensitively.
func lookupPricing(model string) (modelPricing, bool) {
	if model == "" {
		return modelPricing{}, false
	}
	key := strings.ToLower(model)
	if p, ok := modelPrices[key]; ok {
		return p, true
	}
	switch {
	case strings.Contains(key, "opus"):
		return modelPrices["opus"], true
	case strings.Contains(key, "sonnet"):
		return modelPrices["sonnet"], true
	case strings.Contains(key, "haiku"):
		return modelPrices["haiku"], true
	}
	return modelPricing{}, false
}

// computeCost returns the USD cost for the given token counts under the named
// model's pricing. If the model is unknown, it logs a warning and returns 0.
func computeCost(model string, inputTokens, outputTokens, cacheReadTokens, cacheCreationTokens int64) float64 {
	p, ok := lookupPricing(model)
	if !ok {
		slog.Warn("unknown model for cost computation, defaulting to 0", "model", model)
		return 0
	}
	cost := float64(inputTokens)*p.Input/tokensPerMillion +
		float64(outputTokens)*p.Output/tokensPerMillion +
		float64(cacheReadTokens)*p.CacheRead/tokensPerMillion +
		float64(cacheCreationTokens)*p.CacheCreation/tokensPerMillion
	return cost
}
