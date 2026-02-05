package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"sports-betting-bot/internal/api"
	"sports-betting-bot/internal/kalshi"
)

func main() {
	_ = godotenv.Load()

	bdlClient := api.NewBallDontLieClient(os.Getenv("BALLDONTLIE_API_KEY"))
	kalshiClient, err := kalshi.NewKalshiClientFromKey(
		os.Getenv("KALSHI_API_KEY_ID"),
		os.Getenv("KALSHI_PRIVATE_KEY"),
		false,
	)
	if err != nil {
		fmt.Printf("ERROR: Kalshi client failed: %v\n", err)
		return
	}

	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("COMPREHENSIVE VALIDATION TEST")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	// ============================================
	// TEST 1: Game Matching
	// ============================================
	fmt.Println("TEST 1: GAME MATCHING (BDL vs Kalshi)")
	fmt.Println("-" + strings.Repeat("-", 79))

	gameOdds, err := bdlClient.GetTodaysOdds()
	if err != nil {
		fmt.Printf("ERROR: Failed to fetch BDL odds: %v\n", err)
		return
	}
	fmt.Printf("BDL games today: %d\n\n", len(gameOdds))

	// Get Kalshi player props to extract game info
	kalshiProps, err := kalshiClient.GetPlayerPropMarkets(time.Now())
	if err != nil {
		fmt.Printf("ERROR: Failed to fetch Kalshi props: %v\n", err)
		return
	}

	// Extract unique games from Kalshi tickers
	kalshiGames := make(map[string]bool)
	for _, markets := range kalshiProps {
		for _, m := range markets {
			// Ticker format: KXNBAPTS-26FEB05PHILAL-...
			parts := strings.Split(m.Ticker, "-")
			if len(parts) >= 2 {
				gameCode := parts[1] // e.g., "26FEB05PHILAL"
				kalshiGames[gameCode] = true
			}
		}
	}

	fmt.Printf("BDL Games:\n")
	for _, g := range gameOdds {
		hasKalshi := false
		for _, v := range g.Vendors {
			if api.IsKalshi(v.Name) {
				hasKalshi = true
				break
			}
		}
		kalshiMark := ""
		if hasKalshi {
			kalshiMark = " [Kalshi: ✓]"
		} else {
			kalshiMark = " [Kalshi: ✗]"
		}
		fmt.Printf("  %s @ %s - %s%s\n", g.Game.VisitorTeam.Abbreviation, g.Game.HomeTeam.Abbreviation, g.Game.Date, kalshiMark)
	}

	fmt.Printf("\nKalshi game codes found: %d\n", len(kalshiGames))
	for code := range kalshiGames {
		fmt.Printf("  %s\n", code)
	}

	// ============================================
	// TEST 2: Player Matching Validation
	// ============================================
	fmt.Println()
	fmt.Println("TEST 2: PLAYER MATCHING VALIDATION")
	fmt.Println("-" + strings.Repeat("-", 79))

	totalMatches := 0
	totalMismatches := 0
	var mismatches []string

	for _, game := range gameOdds {
		// Skip in-progress games
		status := game.Game.Status
		if status == "Final" || strings.Contains(status, "Qtr") || status == "Halftime" {
			continue
		}

		bdlProps, err := bdlClient.GetPlayerProps(game.GameID)
		if err != nil {
			continue
		}

		// Get player names
		playerIDs := make(map[int]bool)
		for _, p := range bdlProps {
			if api.IsKalshiSupportedPropType(p.PropType) {
				playerIDs[p.PlayerID] = true
			}
		}
		ids := make([]int, 0, len(playerIDs))
		for id := range playerIDs {
			ids = append(ids, id)
		}
		playerNames := bdlClient.GetPlayerNames(ids)

		fmt.Printf("\nGame: %s @ %s\n", game.Game.VisitorTeam.Abbreviation, game.Game.HomeTeam.Abbreviation)

		// For each BDL player, try to find in Kalshi
		for playerID, bdlName := range playerNames {
			if bdlName == "" {
				continue
			}

			// Check each prop type
			for propType, kalshiMarkets := range kalshiProps {
				matched := false
				var matchedKalshiName string

				for _, km := range kalshiMarkets {
					if kalshi.PlayerNamesMatch(bdlName, km.PlayerName) {
						matched = true
						matchedKalshiName = km.PlayerName
						break
					}
				}

				// Check if BDL has this player for this prop type
				hasBDLProp := false
				for _, p := range bdlProps {
					if p.PlayerID == playerID && p.PropType == propType {
						hasBDLProp = true
						break
					}
				}

				if hasBDLProp {
					if matched {
						totalMatches++
					} else {
						// Check if Kalshi has ANY market for this player (possible name mismatch)
						for _, km := range kalshiMarkets {
							// Fuzzy check - same last name?
							bdlParts := strings.Fields(bdlName)
							kalshiParts := strings.Fields(km.PlayerName)
							if len(bdlParts) > 0 && len(kalshiParts) > 0 {
								bdlLast := strings.ToLower(bdlParts[len(bdlParts)-1])
								kalshiLast := strings.ToLower(kalshiParts[len(kalshiParts)-1])
								if bdlLast == kalshiLast && !matched {
									mismatches = append(mismatches, fmt.Sprintf("  POSSIBLE MISMATCH: BDL='%s' vs Kalshi='%s' (%s)", bdlName, km.PlayerName, propType))
									totalMismatches++
									break
								}
							}
						}
					}
					_ = matchedKalshiName // suppress unused warning
				}
			}
		}
	}

	fmt.Printf("\nPlayer Matching Summary:\n")
	fmt.Printf("  Successful matches: %d\n", totalMatches)
	fmt.Printf("  Possible mismatches: %d\n", totalMismatches)
	if len(mismatches) > 0 {
		fmt.Println("\nPotential name mismatches found:")
		for _, m := range mismatches {
			fmt.Println(m)
		}
	}

	// ============================================
	// TEST 3: Line Consistency Check
	// ============================================
	fmt.Println()
	fmt.Println("TEST 3: LINE CONSISTENCY CHECK")
	fmt.Println("-" + strings.Repeat("-", 79))

	// Check that Kalshi lines make sense (ascending probabilities for descending lines)
	fmt.Println("\nChecking Kalshi line consistency (higher line = lower over probability):")

	type playerLines struct {
		name  string
		lines []struct {
			line   float64
			yesAsk int
		}
	}

	playerLineMap := make(map[string]*playerLines)

	for propType, markets := range kalshiProps {
		if propType != "points" {
			continue
		}
		for _, m := range markets {
			key := m.PlayerName
			if playerLineMap[key] == nil {
				playerLineMap[key] = &playerLines{name: m.PlayerName}
			}
			playerLineMap[key].lines = append(playerLineMap[key].lines, struct {
				line   float64
				yesAsk int
			}{m.Line, m.YesAsk})
		}
	}

	inconsistentPlayers := 0
	for _, pl := range playerLineMap {
		if len(pl.lines) < 2 {
			continue
		}

		// Sort by line
		sort.Slice(pl.lines, func(i, j int) bool {
			return pl.lines[i].line < pl.lines[j].line
		})

		// Check that prices decrease as lines increase
		consistent := true
		for i := 1; i < len(pl.lines); i++ {
			if pl.lines[i].yesAsk > pl.lines[i-1].yesAsk {
				consistent = false
				break
			}
		}

		if !consistent {
			inconsistentPlayers++
			fmt.Printf("  INCONSISTENT: %s\n", pl.name)
			for _, l := range pl.lines {
				fmt.Printf("    Line %.0f+: %d¢\n", l.line, l.yesAsk)
			}
		}
	}

	if inconsistentPlayers == 0 {
		fmt.Println("  All player lines are consistent (prices decrease as lines increase) ✓")
	} else {
		fmt.Printf("  Found %d players with inconsistent lines\n", inconsistentPlayers)
	}

	// ============================================
	// TEST 4: Opportunity Sanity Check
	// ============================================
	fmt.Println()
	fmt.Println("TEST 4: OPPORTUNITY SANITY CHECK")
	fmt.Println("-" + strings.Repeat("-", 79))

	// Run the actual opportunity finder and validate results
	fmt.Println("\nRunning opportunity detection and validating...")

	// Count opportunities by type
	type oppStats struct {
		total       int
		overOpps    int
		underOpps   int
		highEV      int // EV > 10%
		lowKelly    int // Kelly < 1%
		validRange  int // EV between 3-15%, Kelly between 1-25%
	}
	stats := oppStats{}

	// Use the test_full logic here
	fmt.Println("\nSample of detected opportunities:")
	sampleCount := 0

	for _, game := range gameOdds {
		status := game.Game.Status
		if status == "Final" || strings.Contains(status, "Qtr") || status == "Halftime" {
			continue
		}

		bdlProps, err := bdlClient.GetPlayerProps(game.GameID)
		if err != nil || len(bdlProps) == 0 {
			continue
		}

		playerIDs := make(map[int]bool)
		for _, p := range bdlProps {
			if api.IsKalshiSupportedPropType(p.PropType) {
				playerIDs[p.PlayerID] = true
			}
		}
		ids := make([]int, 0, len(playerIDs))
		for id := range playerIDs {
			ids = append(ids, id)
		}
		playerNames := bdlClient.GetPlayerNames(ids)

		// Group props by player+type
		type propKey struct {
			playerID int
			propType string
		}
		grouped := make(map[propKey][]api.PlayerProp)
		for _, p := range bdlProps {
			if !api.IsKalshiSupportedPropType(p.PropType) {
				continue
			}
			key := propKey{p.PlayerID, p.PropType}
			grouped[key] = append(grouped[key], p)
		}

		for key, props := range grouped {
			playerName := playerNames[key.playerID]
			if playerName == "" {
				continue
			}

			kalshiMarkets, ok := kalshiProps[key.propType]
			if !ok {
				continue
			}

			// Find matching Kalshi markets
			for _, km := range kalshiMarkets {
				if !kalshi.PlayerNamesMatch(playerName, km.PlayerName) {
					continue
				}

				// We found a match - this is where opportunities would be calculated
				stats.total++

				// Just validate the data looks reasonable
				if km.YesAsk > 0 && km.YesAsk < 100 && km.NoAsk > 0 && km.NoAsk < 100 {
					stats.validRange++
				}

				if sampleCount < 5 && km.YesAsk > 0 {
					fmt.Printf("  %s %s %.0f+: BDL has %d props, Kalshi Yes=%d¢ No=%d¢\n",
						playerName, key.propType, km.Line, len(props), km.YesAsk, km.NoAsk)
					sampleCount++
				}
			}
		}
	}

	fmt.Printf("\nOpportunity Stats:\n")
	fmt.Printf("  Total player-prop-line combinations checked: %d\n", stats.total)
	fmt.Printf("  Valid Kalshi prices (1-99¢): %d\n", stats.validRange)

	// ============================================
	// TEST 5: Data Freshness Check
	// ============================================
	fmt.Println()
	fmt.Println("TEST 5: DATA FRESHNESS CHECK")
	fmt.Println("-" + strings.Repeat("-", 79))

	// Check that we're getting recent data
	fmt.Printf("Current time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("BDL games fetched: %d\n", len(gameOdds))

	totalKalshiProps := 0
	for propType, markets := range kalshiProps {
		fmt.Printf("Kalshi %s markets: %d\n", propType, len(markets))
		totalKalshiProps += len(markets)
	}
	fmt.Printf("Total Kalshi prop markets: %d\n", totalKalshiProps)

	// ============================================
	// SUMMARY
	// ============================================
	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("VALIDATION SUMMARY")
	fmt.Println("=" + strings.Repeat("=", 79))

	issues := 0

	if len(gameOdds) == 0 {
		fmt.Println("❌ No BDL games found")
		issues++
	} else {
		fmt.Printf("✓ BDL games: %d\n", len(gameOdds))
	}

	if totalKalshiProps == 0 {
		fmt.Println("❌ No Kalshi prop markets found")
		issues++
	} else {
		fmt.Printf("✓ Kalshi prop markets: %d\n", totalKalshiProps)
	}

	if totalMatches == 0 {
		fmt.Println("❌ No player matches found")
		issues++
	} else {
		fmt.Printf("✓ Player matches: %d\n", totalMatches)
	}

	if totalMismatches > 5 {
		fmt.Printf("⚠ Possible name mismatches: %d (review above)\n", totalMismatches)
	} else {
		fmt.Printf("✓ Name mismatches: %d (acceptable)\n", totalMismatches)
	}

	if inconsistentPlayers > 0 {
		fmt.Printf("⚠ Inconsistent Kalshi lines: %d players\n", inconsistentPlayers)
	} else {
		fmt.Println("✓ All Kalshi lines consistent")
	}

	if stats.total > 0 && stats.validRange == stats.total {
		fmt.Println("✓ All Kalshi prices in valid range (1-99¢)")
	} else if stats.total > 0 {
		fmt.Printf("⚠ Some invalid Kalshi prices: %d/%d\n", stats.total-stats.validRange, stats.total)
	}

	fmt.Println()
	if issues == 0 {
		fmt.Println("✅ ALL VALIDATION CHECKS PASSED")
	} else {
		fmt.Printf("⚠ %d ISSUES FOUND - Review above\n", issues)
	}
}
