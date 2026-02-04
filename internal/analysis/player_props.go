package analysis

import (
	"fmt"

	"sports-betting-bot/internal/api"
	"sports-betting-bot/internal/odds"
)

// PlayerPropOpportunity represents a +EV player prop opportunity
type PlayerPropOpportunity struct {
	GameID       int
	GameDate     string
	HomeTeam     string
	AwayTeam     string
	PlayerID     int
	PlayerName   string
	PropType     string  // "points", "rebounds", "assists", "threes"
	Line         float64 // e.g., 25.5
	Side         string  // "over" or "under"
	TrueProb     float64 // Consensus probability
	KalshiPrice  float64 // Kalshi price (0-1)
	RawEV        float64 // EV before fees
	AdjustedEV   float64 // EV after Kalshi fees
	KellyStake   float64 // Recommended stake fraction
	BookCount    int     // Number of books in consensus
}

// PlayerPropConsensus holds consensus for a single player prop
type PlayerPropConsensus struct {
	PlayerID     int
	PlayerName   string
	PropType     string
	Line         float64
	OverTrueProb float64
	UnderTrueProb float64
	KalshiOverPrice  float64
	KalshiUnderPrice float64
	BookCount    int
}

// CalculatePlayerPropConsensus calculates true probability for a player prop
// by averaging vig-removed probabilities across multiple sportsbooks
func CalculatePlayerPropConsensus(props []api.PlayerProp) *PlayerPropConsensus {
	if len(props) == 0 {
		return nil
	}

	// Group by player, prop type, and line
	first := props[0]

	var overProbs, underProbs []float64
	var kalshiOverPrice, kalshiUnderPrice float64

	for _, prop := range props {
		// Skip if not over/under market
		if prop.Market.Type != "over_under" {
			continue
		}

		// Skip if odds are zero
		if prop.Market.OverOdds == 0 || prop.Market.UnderOdds == 0 {
			continue
		}

		// Check if this is Kalshi
		if api.IsKalshi(prop.Vendor) {
			// Kalshi prices are in cents (1-99), convert to probability
			kalshiOverPrice = odds.OddsToImplied(prop.Market.OverOdds)
			kalshiUnderPrice = odds.OddsToImplied(prop.Market.UnderOdds)
			continue
		}

		// Remove vig for other books
		overProb, underProb := odds.RemoveVigFromAmerican(prop.Market.OverOdds, prop.Market.UnderOdds)
		if overProb > 0 && underProb > 0 {
			overProbs = append(overProbs, overProb)
			underProbs = append(underProbs, underProb)
		}
	}

	if len(overProbs) == 0 {
		return nil
	}

	// Calculate average true probabilities
	var overSum, underSum float64
	for i := range overProbs {
		overSum += overProbs[i]
		underSum += underProbs[i]
	}

	playerName := first.Player.FirstName + " " + first.Player.LastName

	return &PlayerPropConsensus{
		PlayerID:         first.PlayerID,
		PlayerName:       playerName,
		PropType:         first.PropType,
		Line:             first.Line,
		OverTrueProb:     overSum / float64(len(overProbs)),
		UnderTrueProb:    underSum / float64(len(underProbs)),
		KalshiOverPrice:  kalshiOverPrice,
		KalshiUnderPrice: kalshiUnderPrice,
		BookCount:        len(overProbs),
	}
}

// FindPlayerPropOpportunities finds +EV player prop bets
func FindPlayerPropOpportunities(
	props []api.PlayerProp,
	gameDate, homeTeam, awayTeam string,
	gameID int,
	cfg Config,
) []PlayerPropOpportunity {
	var opportunities []PlayerPropOpportunity

	// Group props by player+propType+line
	grouped := groupPlayerProps(props)

	for _, group := range grouped {
		consensus := CalculatePlayerPropConsensus(group)
		if consensus == nil || consensus.BookCount < cfg.MinBookCount {
			continue
		}

		// Skip if Kalshi doesn't have this prop
		if consensus.KalshiOverPrice == 0 && consensus.KalshiUnderPrice == 0 {
			continue
		}

		// Check OVER opportunity
		if consensus.KalshiOverPrice > 0 {
			adjEV := CalculateAdjustedEV(consensus.OverTrueProb, consensus.KalshiOverPrice, cfg.KalshiFee)
			if adjEV >= cfg.EVThreshold {
				opportunities = append(opportunities, PlayerPropOpportunity{
					GameID:      gameID,
					GameDate:    gameDate,
					HomeTeam:    homeTeam,
					AwayTeam:    awayTeam,
					PlayerID:    consensus.PlayerID,
					PlayerName:  consensus.PlayerName,
					PropType:    consensus.PropType,
					Line:        consensus.Line,
					Side:        "over",
					TrueProb:    consensus.OverTrueProb,
					KalshiPrice: consensus.KalshiOverPrice,
					RawEV:       CalculateEV(consensus.OverTrueProb, consensus.KalshiOverPrice),
					AdjustedEV:  adjEV,
					KellyStake:  CalculateKelly(consensus.OverTrueProb, consensus.KalshiOverPrice, cfg.KellyFraction),
					BookCount:   consensus.BookCount,
				})
			}
		}

		// Check UNDER opportunity
		if consensus.KalshiUnderPrice > 0 {
			adjEV := CalculateAdjustedEV(consensus.UnderTrueProb, consensus.KalshiUnderPrice, cfg.KalshiFee)
			if adjEV >= cfg.EVThreshold {
				opportunities = append(opportunities, PlayerPropOpportunity{
					GameID:      gameID,
					GameDate:    gameDate,
					HomeTeam:    homeTeam,
					AwayTeam:    awayTeam,
					PlayerID:    consensus.PlayerID,
					PlayerName:  consensus.PlayerName,
					PropType:    consensus.PropType,
					Line:        consensus.Line,
					Side:        "under",
					TrueProb:    consensus.UnderTrueProb,
					KalshiPrice: consensus.KalshiUnderPrice,
					RawEV:       CalculateEV(consensus.UnderTrueProb, consensus.KalshiUnderPrice),
					AdjustedEV:  adjEV,
					KellyStake:  CalculateKelly(consensus.UnderTrueProb, consensus.KalshiUnderPrice, cfg.KellyFraction),
					BookCount:   consensus.BookCount,
				})
			}
		}
	}

	return opportunities
}

// groupPlayerProps groups props by player ID, prop type, and line
func groupPlayerProps(props []api.PlayerProp) map[string][]api.PlayerProp {
	grouped := make(map[string][]api.PlayerProp)

	for _, prop := range props {
		// Only include Kalshi-supported prop types
		if !api.IsKalshiSupportedPropType(prop.PropType) {
			continue
		}

		// Create unique key for this player/prop/line combination
		key := groupKey(prop.PlayerID, prop.PropType, prop.Line)
		grouped[key] = append(grouped[key], prop)
	}

	return grouped
}

func groupKey(playerID int, propType string, line float64) string {
	return fmt.Sprintf("%d-%s-%.1f", playerID, propType, line)
}
