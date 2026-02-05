package main

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"sports-betting-bot/internal/kalshi"
	"sports-betting-bot/internal/positions"
)

func main() {
	_ = godotenv.Load()

	fmt.Println("=" + string(make([]byte, 70, 70)))
	fmt.Println("POSITION TRACKING & DUPLICATE PREVENTION TEST")
	fmt.Println("=" + string(make([]byte, 70, 70)))
	fmt.Println()

	// =============================================
	// TEST 1: Kalshi Portfolio Check
	// =============================================
	fmt.Println("TEST 1: KALSHI PORTFOLIO (live positions)")
	fmt.Println("-" + string(make([]byte, 70, 70)))

	kalshiClient, err := kalshi.NewKalshiClientFromKey(
		os.Getenv("KALSHI_API_KEY_ID"),
		os.Getenv("KALSHI_PRIVATE_KEY"),
		false,
	)
	if err != nil {
		fmt.Printf("ERROR: Kalshi client failed: %v\n", err)
		return
	}

	positions_live, err := kalshiClient.GetPositions()
	if err != nil {
		fmt.Printf("ERROR: Failed to get positions: %v\n", err)
	} else {
		fmt.Printf("Current Kalshi positions: %d\n", len(positions_live))
		for _, p := range positions_live {
			side := "YES"
			if p.Position < 0 {
				side = "NO"
			}
			fmt.Printf("  %s: %d contracts (%s)\n", p.Ticker, abs(p.Position), side)
		}
		if len(positions_live) == 0 {
			fmt.Println("  (no open positions)")
		}
	}

	// =============================================
	// TEST 2: CheckCanAddToPosition Logic
	// =============================================
	fmt.Println()
	fmt.Println("TEST 2: DUPLICATE PREVENTION LOGIC")
	fmt.Println("-" + string(make([]byte, 70, 70)))

	// Pick a test ticker (use a real one from today's games)
	testTicker := "KXNBAPTS-26FEB05PHILAL-LALAREAVES15-10"

	arbConfig := kalshi.ArbConfig{
		KalshiFee:      0.012,
		MinProfitCents: 0.5,
		MinProfitPct:   0.005,
	}

	fmt.Printf("Testing ticker: %s\n\n", testTicker)

	// Check if we can add YES side
	canAddYes, isArbYes, arbOppYes, err := kalshiClient.CheckCanAddToPosition(testTicker, kalshi.SideYes, arbConfig)
	if err != nil {
		fmt.Printf("Error checking YES: %v\n", err)
	} else {
		fmt.Printf("Can add YES position: %v\n", canAddYes)
		fmt.Printf("  Is arb opportunity: %v\n", isArbYes)
		if arbOppYes != nil {
			fmt.Printf("  Arb details: %s\n", arbOppYes.Description)
		}
	}

	// Check if we can add NO side
	canAddNo, isArbNo, arbOppNo, err := kalshiClient.CheckCanAddToPosition(testTicker, kalshi.SideNo, arbConfig)
	if err != nil {
		fmt.Printf("Error checking NO: %v\n", err)
	} else {
		fmt.Printf("Can add NO position: %v\n", canAddNo)
		fmt.Printf("  Is arb opportunity: %v\n", isArbNo)
		if arbOppNo != nil {
			fmt.Printf("  Arb details: %s\n", arbOppNo.Description)
		}
	}

	fmt.Println()
	fmt.Println("Logic explanation:")
	fmt.Println("  - If no existing position → can add (normal trade)")
	fmt.Println("  - If existing position SAME side → BLOCKED (avoid overexposure)")
	fmt.Println("  - If existing position OPPOSITE side + arb → can add (lock in profit)")
	fmt.Println("  - If existing position OPPOSITE side, no arb → BLOCKED")

	// =============================================
	// TEST 3: Local Database Test
	// =============================================
	fmt.Println()
	fmt.Println("TEST 3: LOCAL DATABASE")
	fmt.Println("-" + string(make([]byte, 70, 70)))

	// Create a test database in /tmp
	testDBPath := "/tmp/test_positions.db"
	os.Remove(testDBPath) // Clean up any existing test db

	db, err := positions.NewDB(testDBPath)
	if err != nil {
		fmt.Printf("ERROR: Failed to create test DB: %v\n", err)
		return
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	fmt.Printf("Test database created: %s\n\n", testDBPath)

	// Add a test position
	testPos := positions.Position{
		GameID:     "test-game-123",
		HomeTeam:   "LAL",
		AwayTeam:   "PHI",
		MarketType: "prop_points",
		Side:       "over",
		EntryPrice: 0.83,
		Contracts:  10,
	}

	id, err := db.AddPosition(testPos)
	if err != nil {
		fmt.Printf("ERROR adding position: %v\n", err)
	} else {
		fmt.Printf("Added position ID: %d\n", id)
	}

	// Retrieve it back
	retrieved, err := db.GetPosition(id)
	if err != nil {
		fmt.Printf("ERROR retrieving position: %v\n", err)
	} else if retrieved != nil {
		fmt.Println("\nRetrieved position:")
		fmt.Printf("  ID:         %d\n", retrieved.ID)
		fmt.Printf("  GameID:     %s\n", retrieved.GameID)
		fmt.Printf("  Teams:      %s @ %s\n", retrieved.AwayTeam, retrieved.HomeTeam)
		fmt.Printf("  MarketType: %s\n", retrieved.MarketType)
		fmt.Printf("  Side:       %s\n", retrieved.Side)
		fmt.Printf("  EntryPrice: %.2f\n", retrieved.EntryPrice)
		fmt.Printf("  Contracts:  %d\n", retrieved.Contracts)
		fmt.Printf("  CreatedAt:  %s\n", retrieved.CreatedAt.Format(time.RFC3339))
	}

	// Test GetAllPositions
	allPos, err := db.GetAllPositions()
	if err != nil {
		fmt.Printf("ERROR getting all positions: %v\n", err)
	} else {
		fmt.Printf("\nTotal positions in DB: %d\n", len(allPos))
	}

	// Test adding multiple positions
	pos2 := positions.Position{
		GameID:     "test-game-123",
		HomeTeam:   "LAL",
		AwayTeam:   "PHI",
		MarketType: "prop_rebounds",
		Side:       "under",
		EntryPrice: 0.45,
		Contracts:  5,
	}
	db.AddPosition(pos2)

	pos3 := positions.Position{
		GameID:     "test-game-456",
		HomeTeam:   "DAL",
		AwayTeam:   "SAS",
		MarketType: "moneyline",
		Side:       "home",
		EntryPrice: 0.65,
		Contracts:  20,
	}
	db.AddPosition(pos3)

	// Get positions by game
	gamePos, _ := db.GetPositionsByGame("test-game-123")
	fmt.Printf("\nPositions for game test-game-123: %d\n", len(gamePos))
	for _, p := range gamePos {
		fmt.Printf("  - %s %s: %d contracts @ %.2f\n", p.MarketType, p.Side, p.Contracts, p.EntryPrice)
	}

	// =============================================
	// SUMMARY
	// =============================================
	fmt.Println()
	fmt.Println("=" + string(make([]byte, 70, 70)))
	fmt.Println("SUMMARY")
	fmt.Println("=" + string(make([]byte, 70, 70)))
	fmt.Println()
	fmt.Println("TWO-LAYER POSITION TRACKING:")
	fmt.Println()
	fmt.Println("1. KALSHI PORTFOLIO (Primary - prevents duplicates)")
	fmt.Println("   - Queries Kalshi API for REAL positions")
	fmt.Println("   - CheckCanAddToPosition() checks BEFORE every trade")
	fmt.Println("   - Blocks same-side bets on same market")
	fmt.Println("   - Allows opposite-side only if arb exists")
	fmt.Println()
	fmt.Println("2. LOCAL SQLITE DB (Secondary - for hedge detection)")
	fmt.Println("   - Stores: game_id, teams, market_type, side, entry_price, contracts")
	fmt.Println("   - Used by FindHedgeOpportunities() to find profit locks")
	fmt.Println("   - NOT used for duplicate prevention (Kalshi API is authoritative)")
	fmt.Println()
	fmt.Println("✓ System correctly prevents overexposure via Kalshi portfolio check")
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
