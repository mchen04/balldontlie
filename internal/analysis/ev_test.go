package analysis

import (
	"math"
	"testing"
)

func TestCalculateEV(t *testing.T) {
	tests := []struct {
		name        string
		trueProb    float64
		kalshiPrice float64
		expectedEV  float64
		delta       float64
	}{
		{
			name:        "Positive EV - underpriced",
			trueProb:    0.55,       // 55% true probability
			kalshiPrice: 0.50,       // Kalshi pricing at 50%
			expectedEV:  0.05,       // 5% edge
			delta:       0.001,
		},
		{
			name:        "Zero EV - fair price",
			trueProb:    0.50,
			kalshiPrice: 0.50,
			expectedEV:  0.0,
			delta:       0.001,
		},
		{
			name:        "Negative EV - overpriced",
			trueProb:    0.45,
			kalshiPrice: 0.50,
			expectedEV:  -0.05,
			delta:       0.001,
		},
		{
			name:        "Big edge on underdog",
			trueProb:    0.35,       // 35% true
			kalshiPrice: 0.25,       // Kalshi at 25 cents
			expectedEV:  0.10,       // (0.35 * 0.75) - (0.65 * 0.25) = 0.2625 - 0.1625 = 0.10
			delta:       0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateEV(tt.trueProb, tt.kalshiPrice)
			if math.Abs(result-tt.expectedEV) > tt.delta {
				t.Errorf("CalculateEV(%v, %v) = %v, want %v",
					tt.trueProb, tt.kalshiPrice, result, tt.expectedEV)
			}
		})
	}
}

func TestCalculateAdjustedEV(t *testing.T) {
	tests := []struct {
		name       string
		trueProb   float64
		price      float64
		fee        float64
		expectedEV float64
		delta      float64
	}{
		{
			name:       "EV after 1.2% fee",
			trueProb:   0.55,
			price:      0.50,
			fee:        0.012, // 1.2%
			expectedEV: 0.05 - (0.50 * 0.012), // 5% - 0.6% = 4.4%
			delta:      0.001,
		},
		{
			name:       "Marginal edge wiped by fees",
			trueProb:   0.51,
			price:      0.50,
			fee:        0.012,
			expectedEV: 0.01 - (0.50 * 0.012), // 1% - 0.6% = 0.4%
			delta:       0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateAdjustedEV(tt.trueProb, tt.price, tt.fee)
			if math.Abs(result-tt.expectedEV) > tt.delta {
				t.Errorf("CalculateAdjustedEV = %v, want %v", result, tt.expectedEV)
			}
		})
	}
}

func TestCalculateEVEdgeCases(t *testing.T) {
	// Invalid price
	if ev := CalculateEV(0.5, 0); ev != 0 {
		t.Errorf("CalculateEV with price 0 should return 0, got %v", ev)
	}

	if ev := CalculateEV(0.5, 1); ev != 0 {
		t.Errorf("CalculateEV with price 1 should return 0, got %v", ev)
	}

	if ev := CalculateEV(0.5, -0.5); ev != 0 {
		t.Errorf("CalculateEV with negative price should return 0, got %v", ev)
	}
}

func TestEVThresholdLogic(t *testing.T) {
	cfg := DefaultConfig()

	// Test that default config has sensible values
	if cfg.EVThreshold != 0.03 {
		t.Errorf("Default EV threshold should be 3%%, got %v", cfg.EVThreshold)
	}
	if cfg.KalshiFee != 0.012 {
		t.Errorf("Default Kalshi fee should be 1.2%%, got %v", cfg.KalshiFee)
	}
	if cfg.KellyFraction != 0.25 {
		t.Errorf("Default Kelly fraction should be 25%%, got %v", cfg.KellyFraction)
	}
}
