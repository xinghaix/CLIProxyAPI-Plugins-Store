package responsemodel

import "strings"

// recognizedServiceTiers is the only OpenAI service_tier allow-list. Pricing
// must classify every entry as either a priced band or an explicit unpriced tier.
var recognizedServiceTiers = []string{"default", "standard", "priority", "fast", "flex"}

func RecognizedServiceTiers() []string {
	out := make([]string, len(recognizedServiceTiers))
	copy(out, recognizedServiceTiers)
	return out
}

func actualServiceTier(doc map[string]any, format string) string {
	if format != "openai" && format != "openai-response" {
		return ""
	}
	if format == "openai-response" {
		if response := object(doc, "response"); response != nil {
			doc = response
		}
	}
	tier := strings.ToLower(strings.TrimSpace(text(doc, "service_tier")))
	for _, candidate := range recognizedServiceTiers {
		if tier == candidate {
			return tier
		}
	}
	return ""
}

// mergeText keeps the first nonblank value. A second different nonblank value
// quarantines the field. Blank input never clears or conflicts.
func mergeText(current, next string) (value string, conflict bool) {
	if next == "" || current == "" || current == next {
		if current == "" {
			return next, false
		}
		return current, false
	}
	return "", true
}

func mergeActualServiceTier(e *entry, tier string) {
	if e == nil || e.serviceTierAmbiguous {
		return
	}
	merged, conflict := mergeText(e.serviceTier, tier)
	e.serviceTier = merged
	e.serviceTierAmbiguous = conflict
}
