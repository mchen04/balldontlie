package odds

import (
	"math"
	"testing"
)

func TestRemoveVig(t *testing.T) {
	tests := []struct {
		name        string
		impliedA    float64
		impliedB    float64
		expectedA   float64
		expectedB   float64
		delta       float64
	}{
		{
			name:      "Standard -110/-110",
			impliedA:  0.5238, // -110
			impliedB:  0.5238, // -110
			expectedA: 0.5,
			expectedB: 0.5,
			delta:     0.001,
		},
		{
			name:      "Favorite -150/+130",
			impliedA:  0.6,    // -150
			impliedB:  0.4348, // +130
			expectedA: 0.58,
			expectedB: 0.42,
			delta:     0.01,
		},
		{
			name:      "Heavy favorite -300/+250",
			impliedA:  0.75,   // -300
			impliedB:  0.2857, // +250
			expectedA: 0.724,
			expectedB: 0.276,
			delta:     0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultA, resultB := RemoveVig(tt.impliedA, tt.impliedB)

			if math.Abs(resultA-tt.expectedA) > tt.delta {
				t.Errorf("RemoveVig probA = %v, want %v", resultA, tt.expectedA)
			}
			if math.Abs(resultB-tt.expectedB) > tt.delta {
				t.Errorf("RemoveVig probB = %v, want %v", resultB, tt.expectedB)
			}

			// Verify they sum to 1
			sum := resultA + resultB
			if math.Abs(sum-1.0) > 0.001 {
				t.Errorf("RemoveVig probs should sum to 1, got %v", sum)
			}
		})
	}
}

func TestRemoveVigFromAmerican(t *testing.T) {
	tests := []struct {
		name      string
		oddsA     int
		oddsB     int
		expectedA float64
		expectedB float64
		delta     float64
	}{
		{
			name:      "Standard -110/-110",
			oddsA:     -110,
			oddsB:     -110,
			expectedA: 0.5,
			expectedB: 0.5,
			delta:     0.001,
		},
		{
			name:      "Even money",
			oddsA:     100,
			oddsB:     -100,
			expectedA: 0.5,
			expectedB: 0.5,
			delta:     0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultA, resultB := RemoveVigFromAmerican(tt.oddsA, tt.oddsB)

			if math.Abs(resultA-tt.expectedA) > tt.delta {
				t.Errorf("RemoveVigFromAmerican probA = %v, want %v", resultA, tt.expectedA)
			}
			if math.Abs(resultB-tt.expectedB) > tt.delta {
				t.Errorf("RemoveVigFromAmerican probB = %v, want %v", resultB, tt.expectedB)
			}
		})
	}
}

func TestRemoveVigEdgeCases(t *testing.T) {
	// Test invalid inputs
	a, b := RemoveVig(0, 0.5)
	if a != 0 || b != 0 {
		t.Error("RemoveVig should return 0,0 for zero input")
	}

	a, b = RemoveVig(-0.5, 0.5)
	if a != 0 || b != 0 {
		t.Error("RemoveVig should return 0,0 for negative input")
	}
}

func TestRemoveVigPower(t *testing.T) {
	tests := []struct {
		name       string
		impliedA   float64
		impliedB   float64
		expectedA  float64
		expectedB  float64
		delta      float64
	}{
		{
			name:      "Standard -110/-110 (equal odds)",
			impliedA:  0.5238, // -110
			impliedB:  0.5238, // -110
			expectedA: 0.5,
			expectedB: 0.5,
			delta:     0.001,
		},
		{
			name:      "Favorite -150/+130",
			impliedA:  0.6,    // -150 favorite
			impliedB:  0.4348, // +130 longshot
			expectedA: 0.576,  // Favorite adjusted slightly less
			expectedB: 0.424,  // Longshot adjusted more
			delta:     0.01,
		},
		{
			name:      "Heavy favorite -300/+250",
			impliedA:  0.75,   // -300 heavy favorite
			impliedB:  0.2857, // +250 big longshot
			expectedA: 0.735,  // Favorite adjusted slightly
			expectedB: 0.265,  // Longshot adjusted significantly more
			delta:     0.02,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultA, resultB := RemoveVigPower(tt.impliedA, tt.impliedB)

			// Verify they sum to 1 (most important property)
			sum := resultA + resultB
			if math.Abs(sum-1.0) > 0.001 {
				t.Errorf("RemoveVigPower probs should sum to 1, got %v", sum)
			}

			if math.Abs(resultA-tt.expectedA) > tt.delta {
				t.Errorf("RemoveVigPower probA = %v, want %v (±%v)", resultA, tt.expectedA, tt.delta)
			}
			if math.Abs(resultB-tt.expectedB) > tt.delta {
				t.Errorf("RemoveVigPower probB = %v, want %v (±%v)", resultB, tt.expectedB, tt.delta)
			}
		})
	}
}

func TestRemoveVigPowerVsMultiplicative(t *testing.T) {
	// Test that Power method adjusts longshots MORE than multiplicative
	// and favorites LESS than multiplicative (relative to original implied)
	tests := []struct {
		name      string
		impliedA  float64 // favorite
		impliedB  float64 // longshot
	}{
		{"Moderate favorite", 0.6, 0.4348},
		{"Heavy favorite", 0.75, 0.2857},
		{"Slight favorite", 0.55, 0.50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get both results
			multA, multB := RemoveVig(tt.impliedA, tt.impliedB)
			powerA, powerB := RemoveVigPower(tt.impliedA, tt.impliedB)

			// Both should sum to 1
			if math.Abs(multA+multB-1.0) > 0.001 {
				t.Errorf("Multiplicative should sum to 1")
			}
			if math.Abs(powerA+powerB-1.0) > 0.001 {
				t.Errorf("Power should sum to 1")
			}

			// Calculate adjustment ratios (true / implied)
			// For favorites: Power should adjust LESS than multiplicative
			// For longshots: Power should adjust MORE than multiplicative
			multFavRatio := multA / tt.impliedA
			powerFavRatio := powerA / tt.impliedA
			multLongshotRatio := multB / tt.impliedB
			powerLongshotRatio := powerB / tt.impliedB

			// Power method should deflate longshots more (smaller ratio)
			if powerLongshotRatio > multLongshotRatio+0.001 {
				t.Errorf("Power should deflate longshots more: power ratio=%v, mult ratio=%v",
					powerLongshotRatio, multLongshotRatio)
			}

			// Power method should deflate favorites less (larger ratio)
			if powerFavRatio < multFavRatio-0.001 {
				t.Errorf("Power should deflate favorites less: power ratio=%v, mult ratio=%v",
					powerFavRatio, multFavRatio)
			}
		})
	}
}

func TestRemoveVigPowerFromAmerican(t *testing.T) {
	tests := []struct {
		name      string
		oddsA     int
		oddsB     int
		expectedA float64
		expectedB float64
		delta     float64
	}{
		{
			name:      "Standard -110/-110",
			oddsA:     -110,
			oddsB:     -110,
			expectedA: 0.5,
			expectedB: 0.5,
			delta:     0.001,
		},
		{
			name:      "Favorite -150/+130",
			oddsA:     -150,
			oddsB:     130,
			expectedA: 0.576,
			expectedB: 0.424,
			delta:     0.02,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultA, resultB := RemoveVigPowerFromAmerican(tt.oddsA, tt.oddsB)

			// Verify sum to 1
			sum := resultA + resultB
			if math.Abs(sum-1.0) > 0.001 {
				t.Errorf("RemoveVigPowerFromAmerican should sum to 1, got %v", sum)
			}

			if math.Abs(resultA-tt.expectedA) > tt.delta {
				t.Errorf("RemoveVigPowerFromAmerican probA = %v, want %v", resultA, tt.expectedA)
			}
			if math.Abs(resultB-tt.expectedB) > tt.delta {
				t.Errorf("RemoveVigPowerFromAmerican probB = %v, want %v", resultB, tt.expectedB)
			}
		})
	}
}

func TestRemoveVigPowerEdgeCases(t *testing.T) {
	// Test invalid inputs
	a, b := RemoveVigPower(0, 0.5)
	if a != 0 || b != 0 {
		t.Error("RemoveVigPower should return 0,0 for zero input")
	}

	a, b = RemoveVigPower(-0.5, 0.5)
	if a != 0 || b != 0 {
		t.Error("RemoveVigPower should return 0,0 for negative input")
	}

	// Already sums to 1 - should return as-is
	a, b = RemoveVigPower(0.6, 0.4)
	if math.Abs(a-0.6) > 0.001 || math.Abs(b-0.4) > 0.001 {
		t.Errorf("RemoveVigPower should return as-is when sum=1, got %v, %v", a, b)
	}
}

func TestFindPowerExponent(t *testing.T) {
	// For standard -110/-110 (sum = 1.0476), k should be > 1
	// since we need to reduce the sum and higher k reduces p^k when p < 1
	k := findPowerExponent(0.5238, 0.5238)
	if k <= 1.0 {
		t.Errorf("findPowerExponent should return k > 1 for overround, got %v", k)
	}

	// Verify the result sums to 1
	sum := math.Pow(0.5238, k) + math.Pow(0.5238, k)
	if math.Abs(sum-1.0) > 0.001 {
		t.Errorf("k=%v should give sum=1, got %v", k, sum)
	}

	// For heavy vig market, k should be higher (more correction needed)
	kHeavy := findPowerExponent(0.75, 0.40) // sum = 1.15
	kLight := findPowerExponent(0.52, 0.52) // sum = 1.04

	if kHeavy <= kLight {
		t.Errorf("Heavier vig should need higher k: heavy=%v, light=%v", kHeavy, kLight)
	}
}
