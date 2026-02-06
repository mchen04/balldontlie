package analysis

import (
	"math"

	"sports-betting-bot/internal/kalshi"
)

// CalculateKelly computes the Kelly criterion bet size with fee adjustment.
// Kelly formula: f* = (p * bNet - q) / bNet
// where: p = probability of winning, q = 1-p, bNet = fee-adjusted odds
//
// For Kalshi: effectiveStake = price + fee, effectiveProfit = 1 - price - fee
// So bNet = effectiveProfit / effectiveStake
//
// fraction parameter scales the result (e.g., 0.25 for quarter Kelly)
func CalculateKelly(trueProb, kalshiPrice, fraction float64) float64 {
	if kalshiPrice <= 0 || kalshiPrice >= 1 || trueProb <= 0 || trueProb >= 1 {
		return 0
	}

	p := trueProb
	q := 1.0 - p
	fee := kalshi.TakerFee(kalshiPrice)
	effectiveStake := kalshiPrice + fee
	effectiveProfit := 1.0 - kalshiPrice - fee
	if effectiveProfit <= 0 {
		return 0
	}
	bNet := effectiveProfit / effectiveStake

	kelly := (p*bNet - q) / bNet

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

// KellyWithEdge calculates Kelly when you know the edge directly.
// Uses impliedProb as price proxy for fee adjustment.
// edge = (trueProb - impliedProb) / impliedProb = EV / stake
func KellyWithEdge(edge, impliedProb, fraction float64) float64 {
	if impliedProb <= 0 || impliedProb >= 1 {
		return 0
	}

	// Fee-adjust the odds using impliedProb as price proxy
	fee := kalshi.TakerFee(impliedProb)
	effectiveStake := impliedProb + fee
	effectiveProfit := 1.0 - impliedProb - fee
	if effectiveProfit <= 0 {
		return 0
	}
	bNet := effectiveProfit / effectiveStake

	// Derive trueProb from edge: edge = (trueProb - impliedProb) / impliedProb
	trueProb := impliedProb * (1 + edge)
	q := 1.0 - trueProb
	kelly := (trueProb*bNet - q) / bNet

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

// RecalculateEVWithSlippage computes new EV given actual fill price
// Returns (rawEV, adjustedEV) accounting for slippage
func RecalculateEVWithSlippage(trueProb, actualFillPrice float64) (float64, float64) {
	if actualFillPrice <= 0 || actualFillPrice >= 1 {
		return 0, 0
	}

	profit := 1.0 - actualFillPrice
	stake := actualFillPrice

	rawEV := (trueProb * profit) - ((1 - trueProb) * stake)
	fee := kalshi.TakerFee(actualFillPrice)
	adjustedEV := rawEV - fee

	return rawEV, adjustedEV
}
