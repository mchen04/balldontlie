package kalshi

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PropType constants matching Ball Don't Lie prop types
const (
	PropTypePoints   = "points"
	PropTypeRebounds = "rebounds"
	PropTypeAssists  = "assists"
	PropTypeThrees   = "threes"
	PropTypeSteals   = "steals"
	PropTypeBlocks   = "blocks"
)

// Series tickers for each prop type
var propSeriesTickers = map[string]string{
	PropTypePoints:   "KXNBAPTS",
	PropTypeRebounds: "KXNBAREB",
	PropTypeAssists:  "KXNBAAST",
	PropTypeThrees:   "KXNBA3PT",
	PropTypeSteals:   "KXNBASTL",
	PropTypeBlocks:   "KXNBABLK",
}

// playerNicknames maps common nicknames/variations to canonical names
// Key: normalized nickname, Value: normalized canonical name
var playerNicknames = map[string]string{
	// Common nickname variations
	"moe wagner":       "moritz wagner",
	"nic claxton":      "nicolas claxton",
	"herb jones":       "herbert jones",
	"cam thomas":       "cameron thomas",
	"cam johnson":      "cameron johnson",
	"pj washington":    "p j washington",
	"cj mccollum":      "c j mccollum",
	"tj mcconnell":     "t j mcconnell",
	"aj green":         "a j green",
	"rj barrett":       "r j barrett",
	"og anunoby":       "o g anunoby",
	"jt thor":          "j t thor",
	"kt martin":        "k t martin",
	"nick smith jr":    "nick smith",
	"gary trent jr":    "gary trent",
	"jaren jackson jr": "jaren jackson",
	"tim hardaway jr":  "tim hardaway",
	"larry nance jr":   "larry nance",
	"gary payton ii":   "gary payton",
	"kel'el ware":      "kelel ware",
	"trey murphy iii":  "trey murphy",
	"lonnie walker iv": "lonnie walker",
	"wendell carter jr": "wendell carter",
	"dennis smith jr":  "dennis smith",
	"marvin bagley iii": "marvin bagley",
	"jabari smith jr":  "jabari smith",
	"marcus morris sr": "marcus morris",
	"kenyon martin jr": "kenyon martin",
	"dereck lively ii": "dereck lively",
	"scotty pippen jr": "scotty pippen",
}

// GetPlayerPropMarkets fetches all open player prop markets for a given date
// Returns markets grouped by prop type
func (c *KalshiClient) GetPlayerPropMarkets(date time.Time) (map[string][]PlayerPropMarket, error) {
	result := make(map[string][]PlayerPropMarket)
	dateStr := date.Format("06Jan02") // e.g., "26Feb04"
	dateStr = strings.ToUpper(dateStr)

	for propType, seriesTicker := range propSeriesTickers {
		markets, err := c.fetchMarketsForSeries(seriesTicker, dateStr)
		if err != nil {
			// Log but continue with other prop types
			continue
		}
		if len(markets) > 0 {
			result[propType] = markets
		}
	}

	return result, nil
}

// fetchMarketsForSeries fetches all open markets for a series ticker
func (c *KalshiClient) fetchMarketsForSeries(seriesTicker, dateStr string) ([]PlayerPropMarket, error) {
	var allMarkets []PlayerPropMarket
	cursor := ""

	for {
		path := fmt.Sprintf("/markets?series_ticker=%s&status=open&limit=100", seriesTicker)
		if cursor != "" {
			path = fmt.Sprintf("%s&cursor=%s", path, cursor)
		}

		body, err := c.doAuthenticatedRequest("GET", path, nil)
		if err != nil {
			return nil, fmt.Errorf("fetching markets: %w", err)
		}

		var resp MarketsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing markets response: %w", err)
		}

		// Parse and filter markets for the requested date
		for _, m := range resp.Markets {
			parsed := parsePlayerPropTickerFromKalshiMarket(m, seriesTicker)
			if parsed != nil && strings.Contains(m.Ticker, dateStr) {
				allMarkets = append(allMarkets, *parsed)
			}
		}

		if resp.Cursor == "" {
			break
		}
		cursor = resp.Cursor
	}

	return allMarkets, nil
}

// parsePlayerPropTickerFromKalshiMarket extracts player name, line, etc. from a Kalshi ticker
// Example: KXNBAPTS-26FEB04BOSHOU-HOUATHOMPSON1-25
// Returns nil if ticker can't be parsed
func parsePlayerPropTickerFromKalshiMarket(market KalshiMarket, seriesTicker string) *PlayerPropMarket {
	ticker := market.Ticker

	// Determine prop type from series ticker
	var propType string
	for pt, st := range propSeriesTickers {
		if st == seriesTicker {
			propType = pt
			break
		}
	}
	if propType == "" {
		return nil
	}

	// Extract parts from ticker
	// Format: KXNBAPTS-26FEB04BOSHOU-HOUATHOMPSON1-25
	parts := strings.Split(ticker, "-")
	if len(parts) < 4 {
		return nil
	}

	// Extract game date and teams from second part (26FEB04BOSHOU)
	datePart := parts[1]
	gameDate := ""
	teams := ""
	if len(datePart) >= 7 {
		gameDate = datePart[:7] // 26FEB04
		teams = datePart[7:]    // BOSHOU
	}

	// Extract line from last part
	lineStr := parts[len(parts)-1]
	line, err := strconv.ParseFloat(lineStr, 64)
	if err != nil {
		return nil
	}

	// Extract player name from title (more reliable than parsing ticker)
	// Title format: "Amen Thompson: 25+ points"
	playerName := extractPlayerNameFromTitle(market.Title)

	return &PlayerPropMarket{
		Ticker:     ticker,
		PlayerName: playerName,
		PropType:   propType,
		Line:       line,
		YesBid:     market.YesBid,
		YesAsk:     market.YesAsk,
		NoBid:      market.NoBid,
		NoAsk:      market.NoAsk,
		GameDate:   gameDate,
		Teams:      teams,
	}
}

// extractPlayerNameFromTitle extracts player name from market title
// Example: "Amen Thompson: 25+ points" -> "Amen Thompson"
func extractPlayerNameFromTitle(title string) string {
	// Split on ": " to get player name
	parts := strings.Split(title, ": ")
	if len(parts) >= 1 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

// NormalizePlayerName normalizes player names for matching
// Handles variations like "LeBron James" vs "Lebron James"
func NormalizePlayerName(name string) string {
	// Lowercase and remove extra spaces
	normalized := strings.ToLower(strings.TrimSpace(name))
	// Remove periods (J.J. vs JJ)
	normalized = strings.ReplaceAll(normalized, ".", "")
	// Remove hyphens
	normalized = strings.ReplaceAll(normalized, "-", " ")
	// Remove apostrophes (De'Aaron -> DeAaron)
	normalized = strings.ReplaceAll(normalized, "'", "")
	// Remove accents (basic - covers common cases)
	normalized = strings.ReplaceAll(normalized, "ć", "c")
	normalized = strings.ReplaceAll(normalized, "č", "c")
	normalized = strings.ReplaceAll(normalized, "š", "s")
	normalized = strings.ReplaceAll(normalized, "ž", "z")
	normalized = strings.ReplaceAll(normalized, "ö", "o")
	normalized = strings.ReplaceAll(normalized, "ü", "u")
	normalized = strings.ReplaceAll(normalized, "é", "e")
	normalized = strings.ReplaceAll(normalized, "ñ", "n")
	// Collapse multiple spaces
	re := regexp.MustCompile(`\s+`)
	normalized = re.ReplaceAllString(normalized, " ")
	return normalized
}

// resolveNickname checks if a normalized name has a known nickname mapping
// Returns the canonical name if found, otherwise returns the input
func resolveNickname(normalizedName string) string {
	if canonical, ok := playerNicknames[normalizedName]; ok {
		return canonical
	}
	// Also check reverse mapping (canonical -> nickname)
	for nickname, canonical := range playerNicknames {
		if canonical == normalizedName {
			return normalizedName // Already canonical
		}
		_ = nickname // Avoid unused variable warning
	}
	return normalizedName
}

// levenshteinDistance calculates the edit distance between two strings
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

// PlayerNamesMatch checks if two player names refer to the same player
// Uses exact match, nickname resolution, and fuzzy matching as fallbacks
func PlayerNamesMatch(name1, name2 string) bool {
	norm1 := NormalizePlayerName(name1)
	norm2 := NormalizePlayerName(name2)

	// Exact match after normalization
	if norm1 == norm2 {
		return true
	}

	// Try nickname resolution
	resolved1 := resolveNickname(norm1)
	resolved2 := resolveNickname(norm2)
	if resolved1 == resolved2 {
		return true
	}

	// Fuzzy match - allow small edit distance for typos/variations
	// Threshold: max 2 edits for names under 15 chars, max 3 for longer names
	maxDist := 2
	if len(norm1) > 15 || len(norm2) > 15 {
		maxDist = 3
	}

	dist := levenshteinDistance(resolved1, resolved2)
	if dist <= maxDist {
		return true
	}

	// Check if one name contains the other (handles missing suffixes like "Jr.")
	if strings.Contains(resolved1, resolved2) || strings.Contains(resolved2, resolved1) {
		return true
	}

	return false
}

// MatchesLine checks if a Ball Don't Lie line (e.g., 24.5) matches a Kalshi line (e.g., 25)
// For over bets: BDL "over 24.5" matches Kalshi "25+"
// For under bets: BDL "under 24.5" matches Kalshi "24+" (inverted)
func MatchesLine(bdlLine float64, kalshiLine float64, isOver bool) bool {
	if isOver {
		// "over 24.5" means scoring 25+, so it matches "25+"
		// Allow small tolerance for rounding
		expectedKalshi := float64(int(bdlLine) + 1)
		return kalshiLine == expectedKalshi || kalshiLine == bdlLine+0.5
	} else {
		// "under 24.5" means scoring 24 or less
		// Kalshi "25+" NO means scoring under 25
		// So "under 24.5" ~ "25+ NO"
		expectedKalshi := float64(int(bdlLine) + 1)
		return kalshiLine == expectedKalshi
	}
}

// FindMatchingKalshiProp finds a Kalshi market that matches a Ball Don't Lie prop
// IMPORTANT: Only matches when outcomes are mathematically equivalent:
//   - BDL "over 24.5" = need 25+ to win → matches Kalshi "25+"
//   - BDL "over 25.0" = need 26+ to win (25 is push) → matches Kalshi "26+" (NOT "25+")
func FindMatchingKalshiProp(
	playerName string,
	propType string,
	bdlLine float64,
	kalshiProps []PlayerPropMarket,
) *PlayerPropMarket {
	// Calculate the Kalshi line that represents the SAME outcome as BDL
	// BDL "over X.5" needs (X+1)+ to win → Kalshi line = X+1
	// BDL "over X.0" needs (X+1)+ to win (X is push) → Kalshi line = X+1
	// In both cases: expectedKalshiLine = ceil(bdlLine + 0.5) = int(bdlLine) + 1
	expectedKalshiLine := float64(int(bdlLine) + 1)

	for _, kp := range kalshiProps {
		if kp.PropType != propType {
			continue
		}

		// Check player name match (uses nickname resolution + fuzzy matching)
		if !PlayerNamesMatch(playerName, kp.PlayerName) {
			continue
		}

		// Only match if Kalshi line equals expected line (same mathematical outcome)
		if kp.Line == expectedKalshiLine {
			return &kp
		}
	}

	return nil
}
