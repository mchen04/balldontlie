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

func TestLogit(t *testing.T) {
	tests := []struct {
		p        float64
		expected float64
		delta    float64
	}{
		{0.5, 0.0, 0.001},
		{0.75, math.Log(3), 0.001},   // log(0.75/0.25) = log(3)
		{0.25, -math.Log(3), 0.001},  // log(0.25/0.75) = -log(3)
		{0.0, math.Log(0.001/0.999), 0.001}, // clamped
		{1.0, math.Log(0.999/0.001), 0.001}, // clamped
	}
	for _, tt := range tests {
		result := Logit(tt.p)
		if math.Abs(result-tt.expected) > tt.delta {
			t.Errorf("Logit(%.4f) = %.4f, want %.4f", tt.p, result, tt.expected)
		}
	}
}

func TestSigmoid(t *testing.T) {
	tests := []struct {
		x        float64
		expected float64
		delta    float64
	}{
		{0, 0.5, 0.001},
		{1, 0.7311, 0.001},
		{-1, 0.2689, 0.001},
		{10, 0.99995, 0.0001},
		{-10, 0.00005, 0.0001},
	}
	for _, tt := range tests {
		result := Sigmoid(tt.x)
		if math.Abs(result-tt.expected) > tt.delta {
			t.Errorf("Sigmoid(%.1f) = %.6f, want %.6f", tt.x, result, tt.expected)
		}
	}
}

func TestLogitSigmoidRoundTrip(t *testing.T) {
	probs := []float64{0.01, 0.1, 0.25, 0.5, 0.75, 0.9, 0.99}
	for _, p := range probs {
		recovered := Sigmoid(Logit(p))
		if math.Abs(recovered-p) > 1e-6 {
			t.Errorf("Round trip failed: Sigmoid(Logit(%.4f)) = %.8f", p, recovered)
		}
	}
}

func TestRegBetaI(t *testing.T) {
	tests := []struct {
		a, b, x  float64
		expected float64
		delta    float64
	}{
		{1, 1, 0.5, 0.5, 0.001},   // Uniform: I_0.5(1,1) = 0.5
		{1, 1, 0.0, 0.0, 0.001},   // Boundary
		{1, 1, 1.0, 1.0, 0.001},   // Boundary
		{2, 2, 0.5, 0.5, 0.001},   // Symmetric: I_0.5(2,2) = 0.5
		{0.5, 3.5, 0.5, 0.967, 0.01}, // I_0.5(0.5, 3.5) ≈ 0.967
	}
	for _, tt := range tests {
		result := RegBetaI(tt.a, tt.b, tt.x)
		if math.Abs(result-tt.expected) > tt.delta {
			t.Errorf("RegBetaI(%.1f, %.1f, %.2f) = %.6f, want %.6f", tt.a, tt.b, tt.x, result, tt.expected)
		}
	}
}

func TestTDistCDF(t *testing.T) {
	tests := []struct {
		tval, df float64
		expected float64
		delta    float64
	}{
		{0, 7, 0.5, 0.001},
		{0, 100, 0.5, 0.001},
		// t=1 with df=7: known value ~0.8261
		{1, 7, 0.8261, 0.005},
		// t=-1 with df=7: should be symmetric
		{-1, 7, 0.1739, 0.005},
		// t=2 with df=9: known value ~0.9620
		{2, 9, 0.9620, 0.005},
	}
	for _, tt := range tests {
		result := TDistCDF(tt.tval, tt.df)
		if math.Abs(result-tt.expected) > tt.delta {
			t.Errorf("TDistCDF(%.1f, %.0f) = %.6f, want %.6f", tt.tval, tt.df, result, tt.expected)
		}
	}
}

func TestTDistInvCDF(t *testing.T) {
	tests := []struct {
		p, df    float64
		expected float64
		delta    float64
	}{
		{0.5, 7, 0.0, 0.001},
		{0.5, 100, 0.0, 0.001},
		{0.975, 7, 2.365, 0.01},   // Known critical value
		{0.025, 7, -2.365, 0.01},  // Symmetric
		{0.95, 9, 1.833, 0.01},    // Known critical value
	}
	for _, tt := range tests {
		result := TDistInvCDF(tt.p, tt.df)
		if math.Abs(result-tt.expected) > tt.delta {
			t.Errorf("TDistInvCDF(%.4f, %.0f) = %.4f, want %.4f", tt.p, tt.df, result, tt.expected)
		}
	}
}

func TestTDistCDFInvRoundTrip(t *testing.T) {
	dfs := []float64{5, 7, 9, 30}
	probs := []float64{0.01, 0.1, 0.25, 0.5, 0.75, 0.9, 0.99}
	for _, df := range dfs {
		for _, p := range probs {
			tval := TDistInvCDF(p, df)
			recovered := TDistCDF(tval, df)
			if math.Abs(recovered-p) > 1e-4 {
				t.Errorf("Round trip failed: TDistCDF(TDistInvCDF(%.4f, %.0f), %.0f) = %.8f", p, df, df, recovered)
			}
		}
	}
}

func TestWinsorizeLogits(t *testing.T) {
	t.Run("caps outlier at 2σ", func(t *testing.T) {
		// 4 books near 0.55, one outlier at 0.95
		logits := []float64{Logit(0.55), Logit(0.55), Logit(0.55), Logit(0.55), Logit(0.95)}
		weights := []float64{1, 1, 1, 1, 1}

		// Before winsorization, last logit is ~2.94 while others are ~0.20
		outlierBefore := logits[4]

		WinsorizeLogits(logits, weights, 2.0)

		// Outlier should be capped down
		if logits[4] >= outlierBefore {
			t.Errorf("Outlier should be capped: was %.4f, still %.4f", outlierBefore, logits[4])
		}

		// Non-outliers should be unchanged
		expected := Logit(0.55)
		for i := 0; i < 4; i++ {
			if math.Abs(logits[i]-expected) > 0.001 {
				t.Errorf("Non-outlier logit[%d] changed: %.4f vs expected %.4f", i, logits[i], expected)
			}
		}
	})

	t.Run("no change with fewer than 3 entries", func(t *testing.T) {
		logits := []float64{Logit(0.90), Logit(0.10)}
		weights := []float64{1, 1}
		before0, before1 := logits[0], logits[1]
		WinsorizeLogits(logits, weights, 2.0)
		if logits[0] != before0 || logits[1] != before1 {
			t.Error("Should not modify with < 3 entries")
		}
	})

	t.Run("no change when all similar", func(t *testing.T) {
		logits := []float64{Logit(0.50), Logit(0.51), Logit(0.49), Logit(0.50)}
		weights := []float64{1, 1, 1, 1}
		originals := make([]float64, len(logits))
		copy(originals, logits)
		WinsorizeLogits(logits, weights, 2.0)
		for i := range logits {
			if math.Abs(logits[i]-originals[i]) > 0.001 {
				t.Errorf("logit[%d] changed when all similar: %.4f vs %.4f", i, logits[i], originals[i])
			}
		}
	})
}

func TestTDistVsNormal(t *testing.T) {
	// t-distribution with df=7 should assign more mass to extremes than normal
	// At t=3: NormalCDF(3) ≈ 0.9987, TDistCDF(3, 7) should be lower (more tail mass)
	normalProb := NormalCDF(3)
	tProb := TDistCDF(3, 7)
	if tProb >= normalProb {
		t.Errorf("t-dist should have fatter tails: TDistCDF(3,7)=%.6f >= NormalCDF(3)=%.6f", tProb, normalProb)
	}

	// The difference should be meaningful (not negligible)
	diff := normalProb - tProb
	if diff < 0.005 {
		t.Errorf("Tail difference too small: %.6f (expected > 0.005)", diff)
	}
}
