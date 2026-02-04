package odds

import (
	"math"
	"testing"
)

func TestAmericanToImplied(t *testing.T) {
	tests := []struct {
		name     string
		odds     int
		expected float64
		delta    float64
	}{
		{"Even money +100", 100, 0.5, 0.001},
		{"Even money -100", -100, 0.5, 0.001},
		{"Favorite -150", -150, 0.6, 0.001},
		{"Underdog +150", 150, 0.4, 0.001},
		{"Heavy favorite -300", -300, 0.75, 0.001},
		{"Big underdog +300", 300, 0.25, 0.001},
		{"Standard -110", -110, 0.5238, 0.001},
		{"Zero odds", 0, 0, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AmericanToImplied(tt.odds)
			if math.Abs(result-tt.expected) > tt.delta {
				t.Errorf("AmericanToImplied(%d) = %v, want %v", tt.odds, result, tt.expected)
			}
		})
	}
}

func TestOddsToImplied(t *testing.T) {
	tests := []struct {
		name     string
		odds     int
		expected float64
		delta    float64
	}{
		// American odds (|value| >= 100)
		{"American favorite -150", -150, 0.6, 0.001},
		{"American underdog +150", 150, 0.4, 0.001},
		{"American even -100", -100, 0.5, 0.001},
		{"American even +100", 100, 0.5, 0.001},

		// Kalshi prices in cents (1-99)
		{"Kalshi price 45 cents", 45, 0.45, 0.001},
		{"Kalshi price 55 cents", 55, 0.55, 0.001},
		{"Kalshi price 1 cent", 1, 0.01, 0.001},
		{"Kalshi price 99 cents", 99, 0.99, 0.001},
		{"Kalshi price 50 cents", 50, 0.50, 0.001},

		// Zero
		{"Zero odds", 0, 0, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OddsToImplied(tt.odds)
			if math.Abs(result-tt.expected) > tt.delta {
				t.Errorf("OddsToImplied(%d) = %v, want %v", tt.odds, result, tt.expected)
			}
		})
	}
}
