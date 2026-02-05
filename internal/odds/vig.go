package odds

import "math"

// RemoveVig removes the vig/juice from a two-way market
// Returns the true probabilities that sum to 1.0
//
// Method: Multiplicative vig removal (proportional)
// trueProbA = impliedA / (impliedA + impliedB)
// trueProbB = impliedB / (impliedA + impliedB)
func RemoveVig(impliedA, impliedB float64) (float64, float64) {
	if impliedA <= 0 || impliedB <= 0 {
		return 0, 0
	}

	total := impliedA + impliedB
	if total <= 0 {
		return 0, 0
	}

	return impliedA / total, impliedB / total
}

// RemoveVigFromAmerican converts American odds to vig-free probabilities
func RemoveVigFromAmerican(oddsA, oddsB int) (float64, float64) {
	impliedA := AmericanToImplied(oddsA)
	impliedB := AmericanToImplied(oddsB)
	return RemoveVig(impliedA, impliedB)
}

// RemoveVigPower removes vig using the Power method
// This accounts for the favorite-longshot bias: longshots are systematically overbet.
// Finds k such that p1^k + p2^k = 1, then:
// - trueProb1 = p1^k
// - trueProb2 = p2^k
// This deflates longshot probabilities more than favorites.
func RemoveVigPower(impliedA, impliedB float64) (float64, float64) {
	if impliedA <= 0 || impliedB <= 0 {
		return 0, 0
	}

	// Edge case: if probabilities already sum to 1, return as-is
	sum := impliedA + impliedB
	if math.Abs(sum-1.0) < 1e-9 {
		return impliedA, impliedB
	}

	// Find k using bisection
	k := findPowerExponent(impliedA, impliedB)

	trueA := math.Pow(impliedA, k)
	trueB := math.Pow(impliedB, k)

	return trueA, trueB
}

// findPowerExponent finds k such that p1^k + p2^k = 1 using bisection search
// For implied probabilities (0 < p < 1), higher k reduces p^k
// So for overround markets (sum > 1), k will be > 1 to reduce the sum
// For underround markets (sum < 1), k will be < 1 to increase the sum
func findPowerExponent(p1, p2 float64) float64 {
	const (
		tolerance = 1e-9
		maxIters  = 100
	)

	// Search in range [0.01, 10] - covers both overround and underround cases
	low, high := 0.01, 10.0

	for i := 0; i < maxIters; i++ {
		mid := (low + high) / 2
		currentSum := math.Pow(p1, mid) + math.Pow(p2, mid)

		if math.Abs(currentSum-1.0) < tolerance {
			return mid
		}

		// For 0 < p < 1: higher k makes p^k smaller, so sum decreases
		// If currentSum > 1, we need higher k to reduce it
		// If currentSum < 1, we need lower k to increase it
		if currentSum > 1 {
			low = mid
		} else {
			high = mid
		}
	}

	return (low + high) / 2
}

// RemoveVigPowerFromAmerican converts American odds to vig-free probabilities using Power method
func RemoveVigPowerFromAmerican(oddsA, oddsB int) (float64, float64) {
	impliedA := AmericanToImplied(oddsA)
	impliedB := AmericanToImplied(oddsB)
	return RemoveVigPower(impliedA, impliedB)
}
