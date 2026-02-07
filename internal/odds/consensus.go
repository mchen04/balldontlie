package odds

import (
	"math"
	"time"

	"sports-betting-bot/internal/api"
	"sports-betting-bot/internal/mathutil"
)

const (
	// NBASpreadStdDev is the standard deviation of NBA margin vs spread
	// Based on historical data: ATS margin follows N(0, ~11.5)
	// Sources: Boyd's Bets, Wayne Winston Mathletics
	NBASpreadStdDev = 11.5

	// NBATotalStdDev is the standard deviation for NBA totals
	// Based on Boyd's Bets O/U margin data (empirical range 15-21)
	NBATotalStdDev = 17.0

	// NBASpreadDF is the degrees of freedom for t-distribution on NBA spreads.
	// Empirical kurtosis ~3.7 → fatter tails than normal.
	NBASpreadDF = 7.0

	// NBATotalDF is the degrees of freedom for t-distribution on NBA totals.
	// Totals have slightly thinner tails than spreads.
	NBATotalDF = 9.0
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

// weightedProb holds a probability pair with its consensus weight
type weightedProb struct {
	a, b   float64
	weight float64
}

// isVendorFresh checks if a vendor's odds are within the staleness threshold.
// Returns true if maxAgeSec is 0 (no filtering) or if the vendor's UpdatedAt
// is within maxAgeSec of now.
func isVendorFresh(vendor api.Vendor, maxAgeSec int) bool {
	if maxAgeSec <= 0 || vendor.UpdatedAt == "" {
		return true // No filtering or no timestamp
	}
	t, err := time.Parse(time.RFC3339, vendor.UpdatedAt)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05Z", vendor.UpdatedAt)
		if err != nil {
			return true // Can't parse, assume fresh
		}
	}
	return time.Since(t) <= time.Duration(maxAgeSec)*time.Second
}

// CalculateConsensus computes consensus true probabilities from multiple vendors
// Normalizes spread/total probabilities to match Kalshi's line
// Vendors are weighted per api.VendorGameWeight (e.g. DraftKings 1.5x, BetMGM 0.7x)
func CalculateConsensus(gameOdds api.GameOdds, maxOddsAgeSec ...int) ConsensusOdds {
	maxAge := 0
	if len(maxOddsAgeSec) > 0 {
		maxAge = maxOddsAgeSec[0]
	}
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

	var mlProbs []weightedProb
	var spreadProbs []weightedProb
	var totalProbs []weightedProb

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

		// Skip stale vendor odds
		if !isVendorFresh(vendor, maxAge) {
			continue
		}

		w := api.VendorGameWeight(vendor.Name)

		// Moneyline (no normalization needed)
		if vendor.Moneyline != nil && vendor.Moneyline.Home != 0 && vendor.Moneyline.Away != 0 {
			homeProb, awayProb := RemoveVigPowerFromAmerican(vendor.Moneyline.Home, vendor.Moneyline.Away)
			if homeProb > 0 && awayProb > 0 {
				mlProbs = append(mlProbs, weightedProb{homeProb, awayProb, w})
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
				spreadProbs = append(spreadProbs, weightedProb{homeCover, awayCover, w})
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
				totalProbs = append(totalProbs, weightedProb{overProb, underProb, w})
			}
		}
	}

	// Calculate weighted averages across all books
	if len(mlProbs) > 0 {
		homeProb, awayProb := logLinearConsensus(mlProbs)
		consensus.Moneyline = &MoneylineConsensus{
			HomeTrueProb: homeProb,
			AwayTrueProb: awayProb,
			BookCount:    len(mlProbs),
		}
	}

	if len(spreadProbs) > 0 {
		homeCover, awayCover := logLinearConsensus(spreadProbs)
		consensus.Spread = &SpreadConsensus{
			HomeSpread:    kalshiSpreadLine, // Use Kalshi line as the reference
			HomeCoverProb: homeCover,
			AwayCoverProb: awayCover,
			BookCount:     len(spreadProbs),
		}
	}

	if len(totalProbs) > 0 {
		overProb, underProb := logLinearConsensus(totalProbs)
		consensus.Total = &TotalConsensus{
			Line:      kalshiTotalLine, // Use Kalshi line as the reference
			OverProb:  overProb,
			UnderProb: underProb,
			BookCount: len(totalProbs),
		}
	}

	return consensus
}

// logLinearConsensus averages probabilities in logit space (log-linear opinion pool).
// Applies winsorization (±2σ) when 3+ books to cap outlier influence.
// Returns (sigmoid(weightedAvgLogit), 1 - sigmoid(weightedAvgLogit)).
func logLinearConsensus(probs []weightedProb) (float64, float64) {
	logits := make([]float64, len(probs))
	weights := make([]float64, len(probs))
	for i, p := range probs {
		logits[i] = mathutil.Logit(p.a)
		weights[i] = p.weight
	}

	// Winsorize outliers at ±2σ when we have enough data points
	mathutil.WinsorizeLogits(logits, weights, 2.0)

	var logitSum, wSum float64
	for i := range logits {
		logitSum += logits[i] * weights[i]
		wSum += weights[i]
	}
	a := mathutil.Sigmoid(logitSum / wSum)
	return a, 1 - a
}

// normalizeSpreadProb adjusts spread probabilities from bookLine to targetLine
// Uses normal distribution model: ATS margin ~ N(0, σ) where σ ≈ 11.5 for NBA
//
// Example: Book has home -6.0 at 50% cover, target is -5.5
// -5.5 is easier to cover (smaller number to beat), so probability increases
//
// For negative spreads: larger absolute value = harder to cover
// Moving from -6.0 to -5.5 = easier = higher cover probability
// spreadSD returns the context-dependent standard deviation for NBA spreads.
// Close games have tighter distributions; blowout-prone games are wider.
func spreadSD(homeSpread float64) float64 {
	abs := homeSpread
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs <= 3:
		return 10.5 // Close matchup: tighter distribution
	case abs <= 7:
		return 11.5 // Standard
	default:
		return 12.5 // Large spread: wider variance
	}
}

// totalSD returns the context-dependent standard deviation for NBA totals.
// Higher-scoring games (higher pace) have wider variance.
func totalSD(totalLine float64) float64 {
	switch {
	case totalLine < 215:
		return 15.5
	case totalLine <= 230:
		return 17.0
	default:
		return 18.5
	}
}

func normalizeSpreadProb(homeCover, awayCover, bookLine, targetLine float64) (float64, float64) {
	if bookLine == targetLine {
		return homeCover, awayCover
	}

	sd := spreadSD(targetLine)

	// Convert book's cover probability to a t-score (fat-tailed)
	bookT := mathutil.TDistInvCDF(homeCover, NBASpreadDF)

	// Line difference: positive when target is easier for home to cover
	lineDiff := targetLine - bookLine

	// Adjust t-score: easier target = higher probability = higher t
	targetT := bookT + (lineDiff / sd)

	// Convert back to probability using t-distribution CDF
	adjustedHome := mathutil.TDistCDF(targetT, NBASpreadDF)
	adjustedAway := 1.0 - adjustedHome

	// Clamp to valid range, then derive away to preserve sum-to-1
	adjustedHome = math.Max(0.01, math.Min(0.99, adjustedHome))
	adjustedAway = 1.0 - adjustedHome

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

	sd := totalSD(targetLine)

	// Convert book's over probability to t-score (fat-tailed)
	bookT := mathutil.TDistInvCDF(overProb, NBATotalDF)

	// For totals: lower target = easier to go over
	// lineDiff > 0 means target is higher (harder to go over)
	lineDiff := targetLine - bookLine
	targetT := bookT - (lineDiff / sd)

	// Convert back to probability using t-distribution CDF
	adjustedOver := mathutil.TDistCDF(targetT, NBATotalDF)
	adjustedUnder := 1.0 - adjustedOver

	// Clamp to valid range, then derive under to preserve sum-to-1
	adjustedOver = math.Max(0.01, math.Min(0.99, adjustedOver))
	adjustedUnder = 1.0 - adjustedOver

	return adjustedOver, adjustedUnder
}
