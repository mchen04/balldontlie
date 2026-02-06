package internal

import (
	"math"
	"os"
	"testing"

	"sports-betting-bot/internal/analysis"
	"sports-betting-bot/internal/api"
	"sports-betting-bot/internal/odds"
	"sports-betting-bot/internal/positions"
)

// TestFullPipeline tests the entire flow from API response to opportunity detection
func TestFullPipeline(t *testing.T) {
	// Create mock game data simulating a Lakers vs Celtics game
	// Multiple sportsbooks with slightly different odds
	mockGame := createMockGameOdds()

	// Step 1: Calculate consensus from multiple books
	consensus := odds.CalculateConsensus(mockGame)

	// Verify consensus was calculated
	if consensus.Moneyline == nil {
		t.Fatal("Expected moneyline consensus to be calculated")
	}
	if consensus.KalshiOdds == nil {
		t.Fatal("Expected Kalshi odds to be extracted")
	}

	t.Logf("Consensus: Home=%.3f, Away=%.3f (from %d books)",
		consensus.Moneyline.HomeTrueProb,
		consensus.Moneyline.AwayTrueProb,
		consensus.Moneyline.BookCount)

	// Verify probabilities sum to ~1.0
	probSum := consensus.Moneyline.HomeTrueProb + consensus.Moneyline.AwayTrueProb
	if math.Abs(probSum-1.0) > 0.01 {
		t.Errorf("Consensus probabilities should sum to 1.0, got %.3f", probSum)
	}

	// Step 2: Find opportunities
	cfg := analysis.Config{
		EVThreshold:   0.02, // 2% threshold for testing
		KellyFraction: 0.25,
	}

	opportunities := analysis.FindAllOpportunities(consensus, cfg)
	t.Logf("Found %d opportunities", len(opportunities))

	// With our mock data, Kalshi has worse odds than consensus, so we expect opportunities
	for _, opp := range opportunities {
		t.Logf("Opportunity: %s %s - True=%.1f%%, Kalshi=%.1f%%, AdjEV=%.2f%%, Kelly=%.2f%%",
			opp.MarketType, opp.Side,
			opp.TrueProb*100, opp.KalshiPrice*100,
			opp.AdjustedEV*100, opp.KellyStake*100)

		// Verify EV calculation is correct
		expectedRawEV := analysis.CalculateEV(opp.TrueProb, opp.KalshiPrice)
		if math.Abs(opp.RawEV-expectedRawEV) > 0.001 {
			t.Errorf("Raw EV mismatch: got %.4f, expected %.4f", opp.RawEV, expectedRawEV)
		}

		// Verify adjusted EV accounts for fees
		expectedAdjEV := analysis.CalculateAdjustedEV(opp.TrueProb, opp.KalshiPrice)
		if math.Abs(opp.AdjustedEV-expectedAdjEV) > 0.001 {
			t.Errorf("Adjusted EV mismatch: got %.4f, expected %.4f", opp.AdjustedEV, expectedAdjEV)
		}

		// Verify Kelly is positive and reasonable
		if opp.KellyStake < 0 || opp.KellyStake > 0.25 {
			t.Errorf("Kelly stake %.4f out of expected range [0, 0.25]", opp.KellyStake)
		}
	}
}

// TestPositionTrackingAndHedge tests position storage and hedge detection
func TestPositionTrackingAndHedge(t *testing.T) {
	// Use temp file for test database
	tmpFile, err := os.CreateTemp("", "test_positions_*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Initialize database
	db, err := positions.NewDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	// Add a position: bought Lakers moneyline at $0.45
	pos := positions.Position{
		GameID:     "12345",
		HomeTeam:   "LAL",
		AwayTeam:   "BOS",
		MarketType: "moneyline",
		Side:       "home",
		EntryPrice: 0.45,
		Contracts:  100,
	}

	id, err := db.AddPosition(pos)
	if err != nil {
		t.Fatalf("Failed to add position: %v", err)
	}
	t.Logf("Added position ID: %d", id)

	// Verify position was stored
	retrieved, err := db.GetPosition(id)
	if err != nil {
		t.Fatalf("Failed to get position: %v", err)
	}
	if retrieved.EntryPrice != 0.45 {
		t.Errorf("Entry price mismatch: got %.2f, expected 0.45", retrieved.EntryPrice)
	}

	// Create consensus where opposite side is now cheap (arb opportunity)
	// Entry: $0.45 for home, Current away: $0.50 -> total $0.95 < $1.00 = ARB!
	consensus := odds.ConsensusOdds{
		GameID:   12345,
		HomeTeam: "LAL",
		AwayTeam: "BOS",
		KalshiOdds: &odds.KalshiOdds{
			Moneyline: &api.Moneyline{
				Home: -100, // 50% implied
				Away: -100, // 50% implied
			},
		},
	}

	allPositions, _ := db.GetAllPositions()
	hedges := positions.FindHedgeOpportunities(allPositions, consensus)

	t.Logf("Found %d hedge opportunities", len(hedges))
	for _, h := range hedges {
		t.Logf("Hedge: %s - Profit=$%.2f", h.Action, h.GuaranteedProfit)
	}

	// With entry at $0.45 and opposite at $0.50, total = $0.95
	// Hedge fee = 0.07 * 0.50 * 0.50 = 0.0175
	// Profit = $1.00 - $0.95 - $0.0175 = $0.0325 per contract
	// For 100 contracts = $3.25
	if len(hedges) > 0 {
		expectedProfit := (1.0 - 0.45 - 0.50 - (0.07 * 0.50 * 0.50)) * 100
		if math.Abs(hedges[0].GuaranteedProfit-expectedProfit) > 0.01 {
			t.Errorf("Hedge profit mismatch: got $%.2f, expected $%.2f",
				hedges[0].GuaranteedProfit, expectedProfit)
		}
	}
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	t.Run("NoKalshiOdds", func(t *testing.T) {
		// Game with no Kalshi vendor
		game := api.GameOdds{
			GameID: 1,
			Game: api.Game{
				HomeTeam:    api.Team{Abbreviation: "LAL"},
				VisitorTeam: api.Team{Abbreviation: "BOS"},
			},
			Vendors: []api.Vendor{
				{
					Name:      "DraftKings",
					Moneyline: &api.Moneyline{Home: -150, Away: 130},
				},
			},
		}

		consensus := odds.CalculateConsensus(game)
		if consensus.KalshiOdds != nil {
			t.Error("Expected nil KalshiOdds when no Kalshi vendor")
		}

		// Should return no opportunities
		cfg := analysis.DefaultConfig()
		opps := analysis.FindAllOpportunities(consensus, cfg)
		if len(opps) != 0 {
			t.Errorf("Expected 0 opportunities without Kalshi, got %d", len(opps))
		}
	})

	t.Run("SingleBook", func(t *testing.T) {
		// Only one sportsbook (minimum viable case)
		game := api.GameOdds{
			GameID: 2,
			Game: api.Game{
				HomeTeam:    api.Team{Abbreviation: "MIA"},
				VisitorTeam: api.Team{Abbreviation: "NYK"},
			},
			Vendors: []api.Vendor{
				{
					Name:      "FanDuel",
					Moneyline: &api.Moneyline{Home: -200, Away: 170},
				},
				{
					Name:      "Kalshi",
					Moneyline: &api.Moneyline{Home: -180, Away: 150},
				},
			},
		}

		consensus := odds.CalculateConsensus(game)
		if consensus.Moneyline == nil {
			t.Fatal("Expected moneyline consensus with single book")
		}
		if consensus.Moneyline.BookCount != 1 {
			t.Errorf("Expected BookCount=1, got %d", consensus.Moneyline.BookCount)
		}
	})

	t.Run("ExtremeOdds", func(t *testing.T) {
		// Very heavy favorite
		game := api.GameOdds{
			GameID: 3,
			Vendors: []api.Vendor{
				{
					Name:      "DraftKings",
					Moneyline: &api.Moneyline{Home: -1000, Away: 700},
				},
				{
					Name:      "Kalshi",
					Moneyline: &api.Moneyline{Home: -800, Away: 550},
				},
			},
		}

		consensus := odds.CalculateConsensus(game)
		if consensus.Moneyline == nil {
			t.Fatal("Expected moneyline consensus with extreme odds")
		}

		// Heavy favorite should have probability close to 0.9+
		if consensus.Moneyline.HomeTrueProb < 0.85 {
			t.Errorf("Heavy favorite should have prob > 0.85, got %.3f",
				consensus.Moneyline.HomeTrueProb)
		}
	})

	t.Run("ZeroOdds", func(t *testing.T) {
		// Invalid zero odds should be handled
		game := api.GameOdds{
			GameID: 4,
			Vendors: []api.Vendor{
				{
					Name:      "BadBook",
					Moneyline: &api.Moneyline{Home: 0, Away: 0},
				},
			},
		}

		consensus := odds.CalculateConsensus(game)
		if consensus.Moneyline != nil {
			t.Error("Expected nil moneyline with zero odds")
		}
	})

	t.Run("MixedAvailability", func(t *testing.T) {
		// Some books have moneyline, some have spread, some have both
		game := api.GameOdds{
			GameID: 5,
			Vendors: []api.Vendor{
				{
					Name:      "Book1",
					Moneyline: &api.Moneyline{Home: -150, Away: 130},
					// No spread
				},
				{
					Name: "Book2",
					// No moneyline
					Spread: &api.Spread{HomeSpread: -5.5, HomeOdds: -110, AwaySpread: 5.5, AwayOdds: -110},
				},
				{
					Name:      "Book3",
					Moneyline: &api.Moneyline{Home: -160, Away: 140},
					Spread:    &api.Spread{HomeSpread: -5.5, HomeOdds: -105, AwaySpread: 5.5, AwayOdds: -115},
				},
				{
					Name:      "Kalshi",
					Moneyline: &api.Moneyline{Home: -140, Away: 120},
					Spread:    &api.Spread{HomeSpread: -5.5, HomeOdds: -110, AwaySpread: 5.5, AwayOdds: -110},
				},
			},
		}

		consensus := odds.CalculateConsensus(game)

		// Should have 2 books for moneyline (Book1, Book3)
		if consensus.Moneyline == nil || consensus.Moneyline.BookCount != 2 {
			t.Errorf("Expected 2 books for moneyline, got %v", consensus.Moneyline)
		}

		// Should have 2 books for spread (Book2, Book3)
		if consensus.Spread == nil || consensus.Spread.BookCount != 2 {
			t.Errorf("Expected 2 books for spread, got %v", consensus.Spread)
		}
	})
}

// TestEVCalculationAccuracy verifies EV math against known values
func TestEVCalculationAccuracy(t *testing.T) {
	testCases := []struct {
		name        string
		trueProb    float64
		kalshiPrice float64
		expectedEV  float64
	}{
		{
			name:        "5% edge on coin flip",
			trueProb:    0.55,
			kalshiPrice: 0.50,
			// Raw EV = 0.55 * 0.50 - 0.45 * 0.50 = 0.275 - 0.225 = 0.05
			// Fee = 0.07 * 0.50 * 0.50 = 0.0175
			// Adj EV = 0.05 - 0.0175 = 0.0325
			expectedEV: 0.0325,
		},
		{
			name:        "10% edge on underdog",
			trueProb:    0.35,
			kalshiPrice: 0.25,
			// Raw EV = 0.35 * 0.75 - 0.65 * 0.25 = 0.2625 - 0.1625 = 0.10
			// Fee = 0.07 * 0.25 * 0.75 = 0.013125
			// Adj EV = 0.10 - 0.013125 = 0.086875
			expectedEV: 0.086875,
		},
		{
			name:        "Negative EV (should not bet)",
			trueProb:    0.48,
			kalshiPrice: 0.50,
			// Raw EV = 0.48 * 0.50 - 0.52 * 0.50 = -0.02
			// Fee = 0.07 * 0.50 * 0.50 = 0.0175
			// Adj EV = -0.02 - 0.0175 = -0.0375
			expectedEV: -0.0375,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adjEV := analysis.CalculateAdjustedEV(tc.trueProb, tc.kalshiPrice)
			if math.Abs(adjEV-tc.expectedEV) > 0.001 {
				t.Errorf("EV calculation wrong: got %.4f, expected %.4f", adjEV, tc.expectedEV)
			}
		})
	}
}

// TestKellyAccuracy verifies Kelly criterion calculations
func TestKellyAccuracy(t *testing.T) {
	testCases := []struct {
		name          string
		trueProb      float64
		kalshiPrice   float64
		fraction      float64
		expectedKelly float64
	}{
		{
			name:        "Standard edge",
			trueProb:    0.55,
			kalshiPrice: 0.50,
			fraction:    1.0,
			// fee = 0.07 * 0.50 * 0.50 = 0.0175
			// bNet = (1 - 0.50 - 0.0175) / (0.50 + 0.0175) = 0.4825 / 0.5175 ≈ 0.9324
			// Kelly = (0.55 * 0.9324 - 0.45) / 0.9324 ≈ 0.0674
			expectedKelly: 0.0674,
		},
		{
			name:        "Quarter Kelly",
			trueProb:    0.55,
			kalshiPrice: 0.50,
			fraction:    0.25,
			// Full Kelly ≈ 0.0674, Quarter ≈ 0.0168
			expectedKelly: 0.0168,
		},
		{
			name:        "Big edge on longshot",
			trueProb:    0.30,
			kalshiPrice: 0.20,
			fraction:    1.0,
			// fee = 0.07 * 0.20 * 0.80 = 0.0112
			// bNet = (1 - 0.20 - 0.0112) / (0.20 + 0.0112) = 0.7888 / 0.2112 ≈ 3.7348
			// Kelly = (0.30 * 3.7348 - 0.70) / 3.7348 ≈ 0.1126
			expectedKelly: 0.1126,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kelly := analysis.CalculateKelly(tc.trueProb, tc.kalshiPrice, tc.fraction)
			if math.Abs(kelly-tc.expectedKelly) > 0.001 {
				t.Errorf("Kelly calculation wrong: got %.4f, expected %.4f", kelly, tc.expectedKelly)
			}
		})
	}
}

// TestHedgeProfitCalculation verifies hedge math
func TestHedgeProfitCalculation(t *testing.T) {
	// Scenario: Bought YES at $0.45, now NO is $0.50
	// Total cost = $0.95, hedge fee = 0.07 * 0.50 * 0.50 = $0.0175
	// Profit = $1.00 - $0.95 - $0.0175 = $0.0325 per contract

	contracts, profit := positions.CalculateHedgeSize(0.45, 0.50, 100)

	expectedProfit := (1.0 - 0.45 - 0.50 - (0.07 * 0.50 * 0.50)) * 100 // $3.25

	if contracts != 100 {
		t.Errorf("Expected 100 contracts, got %d", contracts)
	}

	if math.Abs(profit-expectedProfit) > 0.01 {
		t.Errorf("Hedge profit wrong: got $%.2f, expected $%.2f", profit, expectedProfit)
	}

	t.Logf("Hedge profit: $%.2f for %d contracts (%.2f%% return)",
		profit, contracts, (profit/(0.45*100))*100)
}

// TestConsensusWithDifferentSpreads verifies spread handling
func TestConsensusWithDifferentSpreads(t *testing.T) {
	// Books with same spread line but different odds
	game := api.GameOdds{
		GameID: 10,
		Vendors: []api.Vendor{
			{
				Name:   "Sharp1",
				Spread: &api.Spread{HomeSpread: -5.5, HomeOdds: -108, AwaySpread: 5.5, AwayOdds: -112},
			},
			{
				Name:   "Sharp2",
				Spread: &api.Spread{HomeSpread: -5.5, HomeOdds: -105, AwaySpread: 5.5, AwayOdds: -115},
			},
			{
				Name:   "Kalshi",
				Spread: &api.Spread{HomeSpread: -5.5, HomeOdds: -120, AwaySpread: 5.5, AwayOdds: 100},
			},
		},
	}

	consensus := odds.CalculateConsensus(game)

	if consensus.Spread == nil {
		t.Fatal("Expected spread consensus")
	}

	// Consensus should average the probabilities
	t.Logf("Spread consensus: Line=%.1f, HomeCover=%.3f, AwayCover=%.3f",
		consensus.Spread.HomeSpread,
		consensus.Spread.HomeCoverProb,
		consensus.Spread.AwayCoverProb)

	// Verify probabilities sum to ~1.0
	probSum := consensus.Spread.HomeCoverProb + consensus.Spread.AwayCoverProb
	if math.Abs(probSum-1.0) > 0.01 {
		t.Errorf("Spread probs should sum to 1.0, got %.3f", probSum)
	}
}

// TestOpportunityDetection verifies we actually detect +EV opportunities
func TestOpportunityDetection(t *testing.T) {
	// Create game where Kalshi is significantly mispriced
	game := api.GameOdds{
		GameID: 999,
		Game: api.Game{
			HomeTeam:    api.Team{Abbreviation: "LAL"},
			VisitorTeam: api.Team{Abbreviation: "BOS"},
			Status:      "scheduled",
		},
		Vendors: []api.Vendor{
			// Consensus says home team is 60% favorite
			{Name: "DraftKings", Moneyline: &api.Moneyline{Home: -150, Away: 130}},  // weight 1.5
			{Name: "Bet365", Moneyline: &api.Moneyline{Home: -148, Away: 128}},      // weight 1.3
			{Name: "FanDuel", Moneyline: &api.Moneyline{Home: -152, Away: 132}},     // weight 1.0
			// But Kalshi prices home at only 50% (major mispricing!)
			{Name: "Kalshi", Moneyline: &api.Moneyline{Home: -100, Away: -100}},
		},
	}

	consensus := odds.CalculateConsensus(game)

	t.Logf("Consensus: Home=%.1f%%, Away=%.1f%%",
		consensus.Moneyline.HomeTrueProb*100,
		consensus.Moneyline.AwayTrueProb*100)
	t.Logf("Kalshi: Home=%d (%.1f%%), Away=%d (%.1f%%)",
		consensus.KalshiOdds.Moneyline.Home,
		odds.AmericanToImplied(consensus.KalshiOdds.Moneyline.Home)*100,
		consensus.KalshiOdds.Moneyline.Away,
		odds.AmericanToImplied(consensus.KalshiOdds.Moneyline.Away)*100)

	cfg := analysis.Config{
		EVThreshold:   0.03,
		KellyFraction: 0.25,
	}

	opps := analysis.FindAllOpportunities(consensus, cfg)

	if len(opps) == 0 {
		t.Fatal("Expected to find +EV opportunity with 10% mispricing")
	}

	t.Logf("Found %d opportunities:", len(opps))
	for _, opp := range opps {
		t.Logf("  %s %s: True=%.1f%%, Kalshi=%.1f%%, AdjEV=%.2f%%, Kelly=%.2f%%",
			opp.MarketType, opp.Side,
			opp.TrueProb*100, opp.KalshiPrice*100,
			opp.AdjustedEV*100, opp.KellyStake*100)

		// Verify it's the home side (the mispriced one)
		if opp.Side != "home" {
			t.Error("Expected home side to be the opportunity")
		}

		// Verify EV is significant
		// True=58%, Price=50%, Raw EV=8%, Fee=1.75%, Adj EV=6.25%
		if opp.AdjustedEV < 0.06 {
			t.Errorf("Expected AdjustedEV > 6%%, got %.2f%%", opp.AdjustedEV*100)
		}
	}
}

// createMockGameOdds creates realistic mock data for testing
func createMockGameOdds() api.GameOdds {
	return api.GameOdds{
		ID:     1,
		GameID: 12345,
		Game: api.Game{
			ID:          12345,
			Date:        "2024-01-15",
			HomeTeam:    api.Team{ID: 1, Abbreviation: "LAL", FullName: "Los Angeles Lakers"},
			VisitorTeam: api.Team{ID: 2, Abbreviation: "BOS", FullName: "Boston Celtics"},
			Status:      "scheduled",
		},
		Vendors: []api.Vendor{
			{
				Name:      "DraftKings", // weight 1.5
				Moneyline: &api.Moneyline{Home: -145, Away: 125},
				Spread:    &api.Spread{HomeSpread: -3.5, HomeOdds: -108, AwaySpread: 3.5, AwayOdds: -112},
				Total:     &api.Total{Line: 220.5, OverOdds: -105, UnderOdds: -115},
			},
			{
				Name:      "Bet365", // weight 1.3
				Moneyline: &api.Moneyline{Home: -142, Away: 122},
				Spread:    &api.Spread{HomeSpread: -3.5, HomeOdds: -110, AwaySpread: 3.5, AwayOdds: -110},
				Total:     &api.Total{Line: 220.5, OverOdds: -108, UnderOdds: -112},
			},
			{
				Name:      "FanDuel", // weight 1.0
				Moneyline: &api.Moneyline{Home: -150, Away: 130},
				Spread:    &api.Spread{HomeSpread: -3.5, HomeOdds: -110, AwaySpread: 3.5, AwayOdds: -110},
				Total:     &api.Total{Line: 220.5, OverOdds: -110, UnderOdds: -110},
			},
			{
				Name:      "BetMGM", // weight 0.7
				Moneyline: &api.Moneyline{Home: -148, Away: 126},
				Spread:    &api.Spread{HomeSpread: -3.5, HomeOdds: -112, AwaySpread: 3.5, AwayOdds: -108},
				Total:     &api.Total{Line: 220.5, OverOdds: -112, UnderOdds: -108},
			},
			// Kalshi with slightly off prices (creates +EV opportunity)
			{
				Name:      "Kalshi",
				Moneyline: &api.Moneyline{Home: -130, Away: 110}, // Cheaper than consensus
				Spread:    &api.Spread{HomeSpread: -3.5, HomeOdds: -115, AwaySpread: 3.5, AwayOdds: -105},
				Total:     &api.Total{Line: 220.5, OverOdds: -115, UnderOdds: -105},
			},
		},
	}
}
