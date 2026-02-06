package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"sports-betting-bot/internal/alerts"
	"sports-betting-bot/internal/analysis"
	"sports-betting-bot/internal/api"
	"sports-betting-bot/internal/kalshi"
	"sports-betting-bot/internal/odds"
	"sports-betting-bot/internal/positions"
)

var lastMaintenanceLog time.Time

type Config struct {
	APIKey        string
	EVThreshold   float64
	KalshiFee     float64
	KellyFraction float64
	PollInterval  time.Duration
	DBPath        string
	Port          string

	// Kalshi API settings (API key auth only - email/password deprecated)
	KalshiAPIKeyID   string
	KalshiAPIKeyPath string // For local dev (file path)
	KalshiPrivateKey string // For cloud deployment (key content directly)
	KalshiDemo       bool

	// Execution settings
	AutoExecute           bool
	MaxSlippagePct        float64
	MinLiquidityContracts int
	MaxBetDollars         float64
}

func loadConfig() Config {
	_ = godotenv.Load() // Ignore error if .env doesn't exist

	cfg := Config{
		APIKey:        os.Getenv("BALLDONTLIE_API_KEY"),
		EVThreshold:   0.03,
		KalshiFee:     0.012,
		KellyFraction: 0.25,
		PollInterval:  2 * time.Second, // 2 API calls per poll, so 1 req/sec = 60 req/min (well under 600 limit)
		DBPath:        "/data/positions.db",
		Port:          "8080",

		// Kalshi API key auth (email/password deprecated by Kalshi)
		KalshiAPIKeyID:   os.Getenv("KALSHI_API_KEY_ID"),
		KalshiAPIKeyPath: os.Getenv("KALSHI_API_KEY_PATH"), // Local dev: file path
		KalshiPrivateKey: os.Getenv("KALSHI_PRIVATE_KEY"),  // Cloud: key content directly
		KalshiDemo:       os.Getenv("KALSHI_DEMO") == "true",

		// Execution defaults
		AutoExecute:           false,
		MaxSlippagePct:        0.02,
		MinLiquidityContracts: 10,
		MaxBetDollars:         0, // 0 = no cap
	}

	if v := os.Getenv("EV_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.EVThreshold = f
		}
	}

	if v := os.Getenv("KALSHI_FEE_PCT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.KalshiFee = f
		}
	}

	if v := os.Getenv("KELLY_FRACTION"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.KellyFraction = f
		}
	}

	if v := os.Getenv("POLL_INTERVAL_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil {
			cfg.PollInterval = time.Duration(ms) * time.Millisecond
		}
	}

	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}

	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}

	// Execution settings
	if os.Getenv("AUTO_EXECUTE") == "true" {
		cfg.AutoExecute = true
	}

	if v := os.Getenv("MAX_SLIPPAGE_PCT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.MaxSlippagePct = f
		}
	}

	if v := os.Getenv("MIN_LIQUIDITY_CONTRACTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MinLiquidityContracts = n
		}
	}

	if v := os.Getenv("MAX_BET_DOLLARS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.MaxBetDollars = f
		}
	}

	return cfg
}

func validateConfig(cfg Config) error {
	if cfg.EVThreshold < 0 || cfg.EVThreshold > 1 {
		return fmt.Errorf("EV_THRESHOLD must be between 0 and 1, got %f", cfg.EVThreshold)
	}
	if cfg.KalshiFee < 0 || cfg.KalshiFee > 1 {
		return fmt.Errorf("KALSHI_FEE_PCT must be between 0 and 1, got %f", cfg.KalshiFee)
	}
	if cfg.KellyFraction <= 0 || cfg.KellyFraction > 1 {
		return fmt.Errorf("KELLY_FRACTION must be between 0 and 1, got %f", cfg.KellyFraction)
	}
	if cfg.MaxSlippagePct < 0 || cfg.MaxSlippagePct > 1 {
		return fmt.Errorf("MAX_SLIPPAGE_PCT must be between 0 and 1, got %f", cfg.MaxSlippagePct)
	}
	if cfg.MinLiquidityContracts < 0 {
		return fmt.Errorf("MIN_LIQUIDITY_CONTRACTS must be non-negative, got %d", cfg.MinLiquidityContracts)
	}
	if cfg.MaxBetDollars < 0 {
		return fmt.Errorf("MAX_BET_DOLLARS must be non-negative, got %f", cfg.MaxBetDollars)
	}
	if cfg.PollInterval < 10*time.Millisecond {
		return fmt.Errorf("POLL_INTERVAL_MS must be at least 10ms, got %v", cfg.PollInterval)
	}
	return nil
}

func main() {
	cfg := loadConfig()

	if err := validateConfig(cfg); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	if cfg.APIKey == "" {
		log.Fatal("BALLDONTLIE_API_KEY is required")
	}

	// Initialize components
	client := api.NewBallDontLieClient(cfg.APIKey)
	notifier := alerts.NewNotifier(5 * time.Minute) // 5 min cooldown between same alerts

	analysisConfig := analysis.Config{
		EVThreshold:   cfg.EVThreshold,
		KalshiFee:     cfg.KalshiFee,
		KellyFraction: cfg.KellyFraction,
		MinBookCount:  4, // Require at least 4 books for reliable consensus
	}

	// Initialize Kalshi client if API key credentials provided
	// Note: Kalshi requires API key auth (email/password deprecated)
	// Supports two modes:
	//   1. Cloud: KALSHI_PRIVATE_KEY env var with key content (preferred for Fly.io)
	//   2. Local: KALSHI_API_KEY_PATH env var with file path
	var kalshiClient *kalshi.KalshiClient
	if cfg.KalshiAPIKeyID != "" && (cfg.KalshiPrivateKey != "" || cfg.KalshiAPIKeyPath != "") {
		var err error
		if cfg.KalshiPrivateKey != "" {
			// Cloud mode: key content passed directly via env var
			kalshiClient, err = kalshi.NewKalshiClientFromKey(cfg.KalshiAPIKeyID, cfg.KalshiPrivateKey, cfg.KalshiDemo)
		} else {
			// Local mode: key loaded from file
			kalshiClient, err = kalshi.NewKalshiClient(cfg.KalshiAPIKeyID, cfg.KalshiAPIKeyPath, cfg.KalshiDemo)
		}
		if err != nil {
			log.Printf("Kalshi disabled: %v", err)
		} else {
			mode := "production"
			if cfg.KalshiDemo {
				mode = "demo"
			}
			log.Printf("Kalshi initialized (%s)", mode)
		}
	}

	// Fetch initial balance if Kalshi client is available
	if kalshiClient != nil {
		balance, err := kalshiClient.GetBalanceDollars()
		if err != nil {
			log.Printf("Kalshi balance error: %v", err)
		} else {
			log.Printf("Kalshi balance: $%.2f", balance)
		}
	}

	// Execution config
	execConfig := kalshi.OrderConfig{
		MaxSlippagePct:        cfg.MaxSlippagePct,
		MinLiquidityContracts: cfg.MinLiquidityContracts,
		DryRun:                !cfg.AutoExecute, // Dry run if not auto-executing
	}

	// Initialize database
	db, err := positions.NewDB(cfg.DBPath)
	if err != nil {
		log.Printf("DB disabled: %v", err)
		db = nil
	} else {
		defer db.Close()
	}

	// Build execution mode string
	execMode := "alerts only"
	if cfg.AutoExecute && kalshiClient != nil {
		execMode = "AUTO EXECUTE"
		if cfg.KalshiDemo {
			execMode = "AUTO EXECUTE (DEMO)"
		}
	} else if kalshiClient != nil {
		execMode = "dry run (balance/liquidity checks enabled)"
	}

	// Log startup
	notifier.LogStartup(fmt.Sprintf(" ev=%.1f%% fee=%.2f%% kelly=%.0f%% poll=%s db=%s mode=%s slippage=%.1f%% minLiq=%d maxBet=%s",
		cfg.EVThreshold*100, cfg.KalshiFee*100, cfg.KellyFraction*100, cfg.PollInterval, cfg.DBPath,
		execMode, cfg.MaxSlippagePct*100, cfg.MinLiquidityContracts, formatMaxBet(cfg.MaxBetDollars)))

	// Start health check server
	go startHealthServer(cfg.Port)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, stopping...")
		cancel()
	}()

	// Main polling loop
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(10 * time.Minute)
	defer cleanupTicker.Stop()

	log.Println("Starting polling loop...")

	for {
		select {
		case <-ctx.Done():
			log.Println("Bot stopped gracefully")
			return

		case <-cleanupTicker.C:
			notifier.CleanupOldAlerts()

		case <-ticker.C:
			scanForOpportunities(client, db, notifier, analysisConfig, kalshiClient, execConfig, cfg)
		}
	}
}

func formatMaxBet(maxBet float64) string {
	if maxBet <= 0 {
		return "no cap"
	}
	return fmt.Sprintf("$%.2f", maxBet)
}

func scanForOpportunities(
	client *api.BallDontLieClient,
	db *positions.DB,
	notifier *alerts.Notifier,
	cfg analysis.Config,
	kalshiClient *kalshi.KalshiClient,
	execConfig kalshi.OrderConfig,
	mainCfg Config,
) {
	gameOdds, err := client.GetTodaysOdds()
	if err != nil {
		notifier.LogError("fetching odds", err)
		return
	}

	if len(gameOdds) == 0 {
		return
	}

	var allPositions []positions.Position
	if db != nil {
		allPositions, _ = db.GetAllPositions()
	}

	// Fetch current balance if Kalshi client available
	var bankroll float64
	var kalshiAvailable bool
	if kalshiClient != nil {
		// Check for maintenance window first (Thursday 3-5 AM ET)
		if kalshi.IsMaintenanceWindowNow() {
			if time.Since(lastMaintenanceLog) > 10*time.Minute {
				log.Println("Kalshi maintenance window (Thu 3-5am ET) - skipping execution")
				lastMaintenanceLog = time.Now()
			}
		} else {
			bankroll, err = kalshiClient.GetBalanceDollars()
			if err != nil {
				notifier.LogError("fetching Kalshi balance", err)
				// Continue without execution capability
			} else {
				kalshiAvailable = true
			}
		}
	}

	// Collect all opportunities first, then sort by EV and execute
	var allGameOpps []analysis.Opportunity
	var allPropOpps []analysis.PlayerPropOpportunity

	// Fetch Kalshi player props once for all games (more efficient)
	// Use ET since NBA schedule and Kalshi tickers use Eastern Time dates
	var kalshiPlayerProps map[string][]kalshi.PlayerPropMarket
	if kalshiClient != nil {
		et, err := time.LoadLocation("America/New_York")
		if err != nil {
			et = time.FixedZone("ET", -5*60*60)
		}
		nowET := time.Now().In(et)
		kalshiPlayerProps, err = kalshiClient.GetPlayerPropMarkets(nowET)
		if err != nil {
			notifier.LogError("fetching Kalshi player props", err)
		}
	}

	for _, game := range gameOdds {
		// Skip games that have started or finished
		// Status values: "Final", "1st Qtr", "2nd Qtr", "Halftime", "3rd Qtr", "4th Qtr", "OT"
		// For scheduled games, status is typically a datetime (e.g., "2026-02-05T03:00:00Z")
		status := game.Game.Status
		if status == "Final" || strings.Contains(status, "Qtr") || status == "Halftime" || status == "OT" {
			continue
		}

		// Skip games starting within 1 minute (pre-game odds become unreliable)
		if game.Game.StartsWithin(1 * time.Minute) {
			continue
		}

		// Calculate consensus for game markets
		consensus := odds.CalculateConsensus(game)

		// Find +EV game opportunities (moneyline, spread, total)
		opportunities := analysis.FindAllOpportunities(consensus, cfg)
		allGameOpps = append(allGameOpps, opportunities...)

		// Fetch and analyze player props for this game
		playerProps, err := client.GetPlayerProps(game.GameID)
		if err == nil && len(playerProps) > 0 {
			// Use the new function that matches BDL props with Kalshi props
			if len(kalshiPlayerProps) > 0 {
				// Collect unique player IDs and fetch their names
				playerIDSet := make(map[int]bool)
				for _, prop := range playerProps {
					playerIDSet[prop.PlayerID] = true
				}
				playerIDs := make([]int, 0, len(playerIDSet))
				for id := range playerIDSet {
					playerIDs = append(playerIDs, id)
				}
				playerNames := client.GetPlayerNames(playerIDs)

				// Use interpolation to compare any BDL line with any Kalshi line
				propOpportunities := analysis.FindPlayerPropOpportunitiesWithInterpolation(
					playerProps,
					kalshiPlayerProps,
					playerNames,
					game.Game.Date,
					game.Game.HomeTeam.Abbreviation,
					game.Game.VisitorTeam.Abbreviation,
					game.GameID,
					cfg,
				)
				allPropOpps = append(allPropOpps, propOpportunities...)
			}
		}

		// Check for hedge opportunities on existing positions
		if db != nil && len(allPositions) > 0 {
			hedges := positions.FindHedgeOpportunities(allPositions, consensus, cfg.KalshiFee)
			for _, hedge := range hedges {
				notifier.AlertHedge(hedge)
			}
		}
	}

	// Sort game opportunities by AdjustedEV (highest first)
	sort.Slice(allGameOpps, func(i, j int) bool {
		return allGameOpps[i].AdjustedEV > allGameOpps[j].AdjustedEV
	})

	// Sort player prop opportunities by AdjustedEV (highest first)
	sort.Slice(allPropOpps, func(i, j int) bool {
		return allPropOpps[i].AdjustedEV > allPropOpps[j].AdjustedEV
	})

	// Execute game opportunities (sorted by EV, updating bankroll after each)
	for _, opp := range allGameOpps {
		notifier.AlertOpportunity(opp)
		if kalshiAvailable && bankroll > 0 {
			spent := executeOpportunity(kalshiClient, opp, bankroll, execConfig, mainCfg, notifier, db)
			bankroll -= spent // Update bankroll for next bet's Kelly calculation
		}
	}

	// Execute player prop opportunities (sorted by EV, updating bankroll after each)
	for _, propOpp := range allPropOpps {
		notifier.AlertPlayerProp(propOpp)
		if kalshiAvailable && bankroll > 0 {
			spent := executePlayerPropOpportunity(kalshiClient, propOpp, bankroll, execConfig, mainCfg, notifier, db)
			bankroll -= spent // Update bankroll for next bet's Kelly calculation
		}
	}

	notifier.LogScanWithProps(len(gameOdds), len(allGameOpps), len(allPropOpps))
}

// executeOpportunity attempts to execute a +EV opportunity via Kalshi
// Returns the dollar amount spent (for bankroll tracking)
func executeOpportunity(
	kalshiClient *kalshi.KalshiClient,
	opp analysis.Opportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg Config,
	notifier *alerts.Notifier,
	db *positions.DB,
) float64 {
	// Convert opportunity to Kalshi market ticker
	ticker := mapToKalshiTicker(opp)
	if ticker == "" {
		log.Printf("ERROR no ticker for game %d", opp.GameID)
		return 0
	}

	// Determine side
	side := kalshi.SideYes
	if opp.Side == "away" || opp.Side == "under" {
		side = kalshi.SideNo
	}

	// Check database for existing position (persists across restarts)
	betSide := string(side)
	if db != nil {
		hasPosition, err := db.HasPositionOnTicker(ticker, betSide)
		if err != nil {
			log.Printf("ERROR checking DB for %s: %v", ticker, err)
		} else if hasPosition {
			return 0
		}
	}

	// Check for existing position on Kalshi - prevent duplicate bets unless arb
	arbConfig := kalshi.ArbConfig{
		KalshiFee:      cfg.KalshiFee,
		MinProfitCents: 0.5,
		MinProfitPct:   0.005,
	}

	canAdd, isArb, arbOpp, err := kalshiClient.CheckCanAddToPosition(ticker, side, arbConfig)
	if err != nil {
		log.Printf("ERROR checking position for %s: %v", ticker, err)
		return 0
	}

	if !canAdd {
		return 0
	}

	// If this is an arb opportunity, execute the arb instead of the +EV trade
	var spent float64
	if isArb && arbOpp != nil {
		log.Printf("ARB DETECTED: %s | %s", ticker, arbOpp.Description)
		spent = executeArbitrage(kalshiClient, arbOpp, bankroll, execConfig, cfg, notifier, db, opp, ticker, betSide)
	} else {
		// Normal +EV execution flow
		spent = executeNormalTrade(kalshiClient, ticker, side, opp, bankroll, execConfig, cfg, notifier, db, betSide)
	}

	return spent
}

// executeArbitrage executes an arbitrage opportunity
// Returns the dollar amount spent
func executeArbitrage(
	kalshiClient *kalshi.KalshiClient,
	arb *kalshi.ArbOpportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg Config,
	notifier *alerts.Notifier,
	db *positions.DB,
	opp analysis.Opportunity,
	ticker string,
	betSide string,
) float64 {
	// Calculate max contracts based on bankroll
	// Cost per arb = YES price + NO price
	costPerContractCents := arb.TotalCost
	maxAffordable := int(bankroll * 100 / float64(costPerContractCents))

	contracts := min(arb.MaxContracts, maxAffordable)
	if contracts < execConfig.MinLiquidityContracts {
		log.Printf("Arb size (%d contracts) below minimum, skipping", contracts)
		return 0
	}

	// Apply max bet limit
	if cfg.MaxBetDollars > 0 {
		maxFromLimit := int(cfg.MaxBetDollars * 100 / float64(costPerContractCents))
		contracts = min(contracts, maxFromLimit)
	}

	// Store arb position in database BEFORE placing order (for duplicate prevention)
	if db != nil {
		pos := positions.Position{
			GameID:     fmt.Sprintf("%d", opp.GameID),
			HomeTeam:   opp.HomeTeam,
			AwayTeam:   opp.AwayTeam,
			MarketType: "arb_" + string(opp.MarketType),
			Side:       "arb",
			Ticker:     ticker,
			BetSide:    betSide,
			EntryPrice: float64(arb.TotalCost) / 100,
			Contracts:  contracts,
		}
		id, err := db.AddPosition(pos)
		if err != nil {
			log.Printf("ERROR storing arb position: %v", err)
		} else {
			log.Printf("Stored arb pos #%d: %s %s", id, ticker, betSide)
		}
	}

	if execConfig.DryRun {
		profit := kalshi.CalculateArbProfit(arb.YesPrice, arb.NoPrice, contracts, cfg.KalshiFee)
		log.Printf("DRY RUN ARB: %d contracts YES@%d¢+NO@%d¢ profit=$%.2f",
			contracts, arb.YesPrice, arb.NoPrice, profit/100)
		return 0
	}

	log.Printf("EXEC ARB: %s %d contracts YES@%d¢+NO@%d¢", arb.Ticker, contracts, arb.YesPrice, arb.NoPrice)

	yesResult, noResult, err := kalshiClient.ExecuteArb(arb, contracts, execConfig)
	if err != nil {
		log.Printf("ERROR arb exec: %v", err)
		return 0
	}

	// Calculate actual profit and total cost
	totalSpent := 0.0
	if yesResult != nil && yesResult.FilledContracts > 0 {
		totalSpent += float64(yesResult.TotalCost) / 100
	}
	if noResult != nil && noResult.FilledContracts > 0 {
		totalSpent += float64(noResult.TotalCost) / 100
	}

	if yesResult != nil && noResult != nil && yesResult.FilledContracts > 0 && noResult.FilledContracts > 0 {
		matchedContracts := min(yesResult.FilledContracts, noResult.FilledContracts)
		profit := kalshi.CalculateArbProfit(
			int(yesResult.AveragePrice),
			int(noResult.AveragePrice),
			matchedContracts,
			cfg.KalshiFee,
		)
		log.Printf("ARB FILLED: %d matched, YES@%.0f¢ NO@%.0f¢, profit=$%.2f",
			matchedContracts, yesResult.AveragePrice, noResult.AveragePrice, profit/100)
	}

	return totalSpent
}

// executeNormalTrade executes a standard +EV trade
// Returns the dollar amount spent
func executeNormalTrade(
	kalshiClient *kalshi.KalshiClient,
	ticker string,
	side kalshi.Side,
	opp analysis.Opportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg Config,
	notifier *alerts.Notifier,
	db *positions.DB,
	betSide string,
) float64 {
	// Calculate bet size using real bankroll
	priceInCents := int(opp.KalshiPrice * 100)
	contracts := analysis.CalculateKellyContracts(
		opp.TrueProb,
		opp.KalshiPrice,
		cfg.KellyFraction,
		bankroll,
		cfg.MaxBetDollars,
		priceInCents,
	)

	if contracts < execConfig.MinLiquidityContracts {
		return 0
	}

	// Fetch order book and check liquidity/slippage
	book, err := kalshiClient.GetOrderBook(ticker)
	if err != nil {
		log.Printf("ERROR orderbook %s: %v", ticker, err)
		return 0
	}

	// Calculate slippage
	slippage := kalshiClient.CalculateSlippage(book, side, kalshi.ActionBuy, contracts)
	if !slippage.Acceptable {
		return 0
	}

	// Recalculate EV with actual fill price
	actualFillPrice := slippage.AverageFillPrice / 100.0
	_, adjustedEV := analysis.RecalculateEVWithSlippage(opp.TrueProb, actualFillPrice, cfg.KalshiFee)

	if adjustedEV < cfg.EVThreshold {
		log.Printf("EV degraded %.2f%%->%.2f%% after slippage, skip %s", opp.AdjustedEV*100, adjustedEV*100, ticker)
		return 0
	}

	log.Printf("EXEC: %s %s %d@%.0f¢ ev=%.2f%% kelly=%.1f%%",
		ticker, side, contracts, slippage.AverageFillPrice, adjustedEV*100, opp.KellyStake*100)

	// Store position in database BEFORE placing order (for duplicate prevention in dry-run and live)
	if db != nil {
		pos := positions.Position{
			GameID:     fmt.Sprintf("%d", opp.GameID),
			HomeTeam:   opp.HomeTeam,
			AwayTeam:   opp.AwayTeam,
			MarketType: string(opp.MarketType),
			Side:       opp.Side,
			Ticker:     ticker,
			BetSide:    betSide,
			EntryPrice: slippage.AverageFillPrice / 100,
			Contracts:  contracts,
		}
		id, err := db.AddPosition(pos)
		if err != nil {
			log.Printf("ERROR storing position: %v", err)
		} else {
			log.Printf("Stored pos #%d: %s %s", id, ticker, betSide)
		}
	}

	// Add EV verification to execution config (safety net for price movement during execution)
	execConfigWithEV := execConfig
	execConfigWithEV.TrueProb = opp.TrueProb
	execConfigWithEV.EVThreshold = cfg.EVThreshold
	execConfigWithEV.FeePct = cfg.KalshiFee

	result, err := kalshiClient.PlaceOrder(ticker, side, kalshi.ActionBuy, contracts, execConfigWithEV)
	if err != nil {
		log.Printf("ERROR order %s: %v", ticker, err)
		return 0
	}

	if result.Success {
		log.Printf("FILLED: %s %d/%d@%.0f¢ cost=$%.2f",
			result.OrderID, result.FilledContracts, result.RequestedContracts,
			result.AveragePrice, float64(result.TotalCost)/100)
		return float64(result.TotalCost) / 100
	}

	log.Printf("REJECTED: %s | %s", ticker, result.RejectionReason)
	return 0
}

// executePlayerPropOpportunity attempts to execute a +EV player prop opportunity via Kalshi
// Returns the dollar amount spent (for bankroll tracking)
func executePlayerPropOpportunity(
	kalshiClient *kalshi.KalshiClient,
	opp analysis.PlayerPropOpportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg Config,
	notifier *alerts.Notifier,
	db *positions.DB,
) float64 {
	// Use the full Kalshi ticker from the opportunity
	ticker := opp.KalshiTicker
	if ticker == "" {
		log.Printf("ERROR no ticker for prop: %s %s", opp.PlayerName, opp.PropType)
		return 0
	}

	// Determine side (over = YES, under = NO)
	side := kalshi.SideYes
	if opp.Side == "under" {
		side = kalshi.SideNo
	}
	betSide := string(side)

	// Check database for existing position (persists across restarts)
	if db != nil {
		hasPosition, err := db.HasPositionOnTicker(ticker, betSide)
		if err != nil {
			log.Printf("ERROR checking DB for prop %s: %v", ticker, err)
		} else if hasPosition {
			return 0
		}
	}

	// Check for existing position on Kalshi - prevent duplicate bets
	arbConfig := kalshi.ArbConfig{
		KalshiFee:      cfg.KalshiFee,
		MinProfitCents: 0.5,
		MinProfitPct:   0.005,
	}

	canAdd, _, _, err := kalshiClient.CheckCanAddToPosition(ticker, side, arbConfig)
	if err != nil {
		log.Printf("ERROR checking position for prop %s: %v", ticker, err)
		return 0
	}

	if !canAdd {
		return 0
	}

	// Calculate bet size using real bankroll
	priceInCents := int(opp.KalshiPrice * 100)
	contracts := analysis.CalculateKellyContracts(
		opp.TrueProb,
		opp.KalshiPrice,
		cfg.KellyFraction,
		bankroll,
		cfg.MaxBetDollars,
		priceInCents,
	)

	if contracts < execConfig.MinLiquidityContracts {
		return 0
	}

	// Fetch order book and check liquidity/slippage
	book, err := kalshiClient.GetOrderBook(ticker)
	if err != nil {
		log.Printf("ERROR orderbook prop %s: %v", ticker, err)
		return 0
	}

	// Calculate slippage
	slippage := kalshiClient.CalculateSlippage(book, side, kalshi.ActionBuy, contracts)
	if !slippage.Acceptable {
		return 0
	}

	// Recalculate EV with actual fill price
	actualFillPrice := slippage.AverageFillPrice / 100.0
	_, adjustedEV := analysis.RecalculateEVWithSlippage(opp.TrueProb, actualFillPrice, cfg.KalshiFee)

	if adjustedEV < cfg.EVThreshold {
		log.Printf("EV degraded %.2f%%->%.2f%% after slippage, skip prop %s", opp.AdjustedEV*100, adjustedEV*100, ticker)
		return 0
	}

	log.Printf("EXEC PROP: %s %s %.0f %s %d@%.0f¢ ev=%.2f%%",
		opp.PlayerName, opp.Side, opp.Line, opp.PropType, contracts, slippage.AverageFillPrice, adjustedEV*100)

	// Store position in database BEFORE placing order (for duplicate prevention in dry-run and live)
	if db != nil {
		pos := positions.Position{
			GameID:     fmt.Sprintf("%d", opp.GameID),
			HomeTeam:   opp.HomeTeam,
			AwayTeam:   opp.AwayTeam,
			MarketType: fmt.Sprintf("prop_%s", opp.PropType),
			Side:       fmt.Sprintf("%s_%s_%.1f", opp.PlayerName, opp.Side, opp.Line),
			Ticker:     ticker,
			BetSide:    betSide,
			EntryPrice: slippage.AverageFillPrice / 100,
			Contracts:  contracts,
		}
		id, err := db.AddPosition(pos)
		if err != nil {
			log.Printf("ERROR storing prop position: %v", err)
		} else {
			log.Printf("Stored prop pos #%d: %s %s", id, ticker, betSide)
		}
	}

	// Add EV verification to execution config (safety net for price movement during execution)
	execConfigWithEV := execConfig
	execConfigWithEV.TrueProb = opp.TrueProb
	execConfigWithEV.EVThreshold = cfg.EVThreshold
	execConfigWithEV.FeePct = cfg.KalshiFee

	result, err := kalshiClient.PlaceOrder(ticker, side, kalshi.ActionBuy, contracts, execConfigWithEV)
	if err != nil {
		log.Printf("ERROR prop order %s: %v", ticker, err)
		return 0
	}

	if result.Success {
		log.Printf("PROP FILLED: %s %d/%d@%.0f¢ cost=$%.2f",
			result.OrderID, result.FilledContracts, result.RequestedContracts,
			result.AveragePrice, float64(result.TotalCost)/100)
		return float64(result.TotalCost) / 100
	}

	log.Printf("PROP REJECTED: %s | %s", ticker, result.RejectionReason)
	return 0
}

// mapToKalshiTicker maps a BallDontLie game opportunity to a Kalshi market ticker
// Kalshi NBA tickers follow this format:
// - KXNBAGAME-26FEB04MEMSAC (moneyline: Memphis @ Sacramento on Feb 4, 2026)
// - KXNBASPREAD-26FEB04MEMSAC (spread: Memphis @ Sacramento on Feb 4, 2026)
// - KXNBATOTAL-26FEB04NOPMIL (total: New Orleans @ Milwaukee on Feb 4, 2026)
func mapToKalshiTicker(opp analysis.Opportunity) string {
	// Get series for market type
	series := kalshi.GetSeriesForMarketType(string(opp.MarketType))
	if series == "" {
		return ""
	}

	// Parse game date (expected format: "2006-01-02")
	gameDate, err := time.Parse("2006-01-02", opp.GameDate)
	if err != nil {
		log.Printf("ERROR bad game date %s: %v", opp.GameDate, err)
		return ""
	}

	ticker := kalshi.BuildNBATicker(series, gameDate, opp.AwayTeam, opp.HomeTeam)
	if ticker == "" {
		log.Printf("ERROR build ticker: %s@%s %s", opp.AwayTeam, opp.HomeTeam, opp.GameDate)
	}

	return ticker
}

// mapToPlayerPropTicker maps a player prop opportunity to a Kalshi ticker
func mapToPlayerPropTicker(opp analysis.PlayerPropOpportunity) string {
	propType := kalshi.PropTypeFromBallDontLie(opp.PropType)
	if propType == "" {
		return ""
	}

	gameDate, err := time.Parse("2006-01-02", opp.GameDate)
	if err != nil {
		log.Printf("ERROR bad game date %s: %v", opp.GameDate, err)
		return ""
	}

	ticker := kalshi.BuildPlayerPropTicker(propType, gameDate, opp.AwayTeam, opp.HomeTeam)
	if ticker == "" {
		log.Printf("ERROR build prop ticker: %s %s %s", opp.PlayerName, opp.PropType, opp.GameDate)
	}

	return ticker
}

func startHealthServer(port string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Sports Betting Bot - Running"))
	})

	addr := ":" + port
	log.Printf("Health server listening on %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("Health server error: %v", err)
	}
}
