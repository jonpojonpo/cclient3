package display

import "strings"

// pricingEntry holds per-million-token prices (USD) for a model.
type pricingEntry struct {
	inputPerMTok  float64 // non-cached input
	outputPerMTok float64
}

// modelPricing is a best-effort table of Claude model prices (USD / 1M tokens).
// Cache reads are billed at 10% of input rate; cache writes at 125%.
// Local/Ollama models have zero cost and are not listed here.
var modelPricing = map[string]pricingEntry{
	"claude-opus-4-6":            {15.0, 75.0},
	"claude-opus-4-5":            {15.0, 75.0},
	"claude-sonnet-4-6":          {3.0, 15.0},
	"claude-sonnet-4-5":          {3.0, 15.0},
	"claude-haiku-4-5":           {0.80, 4.0},
	"claude-haiku-4-5-20251001":  {0.80, 4.0},
	"claude-3-5-sonnet-20241022": {3.0, 15.0},
	"claude-3-5-haiku-20241022":  {0.80, 4.0},
	"claude-3-opus-20240229":     {15.0, 75.0},
	"claude-3-sonnet-20240229":   {3.0, 15.0},
	"claude-3-haiku-20240307":    {0.25, 1.25},
}

// findPricing looks up pricing by exact match, then by prefix (strips date suffix).
func findPricing(model string) (pricingEntry, bool) {
	if p, ok := modelPricing[model]; ok {
		return p, true
	}
	// Strip common 8-digit date suffix: "claude-sonnet-4-6-20250620" → "claude-sonnet-4-6"
	parts := strings.Split(model, "-")
	if len(parts) > 1 {
		last := parts[len(parts)-1]
		if len(last) == 8 {
			prefix := strings.Join(parts[:len(parts)-1], "-")
			if p, ok := modelPricing[prefix]; ok {
				return p, true
			}
		}
	}
	// Prefix match (e.g. "claude-sonnet" matches "claude-sonnet-4-6")
	for k, p := range modelPricing {
		if strings.HasPrefix(model, k) || strings.HasPrefix(k, model) {
			return p, true
		}
	}
	return pricingEntry{}, false
}

// ComputeCost returns the USD cost for a single API call's token usage.
// Returns 0 for unknown/local models.
func ComputeCost(model string, inputTok, outputTok, cacheRead, cacheCreate int) float64 {
	p, ok := findPricing(model)
	if !ok {
		return 0
	}
	// Non-cached input = total input - cache hits - cache writes
	nonCached := inputTok - cacheRead - cacheCreate
	if nonCached < 0 {
		nonCached = 0
	}
	return float64(nonCached)*p.inputPerMTok/1_000_000 +
		float64(cacheRead)*p.inputPerMTok*0.10/1_000_000 +
		float64(cacheCreate)*p.inputPerMTok*1.25/1_000_000 +
		float64(outputTok)*p.outputPerMTok/1_000_000
}
