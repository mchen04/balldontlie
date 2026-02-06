package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"sports-betting-bot/internal/alerts"
	"sports-betting-bot/internal/analysis"
	"sports-betting-bot/internal/api"
	"sports-betting-bot/internal/config"
	"sports-betting-bot/internal/engine"
	"sports-betting-bot/internal/kalshi"
	"sports-betting-bot/internal/positions"
)

func main() {
	cfg := config.Load()

	if err := config.Validate(cfg); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	if cfg.APIKey == "" {
		log.Fatal("BALLDONTLIE_API_KEY is required")
	}

	// Initialize components
	client := api.NewBallDontLieClient(cfg.APIKey)
	notifier := alerts.NewNotifier(config.DefaultAlertCooldown)
	kalshiClient := initKalshi(cfg)
	db := initDB(cfg.DBPath)
	if db != nil {
		defer db.Close()
	}

	analysisCfg := analysis.Config{
		EVThreshold:   cfg.EVThreshold,
		KellyFraction: cfg.KellyFraction,
		MinBookCount:  config.DefaultMinBookCount,
	}

	execConfig := kalshi.OrderConfig{
		MaxSlippagePct:        cfg.MaxSlippagePct,
		MinLiquidityContracts: cfg.MinLiquidityContracts,
		DryRun:                !cfg.AutoExecute,
	}

	// Log startup
	execMode := buildExecModeString(cfg, kalshiClient)
	notifier.LogStartup(fmt.Sprintf(" ev=%.1f%% fee=kalshi_formula kelly=%.0f%% poll=%s db=%s mode=%s slippage=%.1f%% minLiq=%d maxBet=%s",
		cfg.EVThreshold*100, cfg.KellyFraction*100, cfg.PollInterval, cfg.DBPath,
		execMode, cfg.MaxSlippagePct*100, cfg.MinLiquidityContracts, config.FormatMaxBet(cfg.MaxBetDollars)))

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

	// Create engine and run
	eng := engine.New(client, kalshiClient, notifier, db, cfg, analysisCfg, execConfig)
	eng.Run(ctx)
}

func initKalshi(cfg config.Config) *kalshi.KalshiClient {
	if cfg.KalshiAPIKeyID == "" || (cfg.KalshiPrivateKey == "" && cfg.KalshiAPIKeyPath == "") {
		return nil
	}

	var client *kalshi.KalshiClient
	var err error
	if cfg.KalshiPrivateKey != "" {
		client, err = kalshi.NewKalshiClientFromKey(cfg.KalshiAPIKeyID, cfg.KalshiPrivateKey, cfg.KalshiDemo)
	} else {
		client, err = kalshi.NewKalshiClient(cfg.KalshiAPIKeyID, cfg.KalshiAPIKeyPath, cfg.KalshiDemo)
	}
	if err != nil {
		log.Printf("Kalshi disabled: %v", err)
		return nil
	}

	mode := "production"
	if cfg.KalshiDemo {
		mode = "demo"
	}
	log.Printf("Kalshi initialized (%s)", mode)

	balance, err := client.GetBalanceDollars()
	if err != nil {
		log.Printf("Kalshi balance error: %v", err)
	} else {
		log.Printf("Kalshi balance: $%.2f", balance)
	}

	return client
}

func initDB(dbPath string) *positions.DB {
	db, err := positions.NewDB(dbPath)
	if err != nil {
		log.Printf("DB disabled: %v", err)
		return nil
	}
	return db
}

func buildExecModeString(cfg config.Config, kalshiClient *kalshi.KalshiClient) string {
	if cfg.AutoExecute && kalshiClient != nil {
		if cfg.KalshiDemo {
			return "AUTO EXECUTE (DEMO)"
		}
		return "AUTO EXECUTE"
	}
	if kalshiClient != nil {
		return "dry run (balance/liquidity checks enabled)"
	}
	return "alerts only"
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
