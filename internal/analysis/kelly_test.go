package analysis

import (
	"math"
	"testing"
)

func TestCalculateKelly(t *testing.T) {
	tests := []struct {
		name        string
		trueProb    float64
		kalshiPrice float64
		fraction    float64
		expectedMin float64
		expectedMax float64
	}{
		{
			name:        "Positive edge - full Kelly",
			trueProb:    0.55,
			kalshiPrice: 0.50,
			fraction:    1.0,
			expectedMin: 0.05,
			expectedMax: 0.15,
		},
		{
			name:        "Positive edge - quarter Kelly",
			trueProb:    0.55,
			kalshiPrice: 0.50,
			fraction:    0.25,
			expectedMin: 0.01,
			expectedMax: 0.04,
		},
		{
			name:        "No edge - should be near zero",
			trueProb:    0.50,
			kalshiPrice: 0.50,
			fraction:    1.0,
			expectedMin: 0.0,
			expectedMax: 0.01,
		},
		{
			name:        "Negative edge - should be zero",
			trueProb:    0.45,
			kalshiPrice: 0.50,
			fraction:    1.0,
			expectedMin: 0.0,
			expectedMax: 0.0,
		},
		{
			name:        "Big edge on underdog",
			trueProb:    0.40,
			kalshiPrice: 0.25, // Paying 25 cents, true value 40%
			fraction:    0.25,
			expectedMin: 0.02,
			expectedMax: 0.10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateKelly(tt.trueProb, tt.kalshiPrice, tt.fraction)

			if result < tt.expectedMin || result > tt.expectedMax {
				t.Errorf("CalculateKelly(%v, %v, %v) = %v, expected between %v and %v",
					tt.trueProb, tt.kalshiPrice, tt.fraction, result, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestCalculateKellyEdgeCases(t *testing.T) {
	// Invalid inputs should return 0
	cases := []struct {
		trueProb    float64
		kalshiPrice float64
		fraction    float64
	}{
		{0.5, 0, 0.25},      // Zero price
		{0.5, 1, 0.25},      // Price of 1
		{0, 0.5, 0.25},      // Zero probability
		{1, 0.5, 0.25},      // Probability of 1
		{-0.5, 0.5, 0.25},   // Negative probability
		{0.5, -0.5, 0.25},   // Negative price
	}

	for _, tc := range cases {
		result := CalculateKelly(tc.trueProb, tc.kalshiPrice, tc.fraction)
		if result != 0 {
			t.Errorf("CalculateKelly(%v, %v, %v) = %v, expected 0 for invalid input",
				tc.trueProb, tc.kalshiPrice, tc.fraction, result)
		}
	}
}

func TestCalculateKellyDecimal(t *testing.T) {
	tests := []struct {
		name        string
		trueProb    float64
		decimalOdds float64
		fraction    float64
		expectedMin float64
		expectedMax float64
	}{
		{
			name:        "2.0 odds with 55% edge",
			trueProb:    0.55,
			decimalOdds: 2.0, // Even money
			fraction:    1.0,
			expectedMin: 0.05,
			expectedMax: 0.15,
		},
		{
			name:        "3.0 odds with 40% true prob",
			trueProb:    0.40,
			decimalOdds: 3.0, // Implied 33%
			fraction:    1.0,
			expectedMin: 0.05,
			expectedMax: 0.15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateKellyDecimal(tt.trueProb, tt.decimalOdds, tt.fraction)

			if result < tt.expectedMin || result > tt.expectedMax {
				t.Errorf("CalculateKellyDecimal(%v, %v, %v) = %v, expected between %v and %v",
					tt.trueProb, tt.decimalOdds, tt.fraction, result, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestOptimalBetSize(t *testing.T) {
	bankroll := 1000.0
	kellyFraction := 0.05 // 5%

	expected := 50.0
	result := OptimalBetSize(bankroll, kellyFraction)

	if math.Abs(result-expected) > 0.01 {
		t.Errorf("OptimalBetSize(%v, %v) = %v, want %v",
			bankroll, kellyFraction, result, expected)
	}
}

func TestCalculateKellyContractsWithAskDepth(t *testing.T) {
	// Without depth cap
	contracts := CalculateKellyContracts(0.60, 0.45, 0.25, 1000, 100, 45)
	if contracts <= 0 {
		t.Fatal("Expected positive contracts without depth cap")
	}

	// With depth cap lower than Kelly
	capped := CalculateKellyContracts(0.60, 0.45, 0.25, 1000, 100, 45, 3)
	if capped > 3 {
		t.Errorf("Expected contracts capped at 3, got %d", capped)
	}

	// With depth cap higher than Kelly — should not change result
	uncapped := CalculateKellyContracts(0.60, 0.45, 0.25, 1000, 100, 45, 10000)
	if uncapped != contracts {
		t.Errorf("High depth cap should not change result: %d vs %d", uncapped, contracts)
	}

	// askDepth=0 means no cap (same as no arg)
	noCap := CalculateKellyContracts(0.60, 0.45, 0.25, 1000, 100, 45, 0)
	if noCap != contracts {
		t.Errorf("askDepth=0 should mean no cap: %d vs %d", noCap, contracts)
	}
}

func TestKellyWithEdge(t *testing.T) {
	// If we know the edge directly, we can use a simplified formula
	edge := 0.05        // 5% edge
	impliedProb := 0.50 // Fair market at 50%
	fraction := 0.25    // Quarter Kelly

	result := KellyWithEdge(edge, impliedProb, fraction)

	// Should be positive but not huge
	if result <= 0 || result > 0.1 {
		t.Errorf("KellyWithEdge(%v, %v, %v) = %v, expected small positive value",
			edge, impliedProb, fraction, result)
	}
}
