package analysis

import (
	"sports-betting-bot/internal/odds"
)

// Config holds analysis configuration
type Config struct {
	EVThreshold   float64 // Minimum EV to flag opportunity (e.g., 0.03 = 3%)
	KalshiFee     float64 // Kalshi fee percentage (e.g., 0.012 = 1.2%)
	KellyFraction float64 // Fraction of Kelly to use (e.g., 0.25 = quarter Kelly)
	MinBookCount  int     // Minimum number of books required for consensus (default 4)
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		EVThreshold:   0.03,
		KellyFraction: 0.25,
		KalshiFee:     0.012,
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

// CalculateAdjustedEV calculates EV minus Kalshi fees
func CalculateAdjustedEV(trueProb, kalshiPrice, feePct float64) float64 {
	rawEV := CalculateEV(trueProb, kalshiPrice)
	fee := kalshiPrice * feePct
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

	// Check home team
	if homeKalshiProb > 0 {
		adjEV := CalculateAdjustedEV(consensus.Moneyline.HomeTrueProb, homeKalshiProb, cfg.KalshiFee)
		if adjEV >= cfg.EVThreshold {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketMoneyline,
				Side:        "home",
				TrueProb:    consensus.Moneyline.HomeTrueProb,
				KalshiPrice: homeKalshiProb,
				RawEV:       CalculateEV(consensus.Moneyline.HomeTrueProb, homeKalshiProb),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(consensus.Moneyline.HomeTrueProb, homeKalshiProb, cfg.KellyFraction),
				BookCount:   consensus.Moneyline.BookCount,
			})
		}
	}

	// Check away team
	if awayKalshiProb > 0 {
		adjEV := CalculateAdjustedEV(consensus.Moneyline.AwayTrueProb, awayKalshiProb, cfg.KalshiFee)
		if adjEV >= cfg.EVThreshold {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketMoneyline,
				Side:        "away",
				TrueProb:    consensus.Moneyline.AwayTrueProb,
				KalshiPrice: awayKalshiProb,
				RawEV:       CalculateEV(consensus.Moneyline.AwayTrueProb, awayKalshiProb),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(consensus.Moneyline.AwayTrueProb, awayKalshiProb, cfg.KellyFraction),
				BookCount:   consensus.Moneyline.BookCount,
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

	// Check home cover
	if homeCoverKalshi > 0 {
		adjEV := CalculateAdjustedEV(consensus.Spread.HomeCoverProb, homeCoverKalshi, cfg.KalshiFee)
		if adjEV >= cfg.EVThreshold {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketSpread,
				Side:        "home",
				TrueProb:    consensus.Spread.HomeCoverProb,
				KalshiPrice: homeCoverKalshi,
				RawEV:       CalculateEV(consensus.Spread.HomeCoverProb, homeCoverKalshi),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(consensus.Spread.HomeCoverProb, homeCoverKalshi, cfg.KellyFraction),
				BookCount:   consensus.Spread.BookCount,
			})
		}
	}

	// Check away cover
	if awayCoverKalshi > 0 {
		adjEV := CalculateAdjustedEV(consensus.Spread.AwayCoverProb, awayCoverKalshi, cfg.KalshiFee)
		if adjEV >= cfg.EVThreshold {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketSpread,
				Side:        "away",
				TrueProb:    consensus.Spread.AwayCoverProb,
				KalshiPrice: awayCoverKalshi,
				RawEV:       CalculateEV(consensus.Spread.AwayCoverProb, awayCoverKalshi),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(consensus.Spread.AwayCoverProb, awayCoverKalshi, cfg.KellyFraction),
				BookCount:   consensus.Spread.BookCount,
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

	// Check over
	if overKalshi > 0 {
		adjEV := CalculateAdjustedEV(consensus.Total.OverProb, overKalshi, cfg.KalshiFee)
		if adjEV >= cfg.EVThreshold {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketTotal,
				Side:        "over",
				TrueProb:    consensus.Total.OverProb,
				KalshiPrice: overKalshi,
				RawEV:       CalculateEV(consensus.Total.OverProb, overKalshi),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(consensus.Total.OverProb, overKalshi, cfg.KellyFraction),
				BookCount:   consensus.Total.BookCount,
			})
		}
	}

	// Check under
	if underKalshi > 0 {
		adjEV := CalculateAdjustedEV(consensus.Total.UnderProb, underKalshi, cfg.KalshiFee)
		if adjEV >= cfg.EVThreshold {
			opps = append(opps, Opportunity{
				GameID:      consensus.GameID,
				GameDate:    consensus.GameDate,
				HomeTeam:    consensus.HomeTeam,
				AwayTeam:    consensus.AwayTeam,
				MarketType:  odds.MarketTotal,
				Side:        "under",
				TrueProb:    consensus.Total.UnderProb,
				KalshiPrice: underKalshi,
				RawEV:       CalculateEV(consensus.Total.UnderProb, underKalshi),
				AdjustedEV:  adjEV,
				KellyStake:  CalculateKelly(consensus.Total.UnderProb, underKalshi, cfg.KellyFraction),
				BookCount:   consensus.Total.BookCount,
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
