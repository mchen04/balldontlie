package mathutil

import (
	"math"
	"testing"
)

func TestNormalCDF(t *testing.T) {
	tests := []struct {
		z        float64
		expected float64
		delta    float64
	}{
		{0, 0.5, 0.001},
		{1, 0.8413, 0.001},
		{-1, 0.1587, 0.001},
		{2, 0.9772, 0.001},
		{-2, 0.0228, 0.001},
	}

	for _, tt := range tests {
		result := NormalCDF(tt.z)
		if math.Abs(result-tt.expected) > tt.delta {
			t.Errorf("NormalCDF(%.1f) = %.4f, want %.4f", tt.z, result, tt.expected)
		}
	}
}

func TestNormalInvCDF(t *testing.T) {
	tests := []struct {
		p        float64
		expected float64
		delta    float64
	}{
		{0.5, 0, 0.001},
		{0.8413, 1.0, 0.01},
		{0.1587, -1.0, 0.01},
		{0.9772, 2.0, 0.01},
		{0.0228, -2.0, 0.01},
	}

	for _, tt := range tests {
		result := NormalInvCDF(tt.p)
		if math.Abs(result-tt.expected) > tt.delta {
			t.Errorf("NormalInvCDF(%.4f) = %.4f, want %.4f", tt.p, result, tt.expected)
		}
	}
}

func TestNormalInvCDFBoundary(t *testing.T) {
	// Edge cases: clamped to ±10
	if NormalInvCDF(0) != -10 {
		t.Errorf("NormalInvCDF(0) should be -10, got %v", NormalInvCDF(0))
	}
	if NormalInvCDF(1) != 10 {
		t.Errorf("NormalInvCDF(1) should be 10, got %v", NormalInvCDF(1))
	}
}

func TestNormalCDFInvRoundTrip(t *testing.T) {
	// NormalCDF(NormalInvCDF(p)) ≈ p for valid p
	probs := []float64{0.01, 0.1, 0.25, 0.5, 0.75, 0.9, 0.99}
	for _, p := range probs {
		z := NormalInvCDF(p)
		recovered := NormalCDF(z)
		if math.Abs(recovered-p) > 1e-6 {
			t.Errorf("Round trip failed: NormalCDF(NormalInvCDF(%.4f)) = %.8f", p, recovered)
		}
	}
}
