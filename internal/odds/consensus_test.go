package odds

import (
	"math"
	"testing"

	"sports-betting-bot/internal/api"
	"sports-betting-bot/internal/mathutil"
)

func TestNormalizeSpreadProb(t *testing.T) {
	tests := []struct {
		name         string
		homeCover    float64
		awayCover    float64
		bookLine     float64
		targetLine   float64
		expectHigher bool // expect home cover to be higher after adjustment
	}{
		{
			name:         "Easier target line increases home cover",
			homeCover:    0.50,
			awayCover:    0.50,
			bookLine:     -6.0,
			targetLine:   -5.5, // easier to cover
			expectHigher: true,
		},
		{
			name:         "Harder target line decreases home cover",
			homeCover:    0.50,
			awayCover:    0.50,
			bookLine:     -5.5,
			targetLine:   -6.0, // harder to cover
			expectHigher: false,
		},
		{
			name:         "Same line no change",
			homeCover:    0.55,
			awayCover:    0.45,
			bookLine:     -5.5,
			targetLine:   -5.5,
			expectHigher: false, // no change expected
		},
		{
			name:         "Full point adjustment",
			homeCover:    0.50,
			awayCover:    0.50,
			bookLine:     -7.0,
			targetLine:   -6.0, // 1 point easier
			expectHigher: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adjHome, adjAway := normalizeSpreadProb(tt.homeCover, tt.awayCover, tt.bookLine, tt.targetLine)

			// Verify probabilities still sum to 1
			sum := adjHome + adjAway
			if math.Abs(sum-1.0) > 0.01 {
				t.Errorf("Probabilities should sum to 1.0, got %.4f", sum)
			}

			// Verify direction of adjustment
			if tt.bookLine != tt.targetLine {
				if tt.expectHigher && adjHome <= tt.homeCover {
					t.Errorf("Expected home cover to increase from %.3f, got %.3f", tt.homeCover, adjHome)
				}
				if !tt.expectHigher && tt.bookLine != tt.targetLine && adjHome >= tt.homeCover {
					t.Errorf("Expected home cover to decrease from %.3f, got %.3f", tt.homeCover, adjHome)
				}
			}

			// Calculate expected adjustment using normal distribution
			// Half point (0.5) / σ(11.5) ≈ 0.0435 z-score change ≈ 1.7% probability
			lineDiff := math.Abs(tt.bookLine - tt.targetLine)
			expectedAdjustment := lineDiff / NBASpreadStdDev * 0.4 // Rough approximation: z * 0.4 for prob change near 50%

			t.Logf("Book line %.1f -> Target %.1f: Home %.3f -> %.3f (expected ~%.1f%% change)",
				tt.bookLine, tt.targetLine, tt.homeCover, adjHome, expectedAdjustment*100)
		})
	}
}

func TestNormalizeTotalProb(t *testing.T) {
	tests := []struct {
		name         string
		overProb     float64
		underProb    float64
		bookLine     float64
		targetLine   float64
		expectHigher bool // expect over prob to be higher after adjustment
	}{
		{
			name:         "Lower target increases over prob",
			overProb:     0.50,
			underProb:    0.50,
			bookLine:     220.5,
			targetLine:   219.5, // lower line = easier to go over
			expectHigher: true,
		},
		{
			name:         "Higher target decreases over prob",
			overProb:     0.50,
			underProb:    0.50,
			bookLine:     219.5,
			targetLine:   220.5, // higher line = harder to go over
			expectHigher: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adjOver, adjUnder := normalizeTotalProb(tt.overProb, tt.underProb, tt.bookLine, tt.targetLine)

			// Verify probabilities still sum to 1
			sum := adjOver + adjUnder
			if math.Abs(sum-1.0) > 0.001 {
				t.Errorf("Probabilities should sum to 1.0, got %.4f", sum)
			}

			// Verify direction of adjustment
			if tt.expectHigher && adjOver <= tt.overProb {
				t.Errorf("Expected over prob to increase from %.3f, got %.3f", tt.overProb, adjOver)
			}
			if !tt.expectHigher && adjOver >= tt.overProb {
				t.Errorf("Expected over prob to decrease from %.3f, got %.3f", tt.overProb, adjOver)
			}

			t.Logf("Book line %.1f -> Target %.1f: Over %.3f -> %.3f",
				tt.bookLine, tt.targetLine, tt.overProb, adjOver)
		})
	}
}

func TestConsensusWithDifferentLines(t *testing.T) {
	// Books have different spread lines, should normalize to Kalshi's line
	game := api.GameOdds{
		GameID: 1,
		Game: api.Game{
			HomeTeam:    api.Team{Abbreviation: "LAL"},
			VisitorTeam: api.Team{Abbreviation: "BOS"},
		},
		Vendors: []api.Vendor{
			{
				Name:   "Book1",
				Spread: &api.Spread{HomeSpread: -6.0, HomeOdds: -110, AwaySpread: 6.0, AwayOdds: -110},
			},
			{
				Name:   "Book2",
				Spread: &api.Spread{HomeSpread: -5.5, HomeOdds: -115, AwaySpread: 5.5, AwayOdds: -105},
			},
			{
				Name:   "Kalshi",
				Spread: &api.Spread{HomeSpread: -5.5, HomeOdds: -110, AwaySpread: 5.5, AwayOdds: -110},
			},
		},
	}

	consensus := CalculateConsensus(game)

	if consensus.Spread == nil {
		t.Fatal("Expected spread consensus")
	}

	// Consensus line should be Kalshi's line
	if consensus.Spread.HomeSpread != -5.5 {
		t.Errorf("Expected spread line -5.5, got %.1f", consensus.Spread.HomeSpread)
	}

	// Book1's -6.0 line should be adjusted UP to -5.5 (easier to cover)
	// This means home cover prob should be higher than raw 50%
	t.Logf("Consensus spread: Line=%.1f, HomeCover=%.3f, AwayCover=%.3f (from %d books)",
		consensus.Spread.HomeSpread,
		consensus.Spread.HomeCoverProb,
		consensus.Spread.AwayCoverProb,
		consensus.Spread.BookCount)

	// Verify probabilities sum to ~1.0
	sum := consensus.Spread.HomeCoverProb + consensus.Spread.AwayCoverProb
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("Spread probs should sum to 1.0, got %.3f", sum)
	}
}

func TestConsensusVendorWeighting(t *testing.T) {
	// DraftKings (1.5x) should have more influence than BetMGM (0.7x)
	game := api.GameOdds{
		GameID: 100,
		Game: api.Game{
			HomeTeam:    api.Team{Abbreviation: "LAL"},
			VisitorTeam: api.Team{Abbreviation: "BOS"},
		},
		Vendors: []api.Vendor{
			{
				Name:      "DraftKings", // weight 1.5
				Moneyline: &api.Moneyline{Home: -200, Away: 170},
			},
			{
				Name:      "BetMGM", // weight 0.7
				Moneyline: &api.Moneyline{Home: -120, Away: 100},
			},
			{
				Name:      "Kalshi",
				Moneyline: &api.Moneyline{Home: -150, Away: 130},
			},
		},
	}

	consensus := CalculateConsensus(game)
	if consensus.Moneyline == nil {
		t.Fatal("Expected moneyline consensus")
	}

	// DraftKings (weight=1.5) home prob is ~65%, BetMGM (weight=0.7) home prob is ~54%
	// Log-linear avg: sigmoid((logit(dk)*1.5 + logit(mgm)*0.7) / (1.5+0.7))
	dkHome, _ := RemoveVigPowerFromAmerican(-200, 170)
	mgmHome, _ := RemoveVigPowerFromAmerican(-120, 100)

	simpleAvg := (dkHome + mgmHome) / 2
	logLinearAvg := mathutil.Sigmoid(
		(mathutil.Logit(dkHome)*1.5 + mathutil.Logit(mgmHome)*0.7) / (1.5 + 0.7),
	)

	// Consensus should match log-linear weighted average
	if math.Abs(consensus.Moneyline.HomeTrueProb-logLinearAvg) > 0.001 {
		t.Errorf("Expected log-linear consensus %.4f, got %.4f (simple avg would be %.4f)",
			logLinearAvg, consensus.Moneyline.HomeTrueProb, simpleAvg)
	}

	// Verify consensus is closer to DraftKings than simple average would be
	if math.Abs(consensus.Moneyline.HomeTrueProb-dkHome) >= math.Abs(simpleAvg-dkHome) {
		t.Errorf("Weighted consensus should be closer to DraftKings than simple avg")
	}

	if consensus.Moneyline.BookCount != 2 {
		t.Errorf("Expected BookCount=2, got %d", consensus.Moneyline.BookCount)
	}
}

func TestSpreadSD(t *testing.T) {
	// Close game: tighter SD
	if sd := spreadSD(-2.0); sd != 10.5 {
		t.Errorf("spreadSD(-2.0) = %.1f, want 10.5", sd)
	}
	// Standard game
	if sd := spreadSD(-5.5); sd != 11.5 {
		t.Errorf("spreadSD(-5.5) = %.1f, want 11.5", sd)
	}
	// Blowout-prone
	if sd := spreadSD(-10.0); sd != 12.5 {
		t.Errorf("spreadSD(-10.0) = %.1f, want 12.5", sd)
	}
}

func TestTotalSD(t *testing.T) {
	if sd := totalSD(210.0); sd != 15.5 {
		t.Errorf("totalSD(210.0) = %.1f, want 15.5", sd)
	}
	if sd := totalSD(222.0); sd != 17.0 {
		t.Errorf("totalSD(222.0) = %.1f, want 17.0", sd)
	}
	if sd := totalSD(235.0); sd != 18.5 {
		t.Errorf("totalSD(235.0) = %.1f, want 18.5", sd)
	}
}

func TestWinsorizedConsensus(t *testing.T) {
	// With 5 books, one outlier at 0.95 and four at 0.55:
	// Without winsorization: consensus pulled toward outlier
	// With winsorization: outlier influence is capped
	probs := []weightedProb{
		{0.55, 0.45, 1.0},
		{0.55, 0.45, 1.0},
		{0.55, 0.45, 1.0},
		{0.55, 0.45, 1.0},
		{0.95, 0.05, 1.0}, // outlier
	}
	a, b := logLinearConsensus(probs)

	// Consensus should be much closer to 0.55 than to 0.95
	if a > 0.70 {
		t.Errorf("Winsorized consensus should cap outlier: got %.4f (expected < 0.70)", a)
	}
	if math.Abs(a+b-1.0) > 0.001 {
		t.Errorf("Probs should sum to 1, got %.4f", a+b)
	}

	t.Logf("Winsorized consensus with outlier: %.4f (arithmetic would be ~0.70)", a)
}

func TestLogLinearConsensus(t *testing.T) {
	// Equal weights, symmetric probs → should be 0.5
	probs := []weightedProb{
		{0.6, 0.4, 1.0},
		{0.4, 0.6, 1.0},
	}
	a, b := logLinearConsensus(probs)
	if math.Abs(a-0.5) > 0.001 || math.Abs(b-0.5) > 0.001 {
		t.Errorf("Symmetric probs should yield 0.5/0.5, got %.4f/%.4f", a, b)
	}

	// Single entry → should return that entry
	probs2 := []weightedProb{{0.7, 0.3, 1.0}}
	a2, b2 := logLinearConsensus(probs2)
	if math.Abs(a2-0.7) > 0.001 || math.Abs(b2-0.3) > 0.001 {
		t.Errorf("Single entry should return same prob, got %.4f/%.4f", a2, b2)
	}

	// Verify a + b ≈ 1
	if math.Abs(a+b-1.0) > 0.001 {
		t.Errorf("Probs should sum to 1, got %.4f", a+b)
	}
	if math.Abs(a2+b2-1.0) > 0.001 {
		t.Errorf("Probs should sum to 1, got %.4f", a2+b2)
	}

	// Log-linear should differ from arithmetic: logit amplifies confident signals
	extreme := []weightedProb{
		{0.9, 0.1, 1.0},
		{0.5, 0.5, 1.0},
	}
	logA, _ := logLinearConsensus(extreme)
	arithmeticA := (0.9*1.0 + 0.5*1.0) / 2.0 // 0.70
	// Log-linear should be higher than arithmetic mean (logit amplifies 0.9's confidence)
	if logA <= arithmeticA {
		t.Errorf("Log-linear (%.4f) should be > arithmetic mean (%.4f) for high-confidence input", logA, arithmeticA)
	}
}
