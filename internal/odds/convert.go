package odds

import "math"

// OddsToImplied converts odds to implied probability, auto-detecting format
// Handles:
// - American odds: -150, +130, etc. (|value| >= 100)
// - Kalshi prices as cents: 1-99 (interpreted as 0.01-0.99)
// - Kalshi prices as decimal: 0.01-0.99 (used directly)
func OddsToImplied(odds int) float64 {
	if odds == 0 {
		return 0
	}

	// Detect format based on value range
	absOdds := odds
	if absOdds < 0 {
		absOdds = -absOdds
	}

	// American odds are typically >= 100 in absolute value
	if absOdds >= 100 {
		return AmericanToImplied(odds)
	}

	// Values 1-99 are likely Kalshi prices in cents
	// Convert to probability (e.g., 45 -> 0.45)
	if odds >= 1 && odds <= 99 {
		return float64(odds) / 100.0
	}

	// Fallback to American conversion for edge cases
	return AmericanToImplied(odds)
}

// AmericanToImplied converts American odds to implied probability
// Example: -150 → 0.6 (60%), +150 → 0.4 (40%)
func AmericanToImplied(odds int) float64 {
	if odds == 0 {
		return 0
	}

	if odds > 0 {
		// Underdog: probability = 100 / (odds + 100)
		return 100.0 / (float64(odds) + 100.0)
	}
	// Favorite: probability = |odds| / (|odds| + 100)
	return math.Abs(float64(odds)) / (math.Abs(float64(odds)) + 100.0)
}

