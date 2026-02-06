package kalshi

import "math"

// Fee parameters — configurable to handle Kalshi fee schedule changes.
var (
	feeCoeff = 0.07
	feeCap   = 0.0175
)

// ConfigureFees sets the taker fee coefficient and cap.
// Call this at startup with values from config.
func ConfigureFees(coeff, cap float64) {
	if coeff > 0 {
		feeCoeff = coeff
	}
	if cap > 0 {
		feeCap = cap
	}
}

// TakerFee calculates the Kalshi taker fee for a given price (0-1).
// Formula: coeff * price * (1 - price), capped at feeCap per contract.
// Returns fee as a fraction of $1 (e.g., 0.0175 = 1.75 cents).
func TakerFee(price float64) float64 {
	if price <= 0 || price >= 1 {
		return 0
	}
	fee := feeCoeff * price * (1 - price)
	return math.Min(fee, feeCap)
}

// TakerFeeCents calculates the Kalshi taker fee in cents for a price in cents.
// Convenience function for code that works in cents (e.g., arb calculations).
func TakerFeeCents(priceCents int) float64 {
	price := float64(priceCents) / 100.0
	return TakerFee(price) * 100.0
}
