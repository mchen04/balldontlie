package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	"sports-betting-bot/internal/api"
)

func americanToImplied(odds int) float64 {
	if odds == 0 {
		return 0
	}
	if odds > 0 {
		return 100.0 / (float64(odds) + 100.0)
	}
	return float64(-odds) / (float64(-odds) + 100.0)
}

func removeVigPower(impliedA, impliedB float64) (float64, float64) {
	if impliedA <= 0 || impliedB <= 0 {
		return 0, 0
	}
	sum := impliedA + impliedB
	if math.Abs(sum-1.0) < 1e-9 {
		return impliedA, impliedB
	}

	// Find k using bisection
	low, high := 0.01, 10.0
	for i := 0; i < 100; i++ {
		mid := (low + high) / 2
		currentSum := math.Pow(impliedA, mid) + math.Pow(impliedB, mid)
		if math.Abs(currentSum-1.0) < 1e-9 {
			low = mid
			high = mid
			break
		}
		if currentSum > 1 {
			low = mid
		} else {
			high = mid
		}
	}
	k := (low + high) / 2
	return math.Pow(impliedA, k), math.Pow(impliedB, k)
}

func main() {
	_ = godotenv.Load()
	bdlClient := api.NewBallDontLieClient(os.Getenv("BALLDONTLIE_API_KEY"))

	// Get today's games
	gameOdds, _ := bdlClient.GetTodaysOdds()

	// Find PHI @ LAL game (Austin Reaves plays for LAL)
	var lalGameID int
	for _, game := range gameOdds {
		if game.Game.HomeTeam.Abbreviation == "LAL" {
			lalGameID = game.GameID
			fmt.Printf("Found game: %s @ %s (ID: %d)\n\n",
				game.Game.VisitorTeam.Abbreviation,
				game.Game.HomeTeam.Abbreviation,
				game.GameID)
			break
		}
	}

	if lalGameID == 0 {
		log.Fatal("LAL game not found")
	}

	// Get player props for this game
	props, err := bdlClient.GetPlayerProps(lalGameID)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Find all unique player IDs for points props
	playerIDs := make(map[int]bool)
	for _, prop := range props {
		if prop.PropType == "points" {
			playerIDs[prop.PlayerID] = true
		}
	}
	ids := make([]int, 0, len(playerIDs))
	for id := range playerIDs {
		ids = append(ids, id)
	}
	playerNames := bdlClient.GetPlayerNames(ids)

	// Find Austin Reaves player ID
	var reavesID int
	for id, name := range playerNames {
		if strings.Contains(strings.ToLower(name), "reaves") {
			reavesID = id
			fmt.Printf("Found Austin Reaves: ID=%d, Name=%s\n\n", id, name)
			break
		}
	}

	if reavesID == 0 {
		log.Fatal("Austin Reaves not found")
	}

	// Collect ALL data for Austin Reaves points props
	type lineData struct {
		Line      float64
		Vendor    string
		OverOdds  int
		UnderOdds int
		OverTrue  float64
	}

	var allData []lineData

	// Also show Kalshi data
	fmt.Println("=== KALSHI DATA FOR AUSTIN REAVES ===")
	for _, prop := range props {
		if prop.PlayerID != reavesID || prop.PropType != "points" {
			continue
		}
		if api.IsKalshi(prop.Vendor) {
			fmt.Printf("Kalshi: Line=%.1f, Over=%d, Under=%d\n",
				prop.Line(), int(prop.Market.OverOdds), int(prop.Market.UnderOdds))
		}
	}
	fmt.Println()

	for _, prop := range props {
		if prop.PlayerID != reavesID || prop.PropType != "points" {
			continue
		}
		if prop.Market.Type != "over_under" || prop.Market.OverOdds == 0 {
			continue
		}
		if api.IsKalshi(prop.Vendor) {
			continue
		}

		overImplied := americanToImplied(int(prop.Market.OverOdds))
		underImplied := americanToImplied(int(prop.Market.UnderOdds))
		overTrue, _ := removeVigPower(overImplied, underImplied)

		allData = append(allData, lineData{
			Line:      prop.Line(),
			Vendor:    prop.Vendor,
			OverOdds:  int(prop.Market.OverOdds),
			UnderOdds: int(prop.Market.UnderOdds),
			OverTrue:  overTrue,
		})
	}

	// Sort by line
	sort.Slice(allData, func(i, j int) bool {
		return allData[i].Line < allData[j].Line
	})

	fmt.Println("=== ALL AUSTIN REAVES POINTS DATA FROM SPORTSBOOKS ===")
	fmt.Printf("%-15s %8s %8s %8s | %10s\n", "SPORTSBOOK", "LINE", "OVER", "UNDER", "TRUE_OVER%")
	fmt.Println(strings.Repeat("-", 60))

	// Group by line and calculate consensus per line
	lineConsensus := make(map[float64][]float64)
	for _, d := range allData {
		fmt.Printf("%-15s %8.1f %8d %8d | %9.2f%%\n",
			d.Vendor, d.Line, d.OverOdds, d.UnderOdds, d.OverTrue*100)
		lineConsensus[d.Line] = append(lineConsensus[d.Line], d.OverTrue)
	}

	fmt.Println()
	fmt.Println("=== CONSENSUS BY LINE (averaged across books) ===")
	fmt.Printf("%8s | %10s | %s\n", "LINE", "TRUE_OVER%", "BOOKS")
	fmt.Println(strings.Repeat("-", 40))

	// Get sorted lines
	var lines []float64
	for line := range lineConsensus {
		lines = append(lines, line)
	}
	sort.Float64s(lines)

	type consensusPoint struct {
		Line     float64
		OverProb float64
		Books    int
	}
	var points []consensusPoint

	for _, line := range lines {
		probs := lineConsensus[line]
		var sum float64
		for _, p := range probs {
			sum += p
		}
		avg := sum / float64(len(probs))
		points = append(points, consensusPoint{line, avg, len(probs)})
		fmt.Printf("%8.1f | %9.2f%% | %d\n", line, avg*100, len(probs))
	}

	fmt.Println()
	fmt.Println("=== HOW INTERPOLATION WORKS FOR LINE 10.0 ===")
	fmt.Println()
	fmt.Println("Kalshi offers OVER 10 points. BallDontLie has data at other lines.")
	fmt.Println("The system fits a Normal distribution to estimate P(X >= 10).")
	fmt.Println()

	// Do the actual interpolation calculation
	fmt.Println("STEP-BY-STEP CALCULATION:")
	fmt.Println()

	kalshiLine := 10.0

	for _, p := range points {
		if p.Books < 2 {
			continue // Skip low-confidence points
		}

		bdlLine := p.Line
		bdlProb := p.OverProb

		// Step 1: Estimate standard deviation (varies by line)
		// DefaultStdDev: >25 → 32%, >15 → 30%, else → 35%
		var estimatedSD float64
		var sdPct float64
		if bdlLine > 25 {
			sdPct = 0.32
		} else if bdlLine > 15 {
			sdPct = 0.30
		} else {
			sdPct = 0.35
		}
		estimatedSD = bdlLine * sdPct
		fmt.Printf("From BDL line %.1f (%.2f%% over, %d books):\n", bdlLine, bdlProb*100, p.Books)
		fmt.Printf("  1. Estimate SD = %.1f × %.2f = %.2f\n", bdlLine, sdPct, estimatedSD)

		// Step 2: Infer the mean
		// BDL "over 15.5" means P(X >= 16)
		bdlThreshold := int(bdlLine) + 1
		fmt.Printf("  2. BDL 'over %.1f' means P(X >= %d) = %.2f%%\n", bdlLine, bdlThreshold, bdlProb*100)

		// Using inverse normal CDF: find μ such that P(X >= threshold) = bdlProb
		// P(X >= t) = bdlProb  →  Φ((t-0.5-μ)/σ) = 1 - bdlProb
		// z = InverseNormalCDF(1 - bdlProb)
		// μ = (t - 0.5) - σ*z
		z := inverseNormalCDF(1 - bdlProb)
		mean := (float64(bdlThreshold) - 0.5) - estimatedSD*z
		fmt.Printf("  3. Infer mean: z = Φ⁻¹(%.4f) = %.4f\n", 1-bdlProb, z)
		fmt.Printf("     μ = (%.1f - 0.5) - %.2f × %.4f = %.2f points\n",
			float64(bdlThreshold), estimatedSD, z, mean)

		// Step 3: Calculate P(X >= kalshiLine) using inferred mean
		// P(X >= 10) with continuity correction = P(X > 9.5)
		zScore := (kalshiLine - 0.5 - mean) / estimatedSD
		probOver := 1 - normalCDF(zScore)
		fmt.Printf("  4. P(X >= %.0f) = P(X > %.1f) = 1 - Φ((%.1f - %.2f)/%.2f)\n",
			kalshiLine, kalshiLine-0.5, kalshiLine-0.5, mean, estimatedSD)
		fmt.Printf("     = 1 - Φ(%.4f) = 1 - %.4f = %.2f%%\n", zScore, normalCDF(zScore), probOver*100)
		fmt.Println()
	}

	// Now calculate the actual consensus
	var shiftedProbs []float64
	for _, p := range points {
		if p.Books < 2 {
			continue
		}
		bdlLine := p.Line
		bdlProb := p.OverProb
		estimatedSD := bdlLine * 0.30
		bdlThreshold := int(bdlLine) + 1
		z := inverseNormalCDF(1 - bdlProb)
		mean := (float64(bdlThreshold) - 0.5) - estimatedSD*z
		zScore := (kalshiLine - 0.5 - mean) / estimatedSD
		probOver := 1 - normalCDF(zScore)
		shiftedProbs = append(shiftedProbs, probOver)
	}

	if len(shiftedProbs) > 0 {
		var sum float64
		for _, p := range shiftedProbs {
			sum += p
		}
		avg := sum / float64(len(shiftedProbs))
		fmt.Println("FINAL CONSENSUS:")
		fmt.Printf("  Average of %d shifted probabilities = %.2f%%\n", len(shiftedProbs), avg*100)
		fmt.Println()
		fmt.Println("This is the TRUE PROBABILITY used in the EV calculation!")
	}
}

// Helper functions for the calculation demonstration
func normalCDF(z float64) float64 {
	return 0.5 * (1 + math.Erf(z/math.Sqrt(2)))
}

func inverseNormalCDF(p float64) float64 {
	if p <= 0 || p >= 1 {
		return 0
	}
	if p > 0.5 {
		return -inverseNormalCDF(1 - p)
	}
	t := math.Sqrt(-2 * math.Log(p))
	c0, c1, c2 := 2.515517, 0.802853, 0.010328
	d1, d2, d3 := 1.432788, 0.189269, 0.001308
	absZ := t - (c0+c1*t+c2*t*t)/(1+d1*t+d2*t*t+d3*t*t*t)
	return -absZ
}

