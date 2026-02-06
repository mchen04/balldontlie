package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// Clear env vars that could affect defaults
	for _, key := range []string{
		"BALLDONTLIE_API_KEY", "EV_THRESHOLD", "KELLY_FRACTION",
		"POLL_INTERVAL_MS", "DB_PATH", "PORT", "AUTO_EXECUTE",
		"MAX_SLIPPAGE_PCT", "MIN_LIQUIDITY_CONTRACTS", "MAX_BET_DOLLARS",
		"KALSHI_API_KEY_ID", "KALSHI_API_KEY_PATH", "KALSHI_PRIVATE_KEY", "KALSHI_DEMO",
	} {
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.EVThreshold != DefaultEVThreshold {
		t.Errorf("EVThreshold = %f, want %f", cfg.EVThreshold, DefaultEVThreshold)
	}
	if cfg.KellyFraction != DefaultKellyFraction {
		t.Errorf("KellyFraction = %f, want %f", cfg.KellyFraction, DefaultKellyFraction)
	}
	if cfg.PollInterval != DefaultPollInterval {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, DefaultPollInterval)
	}
	if cfg.DBPath != DefaultDBPath {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, DefaultDBPath)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("Port = %q, want %q", cfg.Port, DefaultPort)
	}
	if cfg.MaxSlippagePct != DefaultMaxSlippagePct {
		t.Errorf("MaxSlippagePct = %f, want %f", cfg.MaxSlippagePct, DefaultMaxSlippagePct)
	}
	if cfg.MinLiquidityContracts != DefaultMinLiquidity {
		t.Errorf("MinLiquidityContracts = %d, want %d", cfg.MinLiquidityContracts, DefaultMinLiquidity)
	}
	if cfg.AutoExecute {
		t.Error("AutoExecute should default to false")
	}
	if cfg.MaxBetDollars != 0 {
		t.Errorf("MaxBetDollars = %f, want 0", cfg.MaxBetDollars)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("EV_THRESHOLD", "0.05")
	os.Setenv("KELLY_FRACTION", "0.5")
	os.Setenv("POLL_INTERVAL_MS", "500")
	os.Setenv("MAX_BET_DOLLARS", "100")
	os.Setenv("AUTO_EXECUTE", "true")
	defer func() {
		os.Unsetenv("EV_THRESHOLD")
		os.Unsetenv("KELLY_FRACTION")
		os.Unsetenv("POLL_INTERVAL_MS")
		os.Unsetenv("MAX_BET_DOLLARS")
		os.Unsetenv("AUTO_EXECUTE")
	}()

	cfg := Load()

	if cfg.EVThreshold != 0.05 {
		t.Errorf("EVThreshold = %f, want 0.05", cfg.EVThreshold)
	}
	if cfg.KellyFraction != 0.5 {
		t.Errorf("KellyFraction = %f, want 0.5", cfg.KellyFraction)
	}
	if cfg.PollInterval != 500*time.Millisecond {
		t.Errorf("PollInterval = %v, want 500ms", cfg.PollInterval)
	}
	if cfg.MaxBetDollars != 100 {
		t.Errorf("MaxBetDollars = %f, want 100", cfg.MaxBetDollars)
	}
	if !cfg.AutoExecute {
		t.Error("AutoExecute should be true")
	}
}

func TestValidate(t *testing.T) {
	valid := Config{
		EVThreshold:           0.03,
		KellyFraction:         0.25,
		MaxSlippagePct:        0.02,
		MinLiquidityContracts: 10,
		MaxBetDollars:         0,
		PollInterval:          2 * time.Second,
	}

	if err := Validate(valid); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}

	tests := []struct {
		name   string
		modify func(*Config)
	}{
		{"negative EV", func(c *Config) { c.EVThreshold = -0.1 }},
		{"EV > 1", func(c *Config) { c.EVThreshold = 1.5 }},
		{"zero Kelly", func(c *Config) { c.KellyFraction = 0 }},
		{"Kelly > 1", func(c *Config) { c.KellyFraction = 1.5 }},
		{"negative slippage", func(c *Config) { c.MaxSlippagePct = -0.1 }},
		{"negative liquidity", func(c *Config) { c.MinLiquidityContracts = -1 }},
		{"negative max bet", func(c *Config) { c.MaxBetDollars = -10 }},
		{"poll too fast", func(c *Config) { c.PollInterval = time.Millisecond }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid
			tt.modify(&c)
			if err := Validate(c); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestFormatMaxBet(t *testing.T) {
	if got := FormatMaxBet(0); got != "no cap" {
		t.Errorf("FormatMaxBet(0) = %q, want %q", got, "no cap")
	}
	if got := FormatMaxBet(-1); got != "no cap" {
		t.Errorf("FormatMaxBet(-1) = %q, want %q", got, "no cap")
	}
	if got := FormatMaxBet(50); got != "$50.00" {
		t.Errorf("FormatMaxBet(50) = %q, want %q", got, "$50.00")
	}
}
