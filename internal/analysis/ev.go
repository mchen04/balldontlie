package analysis

import (
	"math"

	"sports-betting-bot/internal/kalshi"
	"sports-betting-bot/internal/odds"
)

// Config holds analysis configuration
type Config struct {
	EVThreshold   float64 // Minimum EV to flag opportunity (e.g., 0.03 = 3%)
	KellyFraction float64 // Fraction of Kelly to use (e.g., 0.25 = quarter Kelly)
	MinBookCount  int     // Minimum number of books required for consensus (default 4)
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		EVThreshold:   0.03,
		KellyFraction: 0.25,
		MinBookCount:  4,
	}
}

// Opportunity represents a +EV betting opportunity
type Opportunity struct {
	GameID        int
	GameDate      string  // Game date in "2006-01-02" format
	HomeTeam      string
	AwayTeam      string
	MarketType    odds.MarketType
	Side          string  // "home", "away", "over", "under"
	TrueProb      float64 // Consensus probability
	KalshiPrice   float64 // Kalshi price (0-1)
	RawEV         float64 // EV before fees
	AdjustedEV    float64 // EV after Kalshi fees
	KellyStake    float64 // Recommended stake fraction
	BookCount     int     // Number of books in consensus
}

// shrinkFullWeightAt is the book count at which shrinkage stops.
const shrinkFullWeightAt = 6

// ShrinkToward blends observed toward prior based on book count.
// At fullWeightAt books or more, returns observed unchanged.
// Below fullWeightAt, applies power-law shrinkage (exponent 1.5) which is
// more aggressive at very low book counts than linear interpolation.
//
// Weight curve (fullWeightAt=6):
//
//	6 books → weight 1.000 (no shrinkage)
//	5 books → weight 0.760
//	4 books → weight 0.544
//	3 books → weight 0.354
//	2 books → weight 0.192
//	1 book  → weight 0.068
func ShrinkToward(observed, prior float64, bookCount, fullWeightAt int) float64 {
	if bookCount >= fullWeightAt {
		return observed
	}
	ratio := float64(bookCount) / float64(fullWeightAt)
	weight := math.Pow(ratio, 1.5)
	return weight*observed + (1-weight)*prior
}

// ScaledEVThreshold raises the EV threshold when book count is low.
// At 6+ books: base. At 5: +1%. At 4: +2%.
func ScaledEVThreshold(baseThreshold float64, bookCount int) float64 {
	gap := 6 - bookCount
	if gap <= 0 {
		return baseThreshold
	}
	return baseThreshold + 0.01*float64(gap)
}

// CalculateEV calculates the expected value of a bet
// EV = (trueProb * profit) - ((1 - trueProb) * stake)
// For Kalshi: stake = price, profit = 1 - price
func CalculateEV(trueProb, kalshiPrice float64) float64 {
	if kalshiPrice <= 0 || kalshiPrice >= 1 {
		return 0
	}

	profit := 1.0 - kalshiPrice
	stake := kalshiPrice

	return (trueProb * profit) - ((1 - trueProb) * stake)
}

// CalculateAdjustedEV calculates EV minus Kalshi taker fees.
// Uses the actual Kalshi fee formula: 0.07 * price * (1-price), capped at $0.0175.
func CalculateAdjustedEV(trueProb, kalshiPrice float64) float64 {
	rawEV := CalculateEV(trueProb, kalshiPrice)
	fee := kalshi.TakerFee(kalshiPrice)
	return rawEV - fee
}

// FindMoneylineOpportunities finds +EV moneyline bets on Kalshi
func FindMoneylineOpportunities(consensus odds.ConsensusOdds, cfg Config) []Opportunity {
	var opps []Opportunity

	if consensus.Moneyline == nil || consensus.KalshiOdds == nil || consensus.KalshiOdds.Moneyline == nil {
		return opps
	}

	// Require minimum book count for reliable consensus
	if consensus.Moneyline.BookCount < cfg.MinBookCount {
		return opps
	}

	kalshi := consensus.KalshiOdds.Moneyline

	// Convert Kalshi odds to probabilities
	// Auto-detects if format is American odds (-150, +130) or Kalshi prices (45, 55)
	homeKalshiProb := odds.OddsToImplied(kalshi.Home)
	awayKalshiProb := odds.OddsToImplied(kalshi.Away)

	bc := consensus.Moneyline.BookCount

	// Shrink consensus toward Kalshi prior when book count is low
	homeProb := ShrinkToward(consensus.Moneyline.HomeTrueProb, homeKalshiProb, bc, shrinkFullWeightAt)
	awayProb := ShrinkToward(consensus.Moneyline.AwayTrueProb, awayKalshiProb, bc, shrinkFullWeightAt)

	// Check home team
	if homeKalshiProb > 0 {
		adjEV := CalculateAdjustedEV(homeProb, homeKalshiProb)
		if adjEV >= ScaledEVThreshold(cfg.EVThreshold, bc) {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketMoneyline,
				Side:        "home",
				TrueProb:    homeProb,
				KalshiPrice: homeKalshiProb,
				RawEV:       CalculateEV(homeProb, homeKalshiProb),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(homeProb, homeKalshiProb, cfg.KellyFraction),
				BookCount:   bc,
			})
		}
	}

	// Check away team
	if awayKalshiProb > 0 {
		adjEV := CalculateAdjustedEV(awayProb, awayKalshiProb)
		if adjEV >= ScaledEVThreshold(cfg.EVThreshold, bc) {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketMoneyline,
				Side:        "away",
				TrueProb:    awayProb,
				KalshiPrice: awayKalshiProb,
				RawEV:       CalculateEV(awayProb, awayKalshiProb),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(awayProb, awayKalshiProb, cfg.KellyFraction),
				BookCount:   bc,
			})
		}
	}

	return opps
}

// FindSpreadOpportunities finds +EV spread bets on Kalshi
func FindSpreadOpportunities(consensus odds.ConsensusOdds, cfg Config) []Opportunity {
	var opps []Opportunity

	if consensus.Spread == nil || consensus.KalshiOdds == nil || consensus.KalshiOdds.Spread == nil {
		return opps
	}

	// Require minimum book count for reliable consensus
	if consensus.Spread.BookCount < cfg.MinBookCount {
		return opps
	}

	kalshi := consensus.KalshiOdds.Spread

	// Auto-detect format for Kalshi odds
	homeCoverKalshi := odds.OddsToImplied(kalshi.HomeOdds)
	awayCoverKalshi := odds.OddsToImplied(kalshi.AwayOdds)

	bc := consensus.Spread.BookCount

	// Shrink consensus toward Kalshi prior when book count is low
	homeCoverProb := ShrinkToward(consensus.Spread.HomeCoverProb, homeCoverKalshi, bc, shrinkFullWeightAt)
	awayCoverProb := ShrinkToward(consensus.Spread.AwayCoverProb, awayCoverKalshi, bc, shrinkFullWeightAt)

	// Check home cover
	if homeCoverKalshi > 0 {
		adjEV := CalculateAdjustedEV(homeCoverProb, homeCoverKalshi)
		if adjEV >= ScaledEVThreshold(cfg.EVThreshold, bc) {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketSpread,
				Side:        "home",
				TrueProb:    homeCoverProb,
				KalshiPrice: homeCoverKalshi,
				RawEV:       CalculateEV(homeCoverProb, homeCoverKalshi),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(homeCoverProb, homeCoverKalshi, cfg.KellyFraction),
				BookCount:   bc,
			})
		}
	}

	// Check away cover
	if awayCoverKalshi > 0 {
		adjEV := CalculateAdjustedEV(awayCoverProb, awayCoverKalshi)
		if adjEV >= ScaledEVThreshold(cfg.EVThreshold, bc) {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketSpread,
				Side:        "away",
				TrueProb:    awayCoverProb,
				KalshiPrice: awayCoverKalshi,
				RawEV:       CalculateEV(awayCoverProb, awayCoverKalshi),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(awayCoverProb, awayCoverKalshi, cfg.KellyFraction),
				BookCount:   bc,
			})
		}
	}

	return opps
}

// FindTotalOpportunities finds +EV total (over/under) bets on Kalshi
func FindTotalOpportunities(consensus odds.ConsensusOdds, cfg Config) []Opportunity {
	var opps []Opportunity

	if consensus.Total == nil || consensus.KalshiOdds == nil || consensus.KalshiOdds.Total == nil {
		return opps
	}

	// Require minimum book count for reliable consensus
	if consensus.Total.BookCount < cfg.MinBookCount {
		return opps
	}

	kalshi := consensus.KalshiOdds.Total

	// Auto-detect format for Kalshi odds
	overKalshi := odds.OddsToImplied(kalshi.OverOdds)
	underKalshi := odds.OddsToImplied(kalshi.UnderOdds)

	bc := consensus.Total.BookCount

	// Shrink consensus toward Kalshi prior when book count is low
	overProb := ShrinkToward(consensus.Total.OverProb, overKalshi, bc, shrinkFullWeightAt)
	underProb := ShrinkToward(consensus.Total.UnderProb, underKalshi, bc, shrinkFullWeightAt)

	// Check over
	if overKalshi > 0 {
		adjEV := CalculateAdjustedEV(overProb, overKalshi)
		if adjEV >= ScaledEVThreshold(cfg.EVThreshold, bc) {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketTotal,
				Side:        "over",
				TrueProb:    overProb,
				KalshiPrice: overKalshi,
				RawEV:       CalculateEV(overProb, overKalshi),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(overProb, overKalshi, cfg.KellyFraction),
				BookCount:   bc,
			})
		}
	}

	// Check under
	if underKalshi > 0 {
		adjEV := CalculateAdjustedEV(underProb, underKalshi)
		if adjEV >= ScaledEVThreshold(cfg.EVThreshold, bc) {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketTotal,
				Side:        "under",
				TrueProb:    underProb,
				KalshiPrice: underKalshi,
				RawEV:       CalculateEV(underProb, underKalshi),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(underProb, underKalshi, cfg.KellyFraction),
				BookCount:   bc,
			})
		}
	}

	return opps
}

// FindAllOpportunities finds all +EV opportunities across all markets
func FindAllOpportunities(consensus odds.ConsensusOdds, cfg Config) []Opportunity {
	var opps []Opportunity
	opps = append(opps, FindMoneylineOpportunities(consensus, cfg)...)
	opps = append(opps, FindSpreadOpportunities(consensus, cfg)...)
	opps = append(opps, FindTotalOpportunities(consensus, cfg)...)
	return opps
}
