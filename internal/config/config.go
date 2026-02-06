package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Defaults for configuration values.
const (
	DefaultEVThreshold            = 0.03
	DefaultKellyFraction          = 0.25
	DefaultPollInterval           = 2 * time.Second
	DefaultDBPath                 = "/data/positions.db"
	DefaultPort                   = "8080"
	DefaultMaxSlippagePct         = 0.02
	DefaultMinLiquidity           = 10
	DefaultAlertCooldown          = 5 * time.Minute
	DefaultMinBookCount           = 4
	DefaultCleanupInterval        = 10 * time.Minute
	DefaultMaintenanceLogCooldown = 10 * time.Minute
	DefaultPreGameSkipWindow      = 1 * time.Minute
	DefaultMinArbProfitCents      = 0.5
	DefaultMinArbProfitPct        = 0.005
)

// Config holds all application configuration.
type Config struct {
	APIKey        string
	EVThreshold   float64
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

// Load reads configuration from environment variables (and .env file if present).
func Load() Config {
	_ = godotenv.Load() // Ignore error if .env doesn't exist

	cfg := Config{
		APIKey:        os.Getenv("BALLDONTLIE_API_KEY"),
		EVThreshold:   DefaultEVThreshold,
		KellyFraction: DefaultKellyFraction,
		PollInterval:  DefaultPollInterval,
		DBPath:        DefaultDBPath,
		Port:          DefaultPort,

		// Kalshi API key auth (email/password deprecated by Kalshi)
		KalshiAPIKeyID:   os.Getenv("KALSHI_API_KEY_ID"),
		KalshiAPIKeyPath: os.Getenv("KALSHI_API_KEY_PATH"), // Local dev: file path
		KalshiPrivateKey: os.Getenv("KALSHI_PRIVATE_KEY"),  // Cloud: key content directly
		KalshiDemo:       os.Getenv("KALSHI_DEMO") == "true",

		// Execution defaults
		AutoExecute:           false,
		MaxSlippagePct:        DefaultMaxSlippagePct,
		MinLiquidityContracts: DefaultMinLiquidity,
		MaxBetDollars:         0, // 0 = no cap
	}

	if v := os.Getenv("EV_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.EVThreshold = f
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

// Validate checks that configuration values are within acceptable ranges.
func Validate(cfg Config) error {
	if cfg.EVThreshold < 0 || cfg.EVThreshold > 1 {
		return fmt.Errorf("EV_THRESHOLD must be between 0 and 1, got %f", cfg.EVThreshold)
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

// FormatMaxBet returns a human-readable string for the max bet setting.
func FormatMaxBet(maxBet float64) string {
	if maxBet <= 0 {
		return "no cap"
	}
	return fmt.Sprintf("$%.2f", maxBet)
}
