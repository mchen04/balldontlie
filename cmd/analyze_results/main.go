// analyze_results fetches production fills & settlements from the Kalshi API,
// joins them with the local positions SQLite database, and prints a calibration
// report so we can tune the model's math parameters.
//
// Usage:
//
//	KALSHI_API_KEY_ID=... KALSHI_PRIVATE_KEY=... \
//	  go run ./cmd/analyze_results/ --db ./logs/positions_prod.db
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"sports-betting-bot/internal/kalshi"
)

// localPos mirrors columns from the positions table (all columns).
type localPos struct {
	ID         int64
	GameID     string
	HomeTeam   string
	AwayTeam   string
	MarketType string
	Side       string
	Ticker     string
	BetSide    string
	EntryPrice float64
	Contracts  int
	CreatedAt  time.Time
}

// enrichedPos joins a localPos with Kalshi settlement/open-position data.
type enrichedPos struct {
	local      localPos
	settled    bool
	win        bool
	pnlCents   int // Revenue − cost, in cents
	settlement *kalshi.Settlement
	openPos    *kalshi.MarketPosition
}

func main() {
	dbPath := flag.String("db", "", "path to SQLite positions DB (default: $DB_PATH or ./logs/positions_prod.db)")
	flag.Parse()

	if *dbPath == "" {
		if v := os.Getenv("DB_PATH"); v != "" {
			*dbPath = v
		} else {
			*dbPath = "./logs/positions_prod.db"
		}
	}

	kc, err := initKalshiClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init Kalshi client: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loading positions from %s...\n", *dbPath)
	localPositions, err := loadLocalPositions(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading DB: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d local positions.\n\n", len(localPositions))

	fmt.Println("Fetching settlements from Kalshi API...")
	settlements, err := kc.GetSettlements()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetching settlements: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Got %d settlements.\n", len(settlements))

	fmt.Println("Fetching open positions from Kalshi API...")
	openPositions, err := kc.GetPositions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetching open positions: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Got %d open positions.\n\n", len(openPositions))

	// Index by ticker for O(1) lookup.
	settleMap := make(map[string]*kalshi.Settlement, len(settlements))
	for i := range settlements {
		settleMap[settlements[i].Ticker] = &settlements[i]
	}
	openMap := make(map[string]*kalshi.MarketPosition, len(openPositions))
	for i := range openPositions {
		openMap[openPositions[i].Ticker] = &openPositions[i]
	}

	// Enrich local positions with Kalshi data.
	enriched := make([]enrichedPos, 0, len(localPositions))
	for _, lp := range localPositions {
		ep := enrichedPos{local: lp}
		if s, ok := settleMap[lp.Ticker]; ok {
			ep.settled = true
			ep.settlement = s
			switch strings.ToLower(lp.BetSide) {
			case "yes":
				ep.win = strings.EqualFold(s.MarketResult, "yes")
				ep.pnlCents = s.Revenue - s.YesTotalCost
			case "no":
				ep.win = strings.EqualFold(s.MarketResult, "no")
				ep.pnlCents = s.Revenue - s.NoTotalCost
			}
		} else if p, ok := openMap[lp.Ticker]; ok {
			ep.openPos = p
		}
		enriched = append(enriched, ep)
	}

	printReport(enriched)
}

// initKalshiClient builds a KalshiClient from environment variables.
func initKalshiClient() (*kalshi.KalshiClient, error) {
	keyID := os.Getenv("KALSHI_API_KEY_ID")
	if keyID == "" {
		return nil, fmt.Errorf("KALSHI_API_KEY_ID env var is required")
	}
	if privKey := os.Getenv("KALSHI_PRIVATE_KEY"); privKey != "" {
		return kalshi.NewKalshiClientFromKey(keyID, privKey, false)
	}
	if keyPath := os.Getenv("KALSHI_API_KEY_PATH"); keyPath != "" {
		return kalshi.NewKalshiClient(keyID, keyPath, false)
	}
	return nil, fmt.Errorf("KALSHI_PRIVATE_KEY or KALSHI_API_KEY_PATH env var is required")
}

// loadLocalPositions reads all rows from the positions table including ticker/bet_side.
func loadLocalPositions(path string) ([]localPos, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, game_id, home_team, away_team, market_type, side,
		       COALESCE(ticker, ''), COALESCE(bet_side, ''),
		       entry_price, contracts, created_at
		FROM positions
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying positions: %w", err)
	}
	defer rows.Close()

	var out []localPos
	for rows.Next() {
		var p localPos
		var createdStr string
		if err := rows.Scan(
			&p.ID, &p.GameID, &p.HomeTeam, &p.AwayTeam,
			&p.MarketType, &p.Side, &p.Ticker, &p.BetSide,
			&p.EntryPrice, &p.Contracts, &createdStr,
		); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		// SQLite stores DATETIME as string; parse best-effort.
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z"} {
			if t, err := time.Parse(layout, createdStr); err == nil {
				p.CreatedAt = t
				break
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- Report ----------------------------------------------------------------

type marketTypeStat struct {
	count    int
	wins     int
	pnlCents int
	entries  []float64 // entry prices for average
}

// priceBucket maps an entry price (0–1) to a display label.
func priceBucket(p float64) string {
	cents := int(p * 100)
	switch {
	case cents < 40:
		return "<40¢"
	case cents < 50:
		return "40-49¢"
	case cents < 60:
		return "50-59¢"
	case cents < 70:
		return "60-69¢"
	default:
		return "70+¢"
	}
}

// bucketExpected returns the midpoint probability for a given bucket label,
// used as the naïve "expected" win rate (Kalshi price = market probability).
func bucketExpected(label string) float64 {
	switch label {
	case "<40¢":
		return 0.33
	case "40-49¢":
		return 0.44
	case "50-59¢":
		return 0.54
	case "60-69¢":
		return 0.63
	case "70+¢":
		return 0.72
	}
	return 0.5
}

func printReport(positions []enrichedPos) {
	settled := make([]enrichedPos, 0)
	open := make([]enrichedPos, 0)
	noTicker := make([]enrichedPos, 0)

	for _, ep := range positions {
		if ep.local.Ticker == "" {
			noTicker = append(noTicker, ep)
		} else if ep.settled {
			settled = append(settled, ep)
		} else {
			open = append(open, ep)
		}
	}

	fmt.Println()
	fmt.Println("=== RESULTS SUMMARY ===")
	fmt.Printf("Total positions:  %d\n", len(positions))
	fmt.Printf("Settled:          %d\n", len(settled))
	fmt.Printf("Open (unsettled): %d\n", len(open))
	if len(noTicker) > 0 {
		fmt.Printf("No ticker:        %d  (legacy rows without ticker; skipped)\n", len(noTicker))
	}

	if len(settled) == 0 {
		fmt.Println("\nNo settled positions to analyze yet.")
		printOpenSummary(open)
		return
	}

	// --- By market type ---
	fmt.Println()
	fmt.Println("=== BY MARKET TYPE (settled positions only) ===")
	fmt.Printf("%-20s | %5s | %4s | %5s | %9s | %12s\n",
		"market_type", "count", "wins", "win%", "avg_entry", "total_pnl_$")
	fmt.Println(strings.Repeat("-", 72))

	typeStats := make(map[string]*marketTypeStat)
	for _, ep := range settled {
		mt := ep.local.MarketType
		if typeStats[mt] == nil {
			typeStats[mt] = &marketTypeStat{}
		}
		s := typeStats[mt]
		s.count++
		s.pnlCents += ep.pnlCents
		s.entries = append(s.entries, ep.local.EntryPrice)
		if ep.win {
			s.wins++
		}
	}

	// Print sorted by market type name.
	mtKeys := make([]string, 0, len(typeStats))
	for k := range typeStats {
		mtKeys = append(mtKeys, k)
	}
	sort.Strings(mtKeys)

	for _, mt := range mtKeys {
		s := typeStats[mt]
		winPct := 0.0
		if s.count > 0 {
			winPct = float64(s.wins) / float64(s.count) * 100
		}
		avgEntry := mean(s.entries)
		pnlDollars := float64(s.pnlCents) / 100.0
		pnlStr := fmt.Sprintf("%+.2f", pnlDollars)
		fmt.Printf("%-20s | %5d | %4d | %4.0f%% | %9.2f | %12s\n",
			mt, s.count, s.wins, winPct, avgEntry, "$"+pnlStr)
	}

	// --- Overall settled P&L ---
	totalPnl := 0
	totalWins := 0
	for _, ep := range settled {
		totalPnl += ep.pnlCents
		if ep.win {
			totalWins++
		}
	}
	fmt.Printf("\n%-20s | %5d | %4d | %4.0f%% | %9s | %12s\n",
		"TOTAL", len(settled), totalWins,
		float64(totalWins)/float64(len(settled))*100,
		"",
		"$"+fmt.Sprintf("%+.2f", float64(totalPnl)/100.0))

	// --- Calibration ---
	fmt.Println()
	fmt.Println("=== CALIBRATION (entry price → actual win rate) ===")
	fmt.Printf("%-8s | %4s | %8s | %10s\n", "bucket", "N", "actual%", "expected%")
	fmt.Println(strings.Repeat("-", 40))

	bucketOrder := []string{"<40¢", "40-49¢", "50-59¢", "60-69¢", "70+¢"}
	bucketStats := make(map[string][2]int) // [wins, total]
	for _, ep := range settled {
		b := priceBucket(ep.local.EntryPrice)
		st := bucketStats[b]
		st[1]++
		if ep.win {
			st[0]++
		}
		bucketStats[b] = st
	}

	type bucketSignal struct {
		label  string
		actual float64
		exp    float64
		n      int
	}
	var signals []bucketSignal

	for _, b := range bucketOrder {
		st, ok := bucketStats[b]
		if !ok {
			continue
		}
		actual := float64(st[0]) / float64(st[1]) * 100
		exp := bucketExpected(b) * 100
		marker := ""
		diff := actual - exp
		switch {
		case math.Abs(diff) <= 5:
			marker = "✓"
		case diff < -5:
			marker = "under"
		case diff > 5:
			marker = "over"
		}
		fmt.Printf("%-8s | %4d | %7.0f%% | %9.0f%%  %s\n", b, st[1], actual, exp, marker)
		if st[1] >= 3 { // only flag buckets with enough data
			signals = append(signals, bucketSignal{b, actual / 100, exp / 100, st[1]})
		}
	}

	// --- Tuning signals ---
	fmt.Println()
	fmt.Println("=== TUNING SIGNALS ===")
	hasSignal := false
	for _, sig := range signals {
		diff := sig.actual - sig.exp
		if math.Abs(diff) <= 0.05 {
			continue
		}
		hasSignal = true
		if diff < 0 {
			fmt.Printf("→ %s bucket underperforms (actual %.0f%% vs expected %.0f%%): model may be overconfident\n",
				sig.label, sig.actual*100, sig.exp*100)
			fmt.Printf("  Consider: raise ShrinkToward exponent or scaled EV threshold for this bucket\n")
		} else {
			fmt.Printf("→ %s bucket overperforms (actual %.0f%% vs expected %.0f%%): model is well-calibrated or conservative\n",
				sig.label, sig.actual*100, sig.exp*100)
		}
	}
	if !hasSignal {
		fmt.Println("→ All buckets within 5pp of expected — model appears well-calibrated.")
	}

	// Overall ROI
	totalCost := 0.0
	for _, ep := range settled {
		totalCost += ep.local.EntryPrice * float64(ep.local.Contracts) * 100 // in cents
	}
	if totalCost > 0 {
		roi := float64(totalPnl) / totalCost * 100
		fmt.Printf("\nOverall ROI on settled bets: %+.1f%%  (pnl $%.2f / cost $%.2f)\n",
			roi, float64(totalPnl)/100.0, totalCost/100.0)
	}

	printOpenSummary(open)
}

func printOpenSummary(open []enrichedPos) {
	if len(open) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("=== OPEN POSITIONS (not yet settled) ===")
	fmt.Printf("%-45s | %8s | %5s | %9s | %10s\n",
		"ticker", "bet_side", "contr", "entry_¢", "unreal_pnl")
	fmt.Println(strings.Repeat("-", 85))
	for _, ep := range open {
		unrealStr := "n/a"
		if ep.openPos != nil {
			unrealStr = fmt.Sprintf("$%.2f", float64(ep.openPos.RealizedPnl)/100.0)
		}
		fmt.Printf("%-45s | %8s | %5d | %9.2f | %10s\n",
			ep.local.Ticker, ep.local.BetSide, ep.local.Contracts,
			ep.local.EntryPrice*100, unrealStr)
	}
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
