package analysis

import (
	"fmt"

	"sports-betting-bot/internal/api"
	"sports-betting-bot/internal/kalshi"
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
	KalshiTicker string  // Full Kalshi market ticker for order execution
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

	type wp struct {
		over, under, weight float64
	}
	var probs []wp
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

		// Remove vig for other books using Power method (accounts for FLB bias)
		overProb, underProb := odds.RemoveVigPowerFromAmerican(prop.Market.OverOdds, prop.Market.UnderOdds)
		if overProb > 0 && underProb > 0 {
			probs = append(probs, wp{overProb, underProb, api.VendorPropWeight(prop.Vendor)})
		}
	}

	if len(probs) == 0 {
		return nil
	}

	// Calculate weighted average true probabilities
	var overSum, underSum, wSum float64
	for _, p := range probs {
		overSum += p.over * p.weight
		underSum += p.under * p.weight
		wSum += p.weight
	}

	// Player name not included in v2 API, use ID as placeholder
	playerName := fmt.Sprintf("Player_%d", first.PlayerID)

	return &PlayerPropConsensus{
		PlayerID:         first.PlayerID,
		PlayerName:       playerName,
		PropType:         first.PropType,
		Line:             first.Line(),
		OverTrueProb:     overSum / wSum,
		UnderTrueProb:    underSum / wSum,
		KalshiOverPrice:  kalshiOverPrice,
		KalshiUnderPrice: kalshiUnderPrice,
		BookCount:        len(probs),
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
			adjEV := CalculateAdjustedEV(consensus.OverTrueProb, consensus.KalshiOverPrice)
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
			adjEV := CalculateAdjustedEV(consensus.UnderTrueProb, consensus.KalshiUnderPrice)
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
		key := groupKey(prop.PlayerID, prop.PropType, prop.Line())
		grouped[key] = append(grouped[key], prop)
	}

	return grouped
}

func groupKey(playerID int, propType string, line float64) string {
	return fmt.Sprintf("%d-%s-%.1f", playerID, propType, line)
}

// FindPlayerPropOpportunitiesWithKalshi finds +EV player prop bets using direct Kalshi data
// This matches Ball Don't Lie consensus with Kalshi prices fetched directly from Kalshi API
// playerNames maps BDL player_id to player name (required for proper matching)
func FindPlayerPropOpportunitiesWithKalshi(
	bdlProps []api.PlayerProp,
	kalshiProps map[string][]kalshi.PlayerPropMarket, // keyed by prop type
	playerNames map[int]string, // BDL player_id -> "FirstName LastName"
	gameDate, homeTeam, awayTeam string,
	gameID int,
	cfg Config,
) []PlayerPropOpportunity {
	var opportunities []PlayerPropOpportunity

	// Group BDL props by player ID + prop type + line
	type propKey struct {
		PlayerID int
		PropType string
		Line     float64
	}
	grouped := make(map[propKey][]api.PlayerProp)

	for _, prop := range bdlProps {
		if !api.IsKalshiSupportedPropType(prop.PropType) {
			continue
		}
		key := propKey{
			PlayerID: prop.PlayerID,
			PropType: prop.PropType,
			Line:     prop.Line(),
		}
		grouped[key] = append(grouped[key], prop)
	}

	// For each BDL prop group, calculate consensus and find matching Kalshi market
	for key, group := range grouped {
		// Calculate consensus from BDL books (excluding Kalshi since it's not in BDL)
		consensus := calculateBDLConsensus(group)
		if consensus == nil || consensus.BookCount < cfg.MinBookCount {
			continue
		}

		// Get player name from the map
		playerName, ok := playerNames[key.PlayerID]
		if !ok || playerName == "" {
			// Can't match without player name
			continue
		}

		// Find matching Kalshi market by BOTH player name AND line
		kalshiMarkets, ok := kalshiProps[key.PropType]
		if !ok || len(kalshiMarkets) == 0 {
			continue
		}

		// Use the proper matching function that checks player name AND line
		matchedKalshi := kalshi.FindMatchingKalshiProp(playerName, key.PropType, key.Line, kalshiMarkets)
		if matchedKalshi == nil {
			continue
		}

		// Calculate Kalshi prices from bid/ask
		// For OVER (YES): use yes_ask (price to buy YES)
		// For UNDER (NO): use no_ask (price to buy NO)
		// Note: We only use actual ask prices - if NoAsk is 0, there's no seller
		kalshiOverPrice := float64(matchedKalshi.YesAsk) / 100.0
		kalshiUnderPrice := float64(matchedKalshi.NoAsk) / 100.0

		// Check OVER opportunity (YES on Kalshi)
		if kalshiOverPrice > 0 && kalshiOverPrice < 1 {
			adjEV := CalculateAdjustedEV(consensus.OverTrueProb, kalshiOverPrice)
			if adjEV >= cfg.EVThreshold {
				opportunities = append(opportunities, PlayerPropOpportunity{
					GameID:       gameID,
					GameDate:     gameDate,
					HomeTeam:     homeTeam,
					AwayTeam:     awayTeam,
					PlayerID:     key.PlayerID,
					PlayerName:   playerName,
					PropType:     key.PropType,
					Line:         matchedKalshi.Line,
					Side:         "over",
					TrueProb:     consensus.OverTrueProb,
					KalshiPrice:  kalshiOverPrice,
					RawEV:        CalculateEV(consensus.OverTrueProb, kalshiOverPrice),
					AdjustedEV:   adjEV,
					KellyStake:   CalculateKelly(consensus.OverTrueProb, kalshiOverPrice, cfg.KellyFraction),
					BookCount:    consensus.BookCount,
					KalshiTicker: matchedKalshi.Ticker,
				})
			}
		}

		// Check UNDER opportunity (NO on Kalshi)
		if kalshiUnderPrice > 0 && kalshiUnderPrice < 1 {
			adjEV := CalculateAdjustedEV(consensus.UnderTrueProb, kalshiUnderPrice)
			if adjEV >= cfg.EVThreshold {
				opportunities = append(opportunities, PlayerPropOpportunity{
					GameID:       gameID,
					GameDate:     gameDate,
					HomeTeam:     homeTeam,
					AwayTeam:     awayTeam,
					PlayerID:     key.PlayerID,
					PlayerName:   playerName,
					PropType:     key.PropType,
					Line:         matchedKalshi.Line,
					Side:         "under",
					TrueProb:     consensus.UnderTrueProb,
					KalshiPrice:  kalshiUnderPrice,
					RawEV:        CalculateEV(consensus.UnderTrueProb, kalshiUnderPrice),
					AdjustedEV:   adjEV,
					KellyStake:   CalculateKelly(consensus.UnderTrueProb, kalshiUnderPrice, cfg.KellyFraction),
					BookCount:    consensus.BookCount,
					KalshiTicker: matchedKalshi.Ticker,
				})
			}
		}
	}

	return opportunities
}

// FindPlayerPropOpportunitiesWithInterpolation finds +EV player prop bets using distribution interpolation
// This allows comparing BDL lines (e.g., over 19.5) with different Kalshi lines (e.g., 25+)
// by fitting a probability distribution and estimating the true probability at any threshold
func FindPlayerPropOpportunitiesWithInterpolation(
	bdlProps []api.PlayerProp,
	kalshiProps map[string][]kalshi.PlayerPropMarket,
	playerNames map[int]string,
	gameDate, homeTeam, awayTeam string,
	gameID int,
	cfg Config,
) []PlayerPropOpportunity {
	var opportunities []PlayerPropOpportunity

	// Group BDL props by player ID + prop type (NOT line - we want all lines for a player)
	type playerPropKey struct {
		PlayerID int
		PropType string
	}
	type lineData struct {
		Line      float64
		OverProb  float64
		UnderProb float64
		BookCount int
	}
	playerProps := make(map[playerPropKey][]lineData)

	// First pass: collect all BDL lines for each player+propType
	type propKey struct {
		PlayerID int
		PropType string
		Line     float64
	}
	grouped := make(map[propKey][]api.PlayerProp)

	for _, prop := range bdlProps {
		if !api.IsKalshiSupportedPropType(prop.PropType) {
			continue
		}
		key := propKey{
			PlayerID: prop.PlayerID,
			PropType: prop.PropType,
			Line:     prop.Line(),
		}
		grouped[key] = append(grouped[key], prop)
	}

	// Calculate consensus for each line
	for key, group := range grouped {
		consensus := calculateBDLConsensus(group)
		if consensus == nil || consensus.BookCount < cfg.MinBookCount {
			continue
		}

		ppKey := playerPropKey{PlayerID: key.PlayerID, PropType: key.PropType}
		playerProps[ppKey] = append(playerProps[ppKey], lineData{
			Line:      key.Line,
			OverProb:  consensus.OverTrueProb,
			UnderProb: consensus.UnderTrueProb,
			BookCount: consensus.BookCount,
		})
	}

	// For each player+propType, find Kalshi markets and estimate probabilities
	for ppKey, lines := range playerProps {
		playerName, ok := playerNames[ppKey.PlayerID]
		if !ok || playerName == "" {
			continue
		}

		kalshiMarkets, ok := kalshiProps[ppKey.PropType]
		if !ok || len(kalshiMarkets) == 0 {
			continue
		}

		// Find all Kalshi markets for this player
		for _, km := range kalshiMarkets {
			if !kalshi.PlayerNamesMatch(playerName, km.PlayerName) {
				continue
			}

			// Sort BDL lines and extract probabilities
			var bdlLines []float64
			var bdlOverProbs []float64
			var bdlUnderProbs []float64
			var totalBooks int

			for _, ld := range lines {
				bdlLines = append(bdlLines, ld.Line)
				bdlOverProbs = append(bdlOverProbs, ld.OverProb)
				bdlUnderProbs = append(bdlUnderProbs, ld.UnderProb)
				totalBooks += ld.BookCount
			}
			avgBooks := totalBooks / len(lines)

			// Use distribution interpolation to estimate probability at Kalshi line
			kalshiLine := km.Line

			// For OVER: estimate P(X >= kalshiLine)
			var estimatedOverProb float64
			if len(bdlLines) == 1 {
				estimatedOverProb = EstimateProbabilityAtLine(bdlLines[0], bdlOverProbs[0], kalshiLine, ppKey.PropType)
			} else {
				estimatedOverProb = EstimateProbabilityFromMultipleLines(bdlLines, bdlOverProbs, kalshiLine, ppKey.PropType)
			}

			// For UNDER: P(X < kalshiLine) = 1 - P(X >= kalshiLine)
			estimatedUnderProb := 1 - estimatedOverProb

			// Skip if estimation failed
			if estimatedOverProb <= 0 || estimatedOverProb >= 1 {
				continue
			}

			// Get Kalshi prices - only use actual ask prices (no seller = no trade)
			kalshiOverPrice := float64(km.YesAsk) / 100.0
			kalshiUnderPrice := float64(km.NoAsk) / 100.0

			// Check OVER opportunity
			if kalshiOverPrice > 0 && kalshiOverPrice < 1 {
				adjEV := CalculateAdjustedEV(estimatedOverProb, kalshiOverPrice)
				if adjEV >= cfg.EVThreshold {
					opportunities = append(opportunities, PlayerPropOpportunity{
						GameID:       gameID,
						GameDate:     gameDate,
						HomeTeam:     homeTeam,
						AwayTeam:     awayTeam,
						PlayerID:     ppKey.PlayerID,
						PlayerName:   playerName,
						PropType:     ppKey.PropType,
						Line:         kalshiLine,
						Side:         "over",
						TrueProb:     estimatedOverProb,
						KalshiPrice:  kalshiOverPrice,
						RawEV:        CalculateEV(estimatedOverProb, kalshiOverPrice),
						AdjustedEV:   adjEV,
						KellyStake:   CalculateKelly(estimatedOverProb, kalshiOverPrice, cfg.KellyFraction),
						BookCount:    avgBooks,
						KalshiTicker: km.Ticker,
					})
				}
			}

			// Check UNDER opportunity
			if kalshiUnderPrice > 0 && kalshiUnderPrice < 1 {
				adjEV := CalculateAdjustedEV(estimatedUnderProb, kalshiUnderPrice)
				if adjEV >= cfg.EVThreshold {
					opportunities = append(opportunities, PlayerPropOpportunity{
						GameID:       gameID,
						GameDate:     gameDate,
						HomeTeam:     homeTeam,
						AwayTeam:     awayTeam,
						PlayerID:     ppKey.PlayerID,
						PlayerName:   playerName,
						PropType:     ppKey.PropType,
						Line:         kalshiLine,
						Side:         "under",
						TrueProb:     estimatedUnderProb,
						KalshiPrice:  kalshiUnderPrice,
						RawEV:        CalculateEV(estimatedUnderProb, kalshiUnderPrice),
						AdjustedEV:   adjEV,
						KellyStake:   CalculateKelly(estimatedUnderProb, kalshiUnderPrice, cfg.KellyFraction),
						BookCount:    avgBooks,
						KalshiTicker: km.Ticker,
					})
				}
			}
		}
	}

	return opportunities
}

// calculateBDLConsensus calculates consensus from Ball Don't Lie props only (no Kalshi)
// Applies vendor weighting per api.VendorPropWeight (e.g. FanDuel 1.5x, BetMGM 0.7x)
func calculateBDLConsensus(props []api.PlayerProp) *PlayerPropConsensus {
	if len(props) == 0 {
		return nil
	}

	first := props[0]

	type weightedProb struct {
		over, under, weight float64
	}
	var probs []weightedProb

	for _, prop := range props {
		// Skip non over/under markets
		if prop.Market.Type != "over_under" {
			continue
		}

		// Skip if odds are zero
		if prop.Market.OverOdds == 0 || prop.Market.UnderOdds == 0 {
			continue
		}

		// Skip Kalshi (we get their prices directly)
		if api.IsKalshi(prop.Vendor) {
			continue
		}

		// Remove vig using Power method (accounts for FLB bias)
		overProb, underProb := odds.RemoveVigPowerFromAmerican(prop.Market.OverOdds, prop.Market.UnderOdds)
		if overProb > 0 && underProb > 0 {
			probs = append(probs, weightedProb{overProb, underProb, api.VendorPropWeight(prop.Vendor)})
		}
	}

	if len(probs) == 0 {
		return nil
	}

	var overSum, underSum, wSum float64
	for _, p := range probs {
		overSum += p.over * p.weight
		underSum += p.under * p.weight
		wSum += p.weight
	}

	return &PlayerPropConsensus{
		PlayerID:      first.PlayerID,
		PlayerName:    fmt.Sprintf("Player_%d", first.PlayerID),
		PropType:      first.PropType,
		Line:          first.Line(),
		OverTrueProb:  overSum / wSum,
		UnderTrueProb: underSum / wSum,
		BookCount:     len(probs),
	}
}
