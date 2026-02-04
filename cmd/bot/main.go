package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
			log.Printf("Warning: Could not initialize Kalshi client: %v", err)
			log.Println("Kalshi integration will be disabled")
		} else {
			mode := "production"
			if cfg.KalshiDemo {
				mode = "demo"
			}
			log.Printf("Kalshi client initialized (%s mode)", mode)
		}
	}

	// Fetch initial balance if Kalshi client is available
	if kalshiClient != nil {
		balance, err := kalshiClient.GetBalanceDollars()
		if err != nil {
			log.Printf("Warning: Could not fetch Kalshi balance: %v", err)
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
		log.Printf("Warning: Could not open positions database: %v", err)
		log.Println("Position tracking will be disabled")
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
	notifier.LogStartup(fmt.Sprintf(`
EV Threshold:      %.1f%%
Kalshi Fee:        %.2f%%
Kelly Fraction:    %.0f%%
Poll Interval:     %s
Database:          %s
Execution Mode:    %s
Max Slippage:      %.1f%%
Min Liquidity:     %d contracts
Max Bet:           %s
`, cfg.EVThreshold*100, cfg.KalshiFee*100, cfg.KellyFraction*100, cfg.PollInterval, cfg.DBPath,
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
			log.Println("Kalshi maintenance window (Thu 3-5am ET) - skipping execution")
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

	gameOpps := 0
	propOpps := 0

	// Fetch Kalshi player props once for all games (more efficient)
	var kalshiPlayerProps map[string][]kalshi.PlayerPropMarket
	if kalshiClient != nil {
		var err error
		kalshiPlayerProps, err = kalshiClient.GetPlayerPropMarkets(time.Now())
		if err != nil {
			notifier.LogError("fetching Kalshi player props", err)
		}
	}

	for _, game := range gameOdds {
		// Skip games that have started (status will be "Final" or similar for completed games)
		// For scheduled games, status is typically a datetime or empty
		if game.Game.Status == "Final" || game.Game.Status == "In Progress" {
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
		for _, opp := range opportunities {
			notifier.AlertOpportunity(opp)
			gameOpps++

			// Attempt execution if Kalshi available and we have balance
			if kalshiAvailable && bankroll > 0 {
				executeOpportunity(kalshiClient, opp, bankroll, execConfig, mainCfg, notifier, db)
			}
		}

		// Fetch and analyze player props for this game
		playerProps, err := client.GetPlayerProps(game.GameID)
		if err == nil && len(playerProps) > 0 {
			// Use the new function that matches BDL props with Kalshi props
			var propOpportunities []analysis.PlayerPropOpportunity
			if len(kalshiPlayerProps) > 0 {
				propOpportunities = analysis.FindPlayerPropOpportunitiesWithKalshi(
					playerProps,
					kalshiPlayerProps,
					game.Game.Date,
					game.Game.HomeTeam.Abbreviation,
					game.Game.VisitorTeam.Abbreviation,
					game.GameID,
					cfg,
				)
			}
			for _, propOpp := range propOpportunities {
				notifier.AlertPlayerProp(propOpp)
				propOpps++

				// Attempt execution if Kalshi available and we have balance
				if kalshiAvailable && bankroll > 0 {
					executePlayerPropOpportunity(kalshiClient, propOpp, bankroll, execConfig, mainCfg, notifier, db)
				}
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

	notifier.LogScanWithProps(len(gameOdds), gameOpps, propOpps)
}

// executeOpportunity attempts to execute a +EV opportunity via Kalshi
func executeOpportunity(
	kalshiClient *kalshi.KalshiClient,
	opp analysis.Opportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg Config,
	notifier *alerts.Notifier,
	db *positions.DB,
) {
	// Convert opportunity to Kalshi market ticker
	ticker := mapToKalshiTicker(opp)
	if ticker == "" {
		log.Printf("Could not map game %d to Kalshi ticker", opp.GameID)
		return
	}

	// Determine side
	side := kalshi.SideYes
	if opp.Side == "away" || opp.Side == "under" {
		side = kalshi.SideNo
	}

	// Check for existing position - prevent duplicate bets unless arb
	arbConfig := kalshi.ArbConfig{
		KalshiFee:      cfg.KalshiFee,
		MinProfitCents: 0.5,
		MinProfitPct:   0.005,
	}

	canAdd, isArb, arbOpp, err := kalshiClient.CheckCanAddToPosition(ticker, side, arbConfig)
	if err != nil {
		log.Printf("Failed to check existing position for %s: %v", ticker, err)
		return
	}

	if !canAdd {
		log.Printf("Skipping %s %s: already have position on this market (no arb available)", ticker, side)
		return
	}

	// If this is an arb opportunity, execute the arb instead of the +EV trade
	if isArb && arbOpp != nil {
		log.Printf("ARB DETECTED on %s: %s", ticker, arbOpp.Description)
		executeArbitrage(kalshiClient, arbOpp, bankroll, execConfig, cfg, notifier, db, opp)
		return
	}

	// Normal +EV execution flow
	executeNormalTrade(kalshiClient, ticker, side, opp, bankroll, execConfig, cfg, notifier, db)
}

// executeArbitrage executes an arbitrage opportunity
func executeArbitrage(
	kalshiClient *kalshi.KalshiClient,
	arb *kalshi.ArbOpportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg Config,
	notifier *alerts.Notifier,
	db *positions.DB,
	opp analysis.Opportunity,
) {
	// Calculate max contracts based on bankroll
	// Cost per arb = YES price + NO price
	costPerContractCents := arb.TotalCost
	maxAffordable := int(bankroll * 100 / float64(costPerContractCents))

	contracts := min(arb.MaxContracts, maxAffordable)
	if contracts < execConfig.MinLiquidityContracts {
		log.Printf("Arb size (%d contracts) below minimum, skipping", contracts)
		return
	}

	// Apply max bet limit
	if cfg.MaxBetDollars > 0 {
		maxFromLimit := int(cfg.MaxBetDollars * 100 / float64(costPerContractCents))
		contracts = min(contracts, maxFromLimit)
	}

	if execConfig.DryRun {
		profit := kalshi.CalculateArbProfit(arb.YesPrice, arb.NoPrice, contracts, cfg.KalshiFee)
		log.Printf("DRY RUN ARB: Would buy %d contracts YES@%d¢ + NO@%d¢. Guaranteed profit: $%.2f",
			contracts, arb.YesPrice, arb.NoPrice, profit/100)
		return
	}

	log.Printf("Executing ARB: %s %d contracts (YES@%d¢ + NO@%d¢)", arb.Ticker, contracts, arb.YesPrice, arb.NoPrice)

	yesResult, noResult, err := kalshiClient.ExecuteArb(arb, contracts, execConfig)
	if err != nil {
		log.Printf("Arb execution error: %v", err)
		return
	}

	if yesResult != nil && yesResult.Success {
		log.Printf("ARB YES LEG: filled %d @ %.0f¢", yesResult.FilledContracts, yesResult.AveragePrice)
	}
	if noResult != nil && noResult.Success {
		log.Printf("ARB NO LEG: filled %d @ %.0f¢", noResult.FilledContracts, noResult.AveragePrice)
	}

	// Calculate actual profit
	if yesResult != nil && noResult != nil && yesResult.FilledContracts > 0 && noResult.FilledContracts > 0 {
		matchedContracts := min(yesResult.FilledContracts, noResult.FilledContracts)
		profit := kalshi.CalculateArbProfit(
			int(yesResult.AveragePrice),
			int(noResult.AveragePrice),
			matchedContracts,
			cfg.KalshiFee,
		)
		log.Printf("ARB COMPLETE: %d matched contracts. Guaranteed profit: $%.2f", matchedContracts, profit/100)
	}
}

// executeNormalTrade executes a standard +EV trade
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
) {
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
		log.Printf("Kelly bet size (%d contracts) below minimum, skipping", contracts)
		return
	}

	// Fetch order book and check liquidity/slippage
	book, err := kalshiClient.GetOrderBook(ticker)
	if err != nil {
		log.Printf("Failed to fetch order book for %s: %v", ticker, err)
		return
	}

	// Calculate slippage
	slippage := kalshiClient.CalculateSlippage(book, side, kalshi.ActionBuy, contracts)
	if !slippage.Acceptable {
		log.Printf("Slippage %.2f%% exceeds max %.2f%% for %s, skipping",
			slippage.SlippagePct*100, execConfig.MaxSlippagePct*100, ticker)
		return
	}

	// Recalculate EV with actual fill price
	actualFillPrice := slippage.AverageFillPrice / 100.0
	_, adjustedEV := analysis.RecalculateEVWithSlippage(opp.TrueProb, actualFillPrice, cfg.KalshiFee)

	if adjustedEV < cfg.EVThreshold {
		log.Printf("EV degraded from %.2f%% to %.2f%% due to slippage, skipping %s",
			opp.AdjustedEV*100, adjustedEV*100, ticker)
		return
	}

	// Log execution attempt
	log.Printf("Executing: %s %s %d contracts @ %.0f¢ (EV: %.2f%%, Kelly: %.1f%%)",
		ticker, side, contracts, slippage.AverageFillPrice, adjustedEV*100, opp.KellyStake*100)

	// Add EV verification to execution config (safety net for price movement during execution)
	execConfigWithEV := execConfig
	execConfigWithEV.TrueProb = opp.TrueProb
	execConfigWithEV.EVThreshold = cfg.EVThreshold
	execConfigWithEV.FeePct = cfg.KalshiFee

	// Place the order
	result, err := kalshiClient.PlaceOrder(ticker, side, kalshi.ActionBuy, contracts, execConfigWithEV)
	if err != nil {
		log.Printf("Order placement error: %v", err)
		return
	}

	if result.Success {
		log.Printf("ORDER FILLED: %s %d/%d contracts @ %.0f¢ avg, total cost $%.2f",
			result.OrderID, result.FilledContracts, result.RequestedContracts,
			result.AveragePrice, float64(result.TotalCost)/100)

		// Track position in database
		if db != nil && result.FilledContracts > 0 {
			pos := positions.Position{
				GameID:     fmt.Sprintf("%d", opp.GameID),
				HomeTeam:   opp.HomeTeam,
				AwayTeam:   opp.AwayTeam,
				MarketType: string(opp.MarketType),
				Side:       opp.Side,
				EntryPrice: result.AveragePrice / 100,
				Contracts:  result.FilledContracts,
			}
			if _, err := db.AddPosition(pos); err != nil {
				log.Printf("Failed to track position: %v", err)
			}
		}
	} else {
		log.Printf("Order rejected: %s", result.RejectionReason)
	}
}

// executePlayerPropOpportunity attempts to execute a +EV player prop opportunity via Kalshi
func executePlayerPropOpportunity(
	kalshiClient *kalshi.KalshiClient,
	opp analysis.PlayerPropOpportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg Config,
	notifier *alerts.Notifier,
	db *positions.DB,
) {
	// Build player prop ticker
	ticker := mapToPlayerPropTicker(opp)
	if ticker == "" {
		log.Printf("Could not map player prop to Kalshi ticker: %s %s", opp.PlayerName, opp.PropType)
		return
	}

	// Determine side (over = YES, under = NO)
	side := kalshi.SideYes
	if opp.Side == "under" {
		side = kalshi.SideNo
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
		log.Printf("Kelly bet size (%d contracts) below minimum for prop, skipping", contracts)
		return
	}

	// Fetch order book and check liquidity/slippage
	book, err := kalshiClient.GetOrderBook(ticker)
	if err != nil {
		log.Printf("Failed to fetch order book for prop %s: %v", ticker, err)
		return
	}

	// Calculate slippage
	slippage := kalshiClient.CalculateSlippage(book, side, kalshi.ActionBuy, contracts)
	if !slippage.Acceptable {
		log.Printf("Slippage %.2f%% exceeds max for prop %s, skipping",
			slippage.SlippagePct*100, ticker)
		return
	}

	// Recalculate EV with actual fill price
	actualFillPrice := slippage.AverageFillPrice / 100.0
	_, adjustedEV := analysis.RecalculateEVWithSlippage(opp.TrueProb, actualFillPrice, cfg.KalshiFee)

	if adjustedEV < cfg.EVThreshold {
		log.Printf("Prop EV degraded from %.2f%% to %.2f%% due to slippage, skipping %s",
			opp.AdjustedEV*100, adjustedEV*100, ticker)
		return
	}

	// Log execution attempt
	log.Printf("Executing prop: %s %s %s %.1f %d contracts @ %.0f¢ (EV: %.2f%%)",
		opp.PlayerName, opp.Side, opp.PropType, opp.Line, contracts, slippage.AverageFillPrice, adjustedEV*100)

	// Add EV verification to execution config (safety net for price movement during execution)
	execConfigWithEV := execConfig
	execConfigWithEV.TrueProb = opp.TrueProb
	execConfigWithEV.EVThreshold = cfg.EVThreshold
	execConfigWithEV.FeePct = cfg.KalshiFee

	// Place the order
	result, err := kalshiClient.PlaceOrder(ticker, side, kalshi.ActionBuy, contracts, execConfigWithEV)
	if err != nil {
		log.Printf("Prop order placement error: %v", err)
		return
	}

	if result.Success {
		log.Printf("PROP ORDER FILLED: %s %d/%d contracts @ %.0f¢ avg, total cost $%.2f",
			result.OrderID, result.FilledContracts, result.RequestedContracts,
			result.AveragePrice, float64(result.TotalCost)/100)

		// Track position in database
		if db != nil && result.FilledContracts > 0 {
			pos := positions.Position{
				GameID:     fmt.Sprintf("%d", opp.GameID),
				HomeTeam:   opp.HomeTeam,
				AwayTeam:   opp.AwayTeam,
				MarketType: fmt.Sprintf("prop_%s", opp.PropType),
				Side:       fmt.Sprintf("%s_%s_%.1f", opp.PlayerName, opp.Side, opp.Line),
				EntryPrice: result.AveragePrice / 100,
				Contracts:  result.FilledContracts,
			}
			if _, err := db.AddPosition(pos); err != nil {
				log.Printf("Failed to track prop position: %v", err)
			}
		}
	} else {
		log.Printf("Prop order rejected: %s", result.RejectionReason)
	}
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
		log.Printf("Failed to parse game date %s: %v", opp.GameDate, err)
		return ""
	}

	// Build ticker using the kalshi package
	ticker := kalshi.BuildNBATicker(series, gameDate, opp.AwayTeam, opp.HomeTeam)
	if ticker == "" {
		log.Printf("Could not build ticker for %s @ %s on %s", opp.AwayTeam, opp.HomeTeam, opp.GameDate)
	}

	return ticker
}

// mapToPlayerPropTicker maps a player prop opportunity to a Kalshi ticker
// Player prop tickers follow this format:
// - KXNBAPTS-26FEB03PHIGSW (points props for Philadelphia @ Golden State)
// - KXNBAREB-26FEB03LALBKN (rebounds props for Lakers @ Nets)
func mapToPlayerPropTicker(opp analysis.PlayerPropOpportunity) string {
	// Convert prop type to Kalshi format
	propType := kalshi.PropTypeFromBallDontLie(opp.PropType)
	if propType == "" {
		return ""
	}

	// Parse game date (expected format: "2006-01-02")
	gameDate, err := time.Parse("2006-01-02", opp.GameDate)
	if err != nil {
		log.Printf("Failed to parse game date %s: %v", opp.GameDate, err)
		return ""
	}

	// Build ticker using the kalshi package
	ticker := kalshi.BuildPlayerPropTicker(propType, gameDate, opp.AwayTeam, opp.HomeTeam)
	if ticker == "" {
		log.Printf("Could not build prop ticker for %s %s on %s", opp.PlayerName, opp.PropType, opp.GameDate)
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
