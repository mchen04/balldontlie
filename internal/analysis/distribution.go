package analysis

import (
	"math"

	"sports-betting-bot/internal/mathutil"
)

// Distribution inference and probability calculation for player props
// Uses Negative Binomial for discrete counts (rebounds, assists, threes, steals, blocks)
// Uses Normal for points (higher values, approximately continuous)

func logFactorial(n int) float64 {
	if n <= 1 {
		return 0
	}
	r, _ := math.Lgamma(float64(n + 1))
	return r
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

// NegBinCDFOver calculates P(X >= k) for Negative Binomial distribution.
// Uses the regularized incomplete beta function for O(1) computation:
// P(X >= k) = 1 - I_p(r, k) where p = r/(r+mu).
func NegBinCDFOver(k int, mu, r float64) float64 {
	if mu <= 0 || r <= 0 {
		return 0
	}
	if k <= 0 {
		return 1.0
	}
	p := r / (r + mu)
	return 1 - mathutil.RegBetaI(r, float64(k), p)
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
	// steals: var ≈ 1.50*mean → r = mean/0.50 ≈ 2.0*mean
	// blocks: var ≈ 1.67*mean → r = mean/0.67 ≈ 1.5*mean
	switch propType {
	case "rebounds":
		return 3.3 * mean
	case "assists":
		return 2.5 * mean
	case "threes":
		return 2.0 * mean
	case "steals":
		return 2.0 * mean // var ≈ 1.5*mean → r = mean/0.5
	case "blocks":
		return 1.5 * mean // var ≈ 1.67*mean → r = mean/0.67
	default:
		// Default: assume variance ≈ 1.3*mean
		return 3.3 * mean
	}
}

// NormalCDF calculates the cumulative distribution function for normal distribution
func NormalCDF(x, mean, stddev float64) float64 {
	if stddev <= 0 {
		return 0
	}
	return mathutil.NormalCDF((x - mean) / stddev)
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

	z := mathutil.NormalInvCDF(1 - targetProb)
	return threshold - stddev*z
}

// DefaultStdDev returns typical standard deviation for points props
// Based on NBA player scoring variance analysis
func DefaultStdDev(propType string, inferredMean float64) float64 {
	switch propType {
	case "points":
		// NBA scoring SD is typically 30-40% of mean
		// Higher scorers have slightly lower relative variance
		if inferredMean > 25 {
			return inferredMean * 0.38
		} else if inferredMean > 15 {
			return inferredMean * 0.35
		}
		return inferredMean * 0.40
	default:
		// For other props, SD ≈ sqrt(mean) for Poisson-like data
		return math.Sqrt(inferredMean)
	}
}

// EstimateProbabilityAtLine estimates P(X >= kalshiLine) given:
// - bdlLine: the BDL line (e.g., 19.5 for "over 19.5")
// - bdlProb: the true probability from BDL for that line
// - kalshiLine: the Kalshi threshold we want probability for
// - propType: "points", "rebounds", "assists", "threes", "steals", "blocks"
func EstimateProbabilityAtLine(bdlLine float64, bdlProb float64, kalshiLine float64, propType string) float64 {
	if bdlProb <= 0 || bdlProb >= 1 {
		return 0
	}

	// BDL "over X" means need (X+1) or more if X is whole, or ceil(X) if half
	// e.g., "over 19.5" means need 20+, "over 20.0" means need 21+
	bdlThreshold := int(bdlLine) + 1

	if propType == "points" {
		// Normal distribution for points — two-pass SD estimation
		// Pass 1: use BDL line as proxy for mean
		sd1 := DefaultStdDev(propType, bdlLine)
		mean1 := InferNormalMean(float64(bdlThreshold)-0.5, bdlProb, sd1)
		if mean1 <= 0 {
			return 0
		}

		// Pass 2: refine SD using inferred mean (corrects bias at extreme probs)
		sd2 := DefaultStdDev(propType, mean1)
		mean2 := InferNormalMean(float64(bdlThreshold)-0.5, bdlProb, sd2)
		if mean2 <= 0 {
			return 0
		}

		return NormalCDFOver(kalshiLine, mean2, sd2)
	}

	// Negative Binomial for count props (handles overdispersion)
	// Two-pass estimation for dispersion parameter
	// Pass 1: initial estimate using BDL line as proxy
	r1 := DefaultDispersion(propType, bdlLine)
	mu1 := InferNegBinMean(bdlThreshold, bdlProb, r1)
	if mu1 <= 0 {
		return 0
	}

	// Pass 2: refine dispersion with inferred mean, then re-infer mu
	r2 := DefaultDispersion(propType, mu1)
	mu2 := InferNegBinMean(bdlThreshold, bdlProb, r2)
	if mu2 <= 0 {
		return 0
	}

	return NegBinCDFOver(int(kalshiLine), mu2, r2)
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

	// Shift each line's probability to Kalshi's line, then average in logit space.
	// Logit-space averaging is consistent with the log-linear opinion pool used
	// elsewhere and handles extreme probabilities more accurately than arithmetic.

	var logitSum float64
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
			logitSum += mathutil.Logit(shiftedProb)
			validCount++
		}
	}

	if validCount == 0 {
		return 0
	}

	return mathutil.Sigmoid(logitSum / float64(validCount))
}
