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
		expectedEV float64
		delta      float64
	}{
		{
			name:       "EV after Kalshi fee at 50 cents",
			trueProb:   0.55,
			price:      0.50,
			expectedEV: 0.05 - 0.0175, // 5% - 1.75% (max fee at 50c)
			delta:      0.001,
		},
		{
			name:       "Marginal edge wiped by fees",
			trueProb:   0.51,
			price:      0.50,
			expectedEV: 0.01 - 0.0175, // 1% - 1.75% = -0.75%
			delta:       0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateAdjustedEV(tt.trueProb, tt.price)
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

func TestScaledEVThreshold(t *testing.T) {
	base := 0.03
	tests := []struct {
		name      string
		bookCount int
		expected  float64
	}{
		{"6 books = base", 6, 0.03},
		{"7 books = base", 7, 0.03},
		{"5 books = +1%", 5, 0.04},
		{"4 books = +2%", 4, 0.05},
		{"3 books = +3%", 3, 0.06},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScaledEVThreshold(base, tt.bookCount)
			if math.Abs(result-tt.expected) > 0.0001 {
				t.Errorf("ScaledEVThreshold(%v, %d) = %v, want %v",
					base, tt.bookCount, result, tt.expected)
			}
		})
	}
}

func TestShrinkToward(t *testing.T) {
	tests := []struct {
		name                         string
		observed, prior              float64
		bookCount, fullWeightAt      int
		expected                     float64
		delta                        float64
	}{
		{"6+ books = no shrink", 0.60, 0.50, 6, 6, 0.60, 0.001},
		{"7 books = no shrink", 0.60, 0.50, 7, 6, 0.60, 0.001},
		{"5 books = 17% toward prior", 0.60, 0.50, 5, 6, (5*0.60 + 1*0.50) / 6, 0.001},
		{"4 books = 33% toward prior", 0.60, 0.50, 4, 6, (4*0.60 + 2*0.50) / 6, 0.001},
		{"equal obs/prior = no effect", 0.50, 0.50, 4, 6, 0.50, 0.001},
		{"1 book = 83% toward prior", 0.70, 0.50, 1, 6, (1*0.70 + 5*0.50) / 6, 0.001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShrinkToward(tt.observed, tt.prior, tt.bookCount, tt.fullWeightAt)
			if math.Abs(result-tt.expected) > tt.delta {
				t.Errorf("ShrinkToward(%v, %v, %d, %d) = %v, want %v",
					tt.observed, tt.prior, tt.bookCount, tt.fullWeightAt, result, tt.expected)
			}
		})
	}
}

func TestEVThresholdLogic(t *testing.T) {
	cfg := DefaultConfig()

	// Test that default config has sensible values
	if cfg.EVThreshold != 0.03 {
		t.Errorf("Default EV threshold should be 3%%, got %v", cfg.EVThreshold)
	}
	if cfg.KellyFraction != 0.25 {
		t.Errorf("Default Kelly fraction should be 25%%, got %v", cfg.KellyFraction)
	}
}
