package analysis

import (
	"math"
	"testing"
)

func TestNegBinPMF(t *testing.T) {
	tests := []struct {
		name     string
		k        int
		mu       float64
		r        float64
		expected float64
		delta    float64
	}{
		{
			name:     "Low dispersion (high r) should approximate Poisson",
			k:        5,
			mu:       5.0,
			r:        1000, // Very high r → Poisson
			expected: PoissonPMF(5, 5.0),
			delta:    0.01,
		},
		{
			name:     "P(X=0) at mean=3, r=10",
			k:        0,
			mu:       3.0,
			r:        10.0,
			expected: 0.073, // Calculated: (10/(10+3))^10 ≈ 0.073
			delta:    0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NegBinPMF(tt.k, tt.mu, tt.r)
			if math.Abs(result-tt.expected) > tt.delta {
				t.Errorf("NegBinPMF(%d, %.1f, %.1f) = %v, want %v (±%v)",
					tt.k, tt.mu, tt.r, result, tt.expected, tt.delta)
			}
		})
	}
}

func TestNegBinPMFSumsToOne(t *testing.T) {
	// The PMF should approximately sum to 1 over a large range
	mu := 5.0
	r := 15.0

	sum := 0.0
	for k := 0; k <= 100; k++ {
		sum += NegBinPMF(k, mu, r)
	}

	if math.Abs(sum-1.0) > 0.001 {
		t.Errorf("NegBinPMF should sum to ~1.0 over 0-100, got %v", sum)
	}
}

func TestNegBinCDFOver(t *testing.T) {
	tests := []struct {
		name     string
		k        int
		mu       float64
		r        float64
		expected float64
		delta    float64
	}{
		{
			name:     "P(X >= 0) should be 1",
			k:        0,
			mu:       5.0,
			r:        10.0,
			expected: 1.0,
			delta:    0.001,
		},
		{
			name:     "High r should match Poisson",
			k:        5,
			mu:       5.0,
			r:        1000, // Very high r → Poisson
			expected: PoissonCDFOver(5, 5.0),
			delta:    0.02,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NegBinCDFOver(tt.k, tt.mu, tt.r)
			if math.Abs(result-tt.expected) > tt.delta {
				t.Errorf("NegBinCDFOver(%d, %.1f, %.1f) = %v, want %v (±%v)",
					tt.k, tt.mu, tt.r, result, tt.expected, tt.delta)
			}
		})
	}
}

func TestNegBinHeavierTailsThanPoisson(t *testing.T) {
	// Key property: NegBin with same mean should have heavier tails than Poisson
	// because variance > mean (overdispersion)
	mu := 6.0
	r := 12.0 // Gives variance = 6 + 36/12 = 6 + 3 = 9 > 6

	// At the mean, CDF should be similar
	threshold := 10 // A bit above mean

	poissonProb := PoissonCDFOver(threshold, mu)
	negbinProb := NegBinCDFOver(threshold, mu, r)

	// NegBin should have HIGHER probability of extreme values (heavier tails)
	if negbinProb <= poissonProb {
		t.Errorf("NegBin should have heavier tails than Poisson: NegBin P(X>=%d)=%.4f, Poisson=%.4f",
			threshold, negbinProb, poissonProb)
	}
}

func TestInferNegBinMean(t *testing.T) {
	// Test that we can recover the mean from a known probability
	mu := 8.0
	r := 26.4 // 3.3 * 8 (rebounds default)
	threshold := 10

	// Get the "true" probability
	trueProb := NegBinCDFOver(threshold, mu, r)

	// Try to infer the mean back
	inferred := InferNegBinMean(threshold, trueProb, r)

	if math.Abs(inferred-mu) > 0.5 {
		t.Errorf("InferNegBinMean should recover mean=%.1f, got %.1f (target prob=%.4f)",
			mu, inferred, trueProb)
	}
}

func TestDefaultDispersion(t *testing.T) {
	tests := []struct {
		propType string
		mean     float64
		expected float64 // Expected r value
	}{
		{"rebounds", 10.0, 33.0},  // 3.3 * 10
		{"assists", 8.0, 20.0},    // 2.5 * 8
		{"threes", 4.0, 8.0},      // 2.0 * 4
		{"unknown", 5.0, 16.5},    // 3.3 * 5 (default)
	}

	for _, tt := range tests {
		t.Run(tt.propType, func(t *testing.T) {
			result := DefaultDispersion(tt.propType, tt.mean)
			if math.Abs(result-tt.expected) > 0.1 {
				t.Errorf("DefaultDispersion(%s, %.1f) = %v, want %v",
					tt.propType, tt.mean, result, tt.expected)
			}
		})
	}

	// Test edge case: zero mean returns default
	r := DefaultDispersion("rebounds", 0)
	if r != 10 {
		t.Errorf("DefaultDispersion with mean=0 should return 10, got %v", r)
	}
}

func TestPropDistributionType(t *testing.T) {
	tests := []struct {
		propType string
		expected string
	}{
		{"points", "normal"},
		{"rebounds", "negbin"},
		{"assists", "negbin"},
		{"threes", "negbin"},
		{"unknown", "negbin"}, // Default to negbin for count data
	}

	for _, tt := range tests {
		t.Run(tt.propType, func(t *testing.T) {
			result := PropDistributionType(tt.propType)
			if result != tt.expected {
				t.Errorf("PropDistributionType(%s) = %s, want %s",
					tt.propType, result, tt.expected)
			}
		})
	}
}

func TestEstimateProbabilityAtLineNegBin(t *testing.T) {
	// Test that the estimation preserves the original probability at the same line
	bdlLine := 9.5
	bdlProb := 0.45
	kalshiLine := 10.0 // Same threshold as BDL

	result := EstimateProbabilityAtLine(bdlLine, bdlProb, kalshiLine, "rebounds")

	// Should be very close to the original probability
	if math.Abs(result-bdlProb) > 0.02 {
		t.Errorf("EstimateProbabilityAtLine should preserve prob at same line: got %.4f, want ~%.4f",
			result, bdlProb)
	}
}

func TestNegBinVsPoissonComparison(t *testing.T) {
	// Compare NegBin and Poisson for rebounds (typical use case)
	mu := 8.0
	r := 26.4 // 3.3 * 8

	t.Logf("Comparison for rebounds with mean=%.1f:", mu)
	t.Logf("NegBin r=%.1f (variance=%.2f)", r, mu+mu*mu/r)
	t.Logf("Poisson (variance=%.2f)", mu)

	for threshold := 6; threshold <= 14; threshold += 2 {
		poisson := PoissonCDFOver(threshold, mu)
		negbin := NegBinCDFOver(threshold, mu, r)
		diff := (negbin - poisson) * 100

		t.Logf("P(X >= %d): Poisson=%.2f%%, NegBin=%.2f%%, diff=%+.2f%%",
			threshold, poisson*100, negbin*100, diff)
	}
}
