package kalshi

import "math"

// TakerFee calculates the Kalshi taker fee for a given price (0-1).
// Formula: 0.07 * price * (1 - price), capped at $0.0175 per contract.
// Returns fee as a fraction of $1 (e.g., 0.0175 = 1.75 cents).
func TakerFee(price float64) float64 {
	if price <= 0 || price >= 1 {
		return 0
	}
	fee := 0.07 * price * (1 - price)
	// Cap at 1.75 cents per contract
	return math.Min(fee, 0.0175)
}

// TakerFeeCents calculates the Kalshi taker fee in cents for a price in cents.
// Convenience function for code that works in cents (e.g., arb calculations).
func TakerFeeCents(priceCents int) float64 {
	price := float64(priceCents) / 100.0
	return TakerFee(price) * 100.0
}
