package odds

import (
	"math"

	"sports-betting-bot/internal/api"
)

const (
	// NBASpreadStdDev is the standard deviation of NBA margin vs spread
	// Based on historical data: ATS margin follows N(0, ~11.5)
	// Sources: Boyd's Bets, Wayne Winston Mathletics
	NBASpreadStdDev = 11.5

	// NBATotalStdDev is the standard deviation for NBA totals
	// Slightly lower variance than spreads
	NBATotalStdDev = 10.5
)

// MarketType represents the type of betting market
type MarketType string

const (
	MarketMoneyline MarketType = "moneyline"
	MarketSpread    MarketType = "spread"
	MarketTotal     MarketType = "total"
)

// ConsensusOdds holds the calculated true probabilities for a game
type ConsensusOdds struct {
	GameID     int
	GameDate   string // Date in "2006-01-02" format
	HomeTeam   string
	AwayTeam   string
	Moneyline  *MoneylineConsensus
	Spread     *SpreadConsensus
	Total      *TotalConsensus
	KalshiOdds *KalshiOdds
}

// MoneylineConsensus holds consensus probabilities for moneyline
type MoneylineConsensus struct {
	HomeTrueProb float64 // Vig-removed probability
	AwayTrueProb float64
	BookCount    int // Number of books used
}

// SpreadConsensus holds consensus probabilities for spread
type SpreadConsensus struct {
	HomeSpread   float64
	HomeCoverProb float64
	AwayCoverProb float64
	BookCount    int
}

// TotalConsensus holds consensus probabilities for totals
type TotalConsensus struct {
	Line      float64
	OverProb  float64
	UnderProb float64
	BookCount int
}

// KalshiOdds holds Kalshi-specific odds for comparison
type KalshiOdds struct {
	Moneyline *api.Moneyline
	Spread    *api.Spread
	Total     *api.Total
}

// CalculateConsensus computes consensus true probabilities from multiple vendors
// Normalizes spread/total probabilities to match Kalshi's line
func CalculateConsensus(gameOdds api.GameOdds) ConsensusOdds {
	consensus := ConsensusOdds{
		GameID:   gameOdds.GameID,
		GameDate: gameOdds.Game.Date,
		HomeTeam: gameOdds.Game.HomeTeam.Abbreviation,
		AwayTeam: gameOdds.Game.VisitorTeam.Abbreviation,
	}

	// First pass: extract Kalshi odds to get target lines
	for _, vendor := range gameOdds.Vendors {
		if api.IsKalshi(vendor.Name) {
			consensus.KalshiOdds = &KalshiOdds{
				Moneyline: vendor.Moneyline,
				Spread:    vendor.Spread,
				Total:     vendor.Total,
			}
			break
		}
	}

	var mlProbs []struct{ home, away float64 }
	var spreadProbs []struct{ homeCover, awayCover float64 }
	var totalProbs []struct{ over, under float64 }

	// Get Kalshi lines as targets for normalization
	var kalshiSpreadLine, kalshiTotalLine float64
	if consensus.KalshiOdds != nil {
		if consensus.KalshiOdds.Spread != nil {
			kalshiSpreadLine = consensus.KalshiOdds.Spread.HomeSpread
		}
		if consensus.KalshiOdds.Total != nil {
			kalshiTotalLine = consensus.KalshiOdds.Total.Line
		}
	}

	// Second pass: collect and normalize probabilities
	for _, vendor := range gameOdds.Vendors {
		if api.IsKalshi(vendor.Name) {
			continue
		}

		// Moneyline (no normalization needed)
		if vendor.Moneyline != nil && vendor.Moneyline.Home != 0 && vendor.Moneyline.Away != 0 {
			homeProb, awayProb := RemoveVigPowerFromAmerican(vendor.Moneyline.Home, vendor.Moneyline.Away)
			if homeProb > 0 && awayProb > 0 {
				mlProbs = append(mlProbs, struct{ home, away float64 }{homeProb, awayProb})
			}
		}

		// Spread (normalize to Kalshi line)
		if vendor.Spread != nil && vendor.Spread.HomeOdds != 0 && vendor.Spread.AwayOdds != 0 {
			homeCover, awayCover := RemoveVigPowerFromAmerican(vendor.Spread.HomeOdds, vendor.Spread.AwayOdds)
			if homeCover > 0 && awayCover > 0 {
				// Normalize to Kalshi line if available
				if kalshiSpreadLine != 0 {
					homeCover, awayCover = normalizeSpreadProb(
						homeCover, awayCover,
						vendor.Spread.HomeSpread, kalshiSpreadLine,
					)
				}
				spreadProbs = append(spreadProbs, struct{ homeCover, awayCover float64 }{homeCover, awayCover})
			}
		}

		// Total (normalize to Kalshi line)
		if vendor.Total != nil && vendor.Total.OverOdds != 0 && vendor.Total.UnderOdds != 0 {
			overProb, underProb := RemoveVigPowerFromAmerican(vendor.Total.OverOdds, vendor.Total.UnderOdds)
			if overProb > 0 && underProb > 0 {
				// Normalize to Kalshi line if available
				if kalshiTotalLine != 0 {
					overProb, underProb = normalizeTotalProb(
						overProb, underProb,
						vendor.Total.Line, kalshiTotalLine,
					)
				}
				totalProbs = append(totalProbs, struct{ over, under float64 }{overProb, underProb})
			}
		}
	}

	// Calculate averages
	if len(mlProbs) > 0 {
		var homeSum, awaySum float64
		for _, p := range mlProbs {
			homeSum += p.home
			awaySum += p.away
		}
		consensus.Moneyline = &MoneylineConsensus{
			HomeTrueProb: homeSum / float64(len(mlProbs)),
			AwayTrueProb: awaySum / float64(len(mlProbs)),
			BookCount:    len(mlProbs),
		}
	}

	if len(spreadProbs) > 0 {
		var homeCoverSum, awayCoverSum float64
		for _, p := range spreadProbs {
			homeCoverSum += p.homeCover
			awayCoverSum += p.awayCover
		}
		n := float64(len(spreadProbs))
		consensus.Spread = &SpreadConsensus{
			HomeSpread:    kalshiSpreadLine, // Use Kalshi line as the reference
			HomeCoverProb: homeCoverSum / n,
			AwayCoverProb: awayCoverSum / n,
			BookCount:     len(spreadProbs),
		}
	}

	if len(totalProbs) > 0 {
		var overSum, underSum float64
		for _, p := range totalProbs {
			overSum += p.over
			underSum += p.under
		}
		n := float64(len(totalProbs))
		consensus.Total = &TotalConsensus{
			Line:      kalshiTotalLine, // Use Kalshi line as the reference
			OverProb:  overSum / n,
			UnderProb: underSum / n,
			BookCount: len(totalProbs),
		}
	}

	return consensus
}

// normalizeSpreadProb adjusts spread probabilities from bookLine to targetLine
// Uses normal distribution model: ATS margin ~ N(0, σ) where σ ≈ 11.5 for NBA
//
// Example: Book has home -6.0 at 50% cover, target is -5.5
// -5.5 is easier to cover (smaller number to beat), so probability increases
//
// For negative spreads: larger absolute value = harder to cover
// Moving from -6.0 to -5.5 = easier = higher cover probability
func normalizeSpreadProb(homeCover, awayCover, bookLine, targetLine float64) (float64, float64) {
	if bookLine == targetLine {
		return homeCover, awayCover
	}

	// Convert book's cover probability to a z-score
	bookZ := normalInvCDF(homeCover)

	// Line difference: positive when target is easier for home to cover
	// For negative spreads: -5.5 > -6.0, so targetLine - bookLine > 0 when easier
	// For positive spreads: +5.5 < +6.0, so targetLine - bookLine < 0 when easier
	// But both cases: higher number = easier for home, so use (targetLine - bookLine)
	lineDiff := targetLine - bookLine

	// Adjust z-score: easier target = higher probability = higher z
	targetZ := bookZ + (lineDiff / NBASpreadStdDev)

	// Convert back to probability
	adjustedHome := normalCDF(targetZ)
	adjustedAway := 1.0 - adjustedHome

	// Clamp to valid range
	adjustedHome = math.Max(0.01, math.Min(0.99, adjustedHome))
	adjustedAway = math.Max(0.01, math.Min(0.99, adjustedAway))

	return adjustedHome, adjustedAway
}

// normalizeTotalProb adjusts total probabilities from bookLine to targetLine
// Uses normal distribution model similar to spreads
//
// Example: Book has O220.5 at 50%, target is O219.5 (lower line = easier to go over)
func normalizeTotalProb(overProb, underProb, bookLine, targetLine float64) (float64, float64) {
	if bookLine == targetLine {
		return overProb, underProb
	}

	// Convert book's over probability to z-score
	bookZ := normalInvCDF(overProb)

	// For totals: lower target = easier to go over
	// lineDiff > 0 means target is higher (harder to go over)
	lineDiff := targetLine - bookLine
	targetZ := bookZ - (lineDiff / NBATotalStdDev)

	// Convert back to probability
	adjustedOver := normalCDF(targetZ)
	adjustedUnder := 1.0 - adjustedOver

	// Clamp to valid range
	adjustedOver = math.Max(0.01, math.Min(0.99, adjustedOver))
	adjustedUnder = math.Max(0.01, math.Min(0.99, adjustedUnder))

	return adjustedOver, adjustedUnder
}

// normalCDF calculates the cumulative distribution function of the standard normal distribution
// P(Z <= z) where Z ~ N(0,1)
func normalCDF(z float64) float64 {
	return 0.5 * (1 + math.Erf(z/math.Sqrt2))
}

// normalInvCDF calculates the inverse CDF (quantile function) of the standard normal distribution
// Returns z such that P(Z <= z) = p
// Uses Abramowitz and Stegun approximation
func normalInvCDF(p float64) float64 {
	if p <= 0 {
		return -10 // Clamp to reasonable minimum
	}
	if p >= 1 {
		return 10 // Clamp to reasonable maximum
	}
	if p == 0.5 {
		return 0
	}

	// Rational approximation for the inverse normal CDF
	// Abramowitz and Stegun formula 26.2.23
	const (
		a1 = -3.969683028665376e+01
		a2 = 2.209460984245205e+02
		a3 = -2.759285104469687e+02
		a4 = 1.383577518672690e+02
		a5 = -3.066479806614716e+01
		a6 = 2.506628277459239e+00

		b1 = -5.447609879822406e+01
		b2 = 1.615858368580409e+02
		b3 = -1.556989798598866e+02
		b4 = 6.680131188771972e+01
		b5 = -1.328068155288572e+01

		c1 = -7.784894002430293e-03
		c2 = -3.223964580411365e-01
		c3 = -2.400758277161838e+00
		c4 = -2.549732539343734e+00
		c5 = 4.374664141464968e+00
		c6 = 2.938163982698783e+00

		d1 = 7.784695709041462e-03
		d2 = 3.224671290700398e-01
		d3 = 2.445134137142996e+00
		d4 = 3.754408661907416e+00

		pLow  = 0.02425
		pHigh = 1 - pLow
	)

	var q, r float64

	if p < pLow {
		// Rational approximation for lower region
		q = math.Sqrt(-2 * math.Log(p))
		return (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	} else if p <= pHigh {
		// Rational approximation for central region
		q = p - 0.5
		r = q * q
		return (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r + a6) * q /
			(((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r + 1)
	} else {
		// Rational approximation for upper region
		q = math.Sqrt(-2 * math.Log(1-p))
		return -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
}
