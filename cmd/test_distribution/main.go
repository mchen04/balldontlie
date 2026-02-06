package main

import (
	"fmt"
	"sports-betting-bot/internal/analysis"
)

func main() {
	fmt.Println("=== DISTRIBUTION INTERPOLATION TEST ===")

	// Test 1: NegBin for rebounds
	fmt.Println("TEST 1: REBOUNDS (NegBin)")
	fmt.Println("BDL says: over 9.5 rebounds has 45% probability (need 10+)")

	// Infer mean using NegBin
	r := analysis.DefaultDispersion("rebounds", 9.5)
	mu := analysis.InferNegBinMean(10, 0.45, r)
	fmt.Printf("Inferred mean (μ): %.2f rebounds (r=%.1f)\n", mu, r)

	// Now calculate for various Kalshi lines
	kalshiLines := []int{6, 8, 10, 12, 14}
	fmt.Println("\nEstimated probabilities for Kalshi lines:")
	for _, k := range kalshiLines {
		prob := analysis.NegBinCDFOver(k, mu, r)
		fmt.Printf("  P(X >= %d) = %.1f%%\n", k, prob*100)
	}

	// Verify our inference is correct
	fmt.Printf("\nVerification: P(X >= 10) should be ~45%%: %.1f%%\n", analysis.NegBinCDFOver(10, mu, r)*100)

	// Test 2: Using the high-level function
	fmt.Println("\n====================================================")
	fmt.Println("TEST 2: HIGH-LEVEL FUNCTION (rebounds)")
	bdlLine := 9.5
	bdlProb := 0.45

	for _, kLine := range []float64{6, 8, 10, 12, 14} {
		prob := analysis.EstimateProbabilityAtLine(bdlLine, bdlProb, kLine, "rebounds")
		fmt.Printf("  Kalshi %.0f+: %.1f%%\n", kLine, prob*100)
	}

	// Test 3: Points using Normal distribution
	fmt.Println("\n====================================================")
	fmt.Println("TEST 3: POINTS (Normal)")
	fmt.Println("BDL says: over 19.5 points has 49% probability (need 20+)")

	pointsLine := 19.5
	pointsProb := 0.49

	fmt.Println("\nEstimated probabilities for Kalshi lines:")
	for _, kLine := range []float64{15, 20, 25, 30} {
		prob := analysis.EstimateProbabilityAtLine(pointsLine, pointsProb, kLine, "points")
		fmt.Printf("  Kalshi %.0f+: %.1f%%\n", kLine, prob*100)
	}

	// Test 4: Multiple lines for better accuracy
	fmt.Println("\n====================================================")
	fmt.Println("TEST 4: MULTIPLE BDL LINES (points)")
	fmt.Println("BDL has:")
	fmt.Println("  over 18.5 → 55% prob")
	fmt.Println("  over 19.5 → 49% prob")
	fmt.Println("  over 20.5 → 43% prob")

	bdlLines := []float64{18.5, 19.5, 20.5}
	bdlProbs := []float64{0.55, 0.49, 0.43}

	fmt.Println("\nEstimated probabilities for Kalshi lines:")
	for _, kLine := range []float64{15, 20, 25, 30} {
		prob := analysis.EstimateProbabilityFromMultipleLines(bdlLines, bdlProbs, kLine, "points")
		fmt.Printf("  Kalshi %.0f+: %.1f%%\n", kLine, prob*100)
	}

	// Test 5: Threes using Poisson
	fmt.Println("\n====================================================")
	fmt.Println("TEST 5: THREE-POINTERS (Poisson)")
	fmt.Println("BDL says: over 2.5 threes has 55% probability (need 3+)")

	threesLine := 2.5
	threesProb := 0.55

	fmt.Println("\nEstimated probabilities for Kalshi lines:")
	for _, kLine := range []float64{1, 2, 3, 4, 5} {
		prob := analysis.EstimateProbabilityAtLine(threesLine, threesProb, kLine, "threes")
		fmt.Printf("  Kalshi %.0f+: %.1f%%\n", kLine, prob*100)
	}

	// Test 6: Real-world scenario
	fmt.Println("\n====================================================")
	fmt.Println("TEST 6: REAL-WORLD EV CALCULATION")
	fmt.Println("\nScenario: Alperen Sengun Points")
	fmt.Println("BDL: over 19.5 at 49% true probability")
	fmt.Println("Kalshi: 25+ at $0.25 (25% implied)")

	trueProb25 := analysis.EstimateProbabilityAtLine(19.5, 0.49, 25, "points")
	kalshiPrice := 0.25

	// EV = (trueProb × profit) - ((1 - trueProb) × stake)
	ev := (trueProb25 * (1 - kalshiPrice)) - ((1 - trueProb25) * kalshiPrice)

	fmt.Printf("\nEstimated true prob for 25+: %.1f%%\n", trueProb25*100)
	fmt.Printf("Kalshi implied: %.1f%%\n", kalshiPrice*100)
	fmt.Printf("EV = (%.3f × %.2f) - (%.3f × %.2f) = %.2f%%\n",
		trueProb25, 1-kalshiPrice, 1-trueProb25, kalshiPrice, ev*100)

	if ev > 0.03 {
		fmt.Printf("→ +EV OPPORTUNITY! (%.2f%% > 3%% threshold)\n", ev*100)
	} else if ev > 0 {
		fmt.Printf("→ Slightly +EV but below threshold (%.2f%%)\n", ev*100)
	} else {
		fmt.Printf("→ Not +EV (%.2f%%)\n", ev*100)
	}
}
