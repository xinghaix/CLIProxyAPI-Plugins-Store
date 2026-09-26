package store

import "math"

// WindowUsageMetrics is the forecast input/output shape shared with the Vue port.
type WindowUsageMetrics struct {
	Requests int64
	Tokens   int64
	Cost     float64
}

// WindowUsageForecast is the projected window usage.
type WindowUsageForecast struct {
	Requests int64
	Tokens   int64
	Cost     float64
	Basis    string // "quota" | "previous"
}

// EstimateWindowUsage ports CPA-Manager-Plus estimateWindowUsage.ts
// (MIT, Seakee): current * 100/used% with previous fallback; cost to cents.
func EstimateWindowUsage(usedPercent float64, current, previous WindowUsageMetrics) *WindowUsageForecast {
	hasCurrent := usableMetrics(current) && hasUsage(current)
	hasQuota := !math.IsNaN(usedPercent) && !math.IsInf(usedPercent, 0) && usedPercent > 0 && usedPercent <= 100
	if hasCurrent && hasQuota {
		multiplier := 100 / usedPercent
		forecast := &WindowUsageForecast{
			Requests: maxInt64(current.Requests, int64(math.Round(float64(current.Requests)*multiplier))),
			Tokens:   maxInt64(current.Tokens, int64(math.Round(float64(current.Tokens)*multiplier))),
			Cost:     math.Max(current.Cost, roundForecastCost(current.Cost*multiplier)),
			Basis:    "quota",
		}
		if math.IsInf(multiplier, 0) || math.IsNaN(multiplier) || !finiteMetrics(*forecast) {
			// fall through
		} else {
			return forecast
		}
	}
	if usableMetrics(previous) {
		return &WindowUsageForecast{Requests: previous.Requests, Tokens: previous.Tokens, Cost: previous.Cost, Basis: "previous"}
	}
	return nil
}

func usableMetrics(m WindowUsageMetrics) bool {
	return m.Requests >= 0 && m.Tokens >= 0 && !math.IsNaN(m.Cost) && !math.IsInf(m.Cost, 0) && m.Cost >= 0
}

func hasUsage(m WindowUsageMetrics) bool {
	return m.Requests > 0 || m.Tokens > 0 || m.Cost > 0
}

func finiteMetrics(m WindowUsageForecast) bool {
	return !math.IsNaN(m.Cost) && !math.IsInf(m.Cost, 0)
}

func roundForecastCost(value float64) float64 {
	scaled := value * 100
	if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
		return value
	}
	return math.Round(scaled) / 100
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
