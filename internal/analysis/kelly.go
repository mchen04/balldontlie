package analysis

import "math"

// CalculateKelly computes the Kelly criterion bet size
// Kelly formula: f* = (p * b - q) / b
// where: p = probability of winning, q = 1-p, b = odds received (profit/stake)
//
// For Kalshi: stake = price, profit = 1 - price
// So b = (1 - price) / price
//
// fraction parameter scales the result (e.g., 0.25 for quarter Kelly)
func CalculateKelly(trueProb, kalshiPrice, fraction float64) float64 {
	if kalshiPrice <= 0 || kalshiPrice >= 1 || trueProb <= 0 || trueProb >= 1 {
		return 0
	}

	p := trueProb
	q := 1.0 - p
	b := (1.0 - kalshiPrice) / kalshiPrice // decimal odds minus 1

	kelly := (p*b - q) / b

	// Cap at reasonable maximum and floor at 0
	kelly = math.Max(0, kelly)
	kelly = math.Min(kelly, 1.0) // Never bet more than 100% of bankroll

	return kelly * fraction
}

// CalculateKellyDecimal computes Kelly for decimal odds
// f* = (p * d - 1) / (d - 1)
// where d = decimal odds
func CalculateKellyDecimal(trueProb, decimalOdds, fraction float64) float64 {
	if decimalOdds <= 1 || trueProb <= 0 || trueProb >= 1 {
		return 0
	}

	p := trueProb
	d := decimalOdds

	kelly := (p*d - 1) / (d - 1)

	kelly = math.Max(0, kelly)
	kelly = math.Min(kelly, 1.0)

	return kelly * fraction
}

// OptimalBetSize returns the dollar amount to bet given bankroll
func OptimalBetSize(bankroll, kellyFraction float64) float64 {
	return bankroll * kellyFraction
}

// KellyWithEdge calculates Kelly when you know the edge directly
// edge = (trueProb - impliedProb) / impliedProb = EV / stake
func KellyWithEdge(edge, impliedProb, fraction float64) float64 {
	if impliedProb <= 0 || impliedProb >= 1 {
		return 0
	}

	// Convert edge to Kelly
	// Kelly = edge / (1/impliedProb - 1)
	b := (1.0 - impliedProb) / impliedProb
	kelly := edge / b

	kelly = math.Max(0, kelly)
	kelly = math.Min(kelly, 1.0)

	return kelly * fraction
}

// CalculateKellyBetSize calculates the actual dollar amount to bet
// given a real bankroll (in dollars)
// Returns the bet size in dollars, capped at maxBet if provided (0 = no cap)
func CalculateKellyBetSize(trueProb, kalshiPrice, fraction, bankroll, maxBet float64) float64 {
	kellyPct := CalculateKelly(trueProb, kalshiPrice, fraction)
	if kellyPct <= 0 {
		return 0
	}

	betSize := bankroll * kellyPct

	// Apply max bet cap if specified
	if maxBet > 0 && betSize > maxBet {
		betSize = maxBet
	}

	return betSize
}

// CalculateKellyContracts converts Kelly bet size to number of contracts
// priceInCents is the Kalshi price (1-99 cents per contract)
// Returns the number of contracts to buy
func CalculateKellyContracts(trueProb, kalshiPrice, fraction, bankrollDollars, maxBetDollars float64, priceInCents int) int {
	betSize := CalculateKellyBetSize(trueProb, kalshiPrice, fraction, bankrollDollars, maxBetDollars)
	if betSize <= 0 || priceInCents <= 0 {
		return 0
	}

	// Convert bet size to cents, then divide by price per contract
	betSizeCents := betSize * 100
	contracts := int(betSizeCents / float64(priceInCents))

	return contracts
}

// AdjustKellyForSlippage recalculates Kelly using the actual fill price
// instead of the best available price, to account for market impact
func AdjustKellyForSlippage(trueProb, bestPrice, actualFillPrice, fraction float64) float64 {
	// Use the actual fill price instead of best price
	return CalculateKelly(trueProb, actualFillPrice, fraction)
}

// RecalculateEVWithSlippage computes new EV given actual fill price
// Returns (rawEV, adjustedEV) accounting for slippage
func RecalculateEVWithSlippage(trueProb, actualFillPrice, feePct float64) (float64, float64) {
	if actualFillPrice <= 0 || actualFillPrice >= 1 {
		return 0, 0
	}

	profit := 1.0 - actualFillPrice
	stake := actualFillPrice

	rawEV := (trueProb * profit) - ((1 - trueProb) * stake)
	fee := actualFillPrice * feePct
	adjustedEV := rawEV - fee

	return rawEV, adjustedEV
}
