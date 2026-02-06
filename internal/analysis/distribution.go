package analysis

import (
	"math"
)

// Distribution inference and probability calculation for player props
// Uses Poisson for discrete counts (rebounds, assists, threes)
// Uses Normal for points (higher values, approximately continuous)

// PoissonPMF calculates P(X = k) for Poisson distribution with mean λ
func PoissonPMF(k int, lambda float64) float64 {
	if k < 0 || lambda <= 0 {
		return 0
	}
	// P(X=k) = e^(-λ) * λ^k / k!
	// Use log to avoid overflow
	logProb := -lambda + float64(k)*math.Log(lambda) - logFactorial(k)
	return math.Exp(logProb)
}

func logFactorial(n int) float64 {
	if n <= 1 {
		return 0
	}
	result := 0.0
	for i := 2; i <= n; i++ {
		result += math.Log(float64(i))
	}
	return result
}

// PoissonCDFOver calculates P(X >= k) for Poisson distribution
// This is what we need: probability of hitting k or more
func PoissonCDFOver(k int, lambda float64) float64 {
	if lambda <= 0 {
		return 0
	}
	// P(X >= k) = 1 - P(X < k) = 1 - Σ P(X=i) for i=0 to k-1
	sum := 0.0
	for i := 0; i < k; i++ {
		sum += PoissonPMF(i, lambda)
	}
	return 1 - sum
}

// NegBinPMF calculates P(X = k) for Negative Binomial distribution
// Parameters: mu (mean), r (dispersion parameter)
// Variance = mu + mu²/r (when r→∞, reduces to Poisson)
// Uses alternative parameterization: p = r/(r+mu), so mean = r(1-p)/p = mu
func NegBinPMF(k int, mu, r float64) float64 {
	if k < 0 || mu <= 0 || r <= 0 {
		return 0
	}

	// Convert to standard NB parameters: p = r/(r+mu)
	p := r / (r + mu)
	q := 1 - p // q = mu/(r+mu)

	// P(X=k) = C(k+r-1, k) * p^r * q^k
	// Use log-space: log(C(k+r-1,k)) + r*log(p) + k*log(q)
	// log(C(k+r-1,k)) = logΓ(k+r) - logΓ(r) - logΓ(k+1)
	lgKR, _ := math.Lgamma(float64(k) + r)
	lgR, _ := math.Lgamma(r)
	logProb := lgKR - lgR - logFactorial(k) +
		r*math.Log(p) + float64(k)*math.Log(q)

	return math.Exp(logProb)
}

// NegBinCDFOver calculates P(X >= k) for Negative Binomial distribution
func NegBinCDFOver(k int, mu, r float64) float64 {
	if mu <= 0 || r <= 0 {
		return 0
	}
	// P(X >= k) = 1 - P(X < k) = 1 - Σ P(X=i) for i=0 to k-1
	sum := 0.0
	for i := 0; i < k; i++ {
		sum += NegBinPMF(i, mu, r)
	}
	return 1 - sum
}

// InferNegBinMean finds mu such that P(X >= threshold) ≈ targetProb
// Uses binary search since there's no closed-form solution
func InferNegBinMean(threshold int, targetProb, r float64) float64 {
	if targetProb <= 0 || targetProb >= 1 || threshold < 0 || r <= 0 {
		return 0
	}

	// Binary search for mu
	// Higher mu = higher probability of exceeding threshold
	low, high := 0.1, 100.0

	for i := 0; i < 100; i++ { // Max iterations
		mid := (low + high) / 2
		prob := NegBinCDFOver(threshold, mid, r)

		if math.Abs(prob-targetProb) < 0.001 {
			return mid
		}

		if prob < targetProb {
			// Need higher mean to get higher probability
			low = mid
		} else {
			high = mid
		}
	}

	return (low + high) / 2
}

// DefaultDispersion returns empirical dispersion parameter r for Negative Binomial
// Based on NBA player prop variance research:
// - Variance = mu + mu²/r, so r = mu² / (variance - mu)
// - Observed: rebounds var ≈ 1.3*mean, assists var ≈ 1.4*mean, threes var ≈ 1.5*mean
func DefaultDispersion(propType string, mean float64) float64 {
	if mean <= 0 {
		return 10 // Default fallback
	}

	// From variance = c * mean, we get r = mean / (c - 1)
	// rebounds: var ≈ 1.30*mean → r = mean/0.30 ≈ 3.3*mean
	// assists: var ≈ 1.40*mean → r = mean/0.40 ≈ 2.5*mean
	// threes: var ≈ 1.50*mean → r = mean/0.50 ≈ 2.0*mean
	switch propType {
	case "rebounds":
		return 3.3 * mean
	case "assists":
		return 2.5 * mean
	case "threes":
		return 2.0 * mean
	default:
		// Default: assume variance ≈ 1.3*mean
		return 3.3 * mean
	}
}

// InferPoissonMean finds λ such that P(X >= threshold) ≈ targetProb
// Uses binary search since there's no closed-form solution
func InferPoissonMean(threshold int, targetProb float64) float64 {
	if targetProb <= 0 || targetProb >= 1 || threshold < 0 {
		return 0
	}

	// Binary search for λ
	// Higher λ = higher probability of exceeding threshold
	low, high := 0.1, 100.0

	for i := 0; i < 100; i++ { // Max iterations
		mid := (low + high) / 2
		prob := PoissonCDFOver(threshold, mid)

		if math.Abs(prob-targetProb) < 0.001 {
			return mid
		}

		if prob < targetProb {
			// Need higher mean to get higher probability
			low = mid
		} else {
			high = mid
		}
	}

	return (low + high) / 2
}

// NormalCDF calculates the cumulative distribution function for normal distribution
func NormalCDF(x, mean, stddev float64) float64 {
	if stddev <= 0 {
		return 0
	}
	return 0.5 * (1 + math.Erf((x-mean)/(stddev*math.Sqrt(2))))
}

// NormalCDFOver calculates P(X >= k) for normal distribution
// Uses continuity correction for discrete outcomes
func NormalCDFOver(k, mean, stddev float64) float64 {
	if stddev <= 0 {
		return 0
	}
	// Continuity correction: P(X >= k) ≈ P(X > k - 0.5)
	return 1 - NormalCDF(k-0.5, mean, stddev)
}

// InverseNormalCDF calculates the inverse CDF (quantile function)
// Given probability p, returns z such that Φ(z) = p
// Uses Abramowitz and Stegun formula 26.2.23
func InverseNormalCDF(p float64) float64 {
	if p <= 0 || p >= 1 {
		return 0
	}

	// For p > 0.5, use symmetry: Φ^(-1)(p) = -Φ^(-1)(1-p)
	if p > 0.5 {
		return -InverseNormalCDF(1 - p)
	}

	// For p <= 0.5, the z-score is negative (left tail)
	// A&S formula gives the absolute value, so we negate
	t := math.Sqrt(-2 * math.Log(p))

	// Coefficients from Abramowitz & Stegun formula 26.2.23
	c0 := 2.515517
	c1 := 0.802853
	c2 := 0.010328
	d1 := 1.432788
	d2 := 0.189269
	d3 := 0.001308

	// Rational approximation gives |z|, negate for p < 0.5
	absZ := t - (c0+c1*t+c2*t*t)/(1+d1*t+d2*t*t+d3*t*t*t)
	return -absZ
}

// InferNormalMean finds μ such that P(X >= threshold) ≈ targetProb
// Given an assumed standard deviation
func InferNormalMean(threshold float64, targetProb float64, stddev float64) float64 {
	if targetProb <= 0 || targetProb >= 1 || stddev <= 0 {
		return 0
	}
	// P(X >= threshold) = targetProb
	// 1 - Φ((threshold - μ) / σ) = targetProb
	// Φ((threshold - μ) / σ) = 1 - targetProb
	// (threshold - μ) / σ = Φ^(-1)(1 - targetProb)
	// μ = threshold - σ * Φ^(-1)(1 - targetProb)

	z := InverseNormalCDF(1 - targetProb)
	return threshold - stddev*z
}

// PropDistributionType returns the recommended distribution for a prop type
func PropDistributionType(propType string) string {
	switch propType {
	case "points":
		return "normal"
	case "rebounds", "assists", "threes":
		// Use Negative Binomial to handle overdispersion (variance > mean)
		return "negbin"
	default:
		return "negbin" // Default to NegBin for unknown count types
	}
}

// DefaultStdDev returns typical standard deviation for points props
// Based on NBA player scoring variance analysis
func DefaultStdDev(propType string, inferredMean float64) float64 {
	switch propType {
	case "points":
		// NBA scoring SD is typically 25-35% of mean
		// Higher scorers have higher variance
		if inferredMean > 25 {
			return inferredMean * 0.32
		} else if inferredMean > 15 {
			return inferredMean * 0.30
		}
		return inferredMean * 0.35
	default:
		// For other props, SD ≈ sqrt(mean) for Poisson-like data
		return math.Sqrt(inferredMean)
	}
}

// EstimateProbabilityAtLine estimates P(X >= kalshiLine) given:
// - bdlLine: the BDL line (e.g., 19.5 for "over 19.5")
// - bdlProb: the true probability from BDL for that line
// - kalshiLine: the Kalshi threshold we want probability for
// - propType: "points", "rebounds", "assists", "threes"
func EstimateProbabilityAtLine(bdlLine float64, bdlProb float64, kalshiLine float64, propType string) float64 {
	if bdlProb <= 0 || bdlProb >= 1 {
		return 0
	}

	// BDL "over X" means need (X+1) or more if X is whole, or ceil(X) if half
	// e.g., "over 19.5" means need 20+, "over 20.0" means need 21+
	bdlThreshold := int(bdlLine) + 1

	distType := PropDistributionType(propType)

	if distType == "negbin" {
		// Use Negative Binomial for count props (handles overdispersion)
		// First get initial mean estimate from BDL line
		initialMean := bdlLine
		r := DefaultDispersion(propType, initialMean)

		// Infer mean from BDL market
		mu := InferNegBinMean(bdlThreshold, bdlProb, r)
		if mu <= 0 {
			return 0
		}

		// Recalculate r with inferred mean for consistency
		r = DefaultDispersion(propType, mu)

		// Calculate probability at Kalshi line
		return NegBinCDFOver(int(kalshiLine), mu, r)
	}

	if distType == "poisson" {
		// Legacy: Poisson for cases where variance ≈ mean
		lambda := InferPoissonMean(bdlThreshold, bdlProb)
		if lambda <= 0 {
			return 0
		}
		return PoissonCDFOver(int(kalshiLine), lambda)
	}

	// Normal distribution for points
	// First estimate SD, then infer mean
	// Use BDL line as initial estimate for mean to get SD
	estimatedSD := DefaultStdDev(propType, bdlLine)

	// Continuity correction for input: BDL "over 23.5" = P(X > 23.5)
	// For discrete outcomes, this means P(X >= 24)
	mean := InferNormalMean(float64(bdlThreshold)-0.5, bdlProb, estimatedSD)
	if mean <= 0 {
		return 0
	}

	// NormalCDFOver already applies continuity correction internally:
	// NormalCDFOver(k, μ, σ) = 1 - Φ((k - 0.5 - μ) / σ)
	// So pass kalshiLine directly — no external adjustment needed.
	return NormalCDFOver(kalshiLine, mean, estimatedSD)
}

// EstimateProbabilityFromMultipleLines estimates probability using multiple BDL lines
// For each BDL line, shifts its probability to Kalshi's line, then averages
// This is more accurate than averaging means because it preserves book-level information
func EstimateProbabilityFromMultipleLines(
	bdlLines []float64, // e.g., [18.5, 19.5, 20.5]
	bdlProbs []float64, // corresponding probabilities (consensus at each line)
	kalshiLine float64,
	propType string,
) float64 {
	if len(bdlLines) != len(bdlProbs) || len(bdlLines) == 0 {
		return 0
	}

	// If only one line, use single-line estimation
	if len(bdlLines) == 1 {
		return EstimateProbabilityAtLine(bdlLines[0], bdlProbs[0], kalshiLine, propType)
	}

	// CORRECT APPROACH: Shift each line's probability to Kalshi's line, then average
	// This is mathematically correct because:
	// 1. Each BDL line has a consensus probability from multiple books
	// 2. We shift each of those to Kalshi's line using the distribution
	// 3. Then average the shifted probabilities
	//
	// Example: BDL has lines at 23.5 (55%), 24.5 (48%), 25.5 (40%), Kalshi is 25
	// - Shift 23.5@55% to 25 → ~42%
	// - Shift 24.5@48% to 25 → ~45%
	// - Shift 25.5@40% to 25 → ~43%
	// - Average: (42 + 45 + 43) / 3 = 43.3%

	var shiftedProbSum float64
	var validCount int

	for i, bdlLine := range bdlLines {
		bdlProb := bdlProbs[i]

		// Skip invalid probabilities
		if bdlProb <= 0 || bdlProb >= 1 {
			continue
		}

		// Shift this line's probability to Kalshi's line
		shiftedProb := EstimateProbabilityAtLine(bdlLine, bdlProb, kalshiLine, propType)

		// Only count valid shifted probabilities
		if shiftedProb > 0 && shiftedProb < 1 {
			shiftedProbSum += shiftedProb
			validCount++
		}
	}

	if validCount == 0 {
		return 0
	}

	return shiftedProbSum / float64(validCount)
}
