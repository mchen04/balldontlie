package odds

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
