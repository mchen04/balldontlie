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
