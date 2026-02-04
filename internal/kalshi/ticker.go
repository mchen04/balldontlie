package kalshi

import (
	"fmt"
	"strings"
	"time"
)

// KalshiSeries represents the different NBA market series on Kalshi
// Based on official Kalshi market structure as of 2026
type KalshiSeries string

const (
	// Game-level markets
	SeriesMoneyline KalshiSeries = "KXNBAGAME"   // Game winner (moneyline)
	SeriesSpread    KalshiSeries = "KXNBASPREAD" // Point spread
	SeriesTotal     KalshiSeries = "KXNBATOTAL"  // Total points (over/under)

	// Player prop markets
	SeriesPlayerPoints   KalshiSeries = "KXNBAPTS" // Player points
	SeriesPlayerRebounds KalshiSeries = "KXNBAREB" // Player rebounds
	SeriesPlayerAssists  KalshiSeries = "KXNBAAST" // Player assists
	SeriesPlayerThrees   KalshiSeries = "KXNBA3PT" // Player three-pointers made

	// Futures markets
	SeriesChampionship     KalshiSeries = "KXNBA"       // NBA Championship
	SeriesWesternConf      KalshiSeries = "KXNBAWEST"   // Western Conference Champion
	SeriesEasternConf      KalshiSeries = "KXNBAEAST"   // Eastern Conference Champion
	SeriesPlayoffSeries    KalshiSeries = "KXNBASERIES" // Playoff series winner
	SeriesTradeDeadline    KalshiSeries = "KXNBATRADE"  // Trade deadline markets
	SeriesPlayerNextTeam   KalshiSeries = "KXNEXTTEAMNBA" // Player next team

	// Season leader markets
	SeriesLeaderPoints   KalshiSeries = "KXLEADERNBAPPG" // Points per game leader
	SeriesLeaderRebounds KalshiSeries = "KXLEADERNBARPG" // Rebounds per game leader
	SeriesLeaderAssists  KalshiSeries = "KXLEADERNBAAPG" // Assists per game leader
	SeriesLeaderBlocks   KalshiSeries = "KXLEADERNBABLK" // Blocks per game leader
)

// PropType represents the type of player prop
type PropType string

const (
	PropPoints   PropType = "points"
	PropRebounds PropType = "rebounds"
	PropAssists  PropType = "assists"
	PropThrees   PropType = "threes"
)

// MarketType represents the type of game market
type MarketType string

const (
	MarketMoneyline MarketType = "moneyline"
	MarketSpread    MarketType = "spread"
	MarketTotal     MarketType = "total"
)

// BuildNBATicker builds a Kalshi ticker for an NBA game market
// Kalshi NBA tickers follow this format:
// - KXNBAGAME-26FEB04MEMSAC (moneyline: Memphis @ Sacramento on Feb 4, 2026)
// - KXNBASPREAD-26FEB04MEMSAC (spread: Memphis @ Sacramento on Feb 4, 2026)
// - KXNBATOTAL-26FEB04NOPMIL (total: New Orleans @ Milwaukee on Feb 4, 2026)
func BuildNBATicker(series KalshiSeries, gameDate time.Time, awayTeam, homeTeam string) string {
	// Format date as YYMONDD (e.g., "26FEB04")
	dateStr := FormatKalshiDate(gameDate)

	// Map team abbreviations to Kalshi format
	away := MapTeamToKalshi(awayTeam)
	home := MapTeamToKalshi(homeTeam)

	if away == "" || home == "" {
		return ""
	}

	// Build ticker: SERIES-YYMONDDDAWAYTEAMHOMETEAM
	return fmt.Sprintf("%s-%s%s%s", series, dateStr, away, home)
}

// BuildPlayerPropTicker builds a Kalshi ticker for a player prop market
// Player prop tickers follow this format:
// - KXNBAPTS-26FEB03PHIGSW (points props for Philadelphia @ Golden State)
// - KXNBAREB-26FEB03LALBKN (rebounds props for Lakers @ Nets)
func BuildPlayerPropTicker(propType PropType, gameDate time.Time, awayTeam, homeTeam string) string {
	series := GetSeriesForPropType(propType)
	if series == "" {
		return ""
	}
	return BuildNBATicker(series, gameDate, awayTeam, homeTeam)
}

// FormatKalshiDate formats a date for Kalshi ticker (YYMONDD format)
func FormatKalshiDate(t time.Time) string {
	return fmt.Sprintf("%02d%s%02d",
		t.Year()%100,
		strings.ToUpper(t.Format("Jan")),
		t.Day())
}

// GetSeriesForMarketType returns the Kalshi series for a given game market type
func GetSeriesForMarketType(marketType string) KalshiSeries {
	switch marketType {
	case "moneyline":
		return SeriesMoneyline
	case "spread":
		return SeriesSpread
	case "total":
		return SeriesTotal
	default:
		return ""
	}
}

// GetSeriesForPropType returns the Kalshi series for a given player prop type
func GetSeriesForPropType(propType PropType) KalshiSeries {
	switch propType {
	case PropPoints:
		return SeriesPlayerPoints
	case PropRebounds:
		return SeriesPlayerRebounds
	case PropAssists:
		return SeriesPlayerAssists
	case PropThrees:
		return SeriesPlayerThrees
	default:
		return ""
	}
}

// PropTypeFromBallDontLie converts BallDontLie prop type to our PropType
// BallDontLie prop types: points, rebounds, assists, threes, blocks, steals,
// double_double, triple_double, points_rebounds, points_assists, etc.
func PropTypeFromBallDontLie(bdlType string) PropType {
	switch bdlType {
	case "points":
		return PropPoints
	case "rebounds":
		return PropRebounds
	case "assists":
		return PropAssists
	case "threes":
		return PropThrees
	default:
		// Kalshi only supports points, rebounds, assists, threes
		return ""
	}
}

// IsKalshiSupportedProp returns true if the prop type is supported on Kalshi
func IsKalshiSupportedProp(bdlType string) bool {
	return PropTypeFromBallDontLie(bdlType) != ""
}

// GetAllGameSeries returns all game-level series
func GetAllGameSeries() []KalshiSeries {
	return []KalshiSeries{
		SeriesMoneyline,
		SeriesSpread,
		SeriesTotal,
	}
}

// GetAllPlayerPropSeries returns all player prop series
func GetAllPlayerPropSeries() []KalshiSeries {
	return []KalshiSeries{
		SeriesPlayerPoints,
		SeriesPlayerRebounds,
		SeriesPlayerAssists,
		SeriesPlayerThrees,
	}
}

// MapTeamToKalshi converts standard NBA team abbreviations to Kalshi's format
// Most are the same, but handles edge cases
func MapTeamToKalshi(abbrev string) string {
	// Kalshi uses 3-letter codes, mostly matching standard NBA abbreviations
	kalshiMap := map[string]string{
		// Standard abbreviations (BallDontLie → Kalshi)
		"ATL": "ATL", // Atlanta Hawks
		"BOS": "BOS", // Boston Celtics
		"BKN": "BKN", // Brooklyn Nets
		"CHA": "CHA", // Charlotte Hornets
		"CHI": "CHI", // Chicago Bulls
		"CLE": "CLE", // Cleveland Cavaliers
		"DAL": "DAL", // Dallas Mavericks
		"DEN": "DEN", // Denver Nuggets
		"DET": "DET", // Detroit Pistons
		"GSW": "GSW", // Golden State Warriors
		"HOU": "HOU", // Houston Rockets
		"IND": "IND", // Indiana Pacers
		"LAC": "LAC", // Los Angeles Clippers
		"LAL": "LAL", // Los Angeles Lakers
		"MEM": "MEM", // Memphis Grizzlies
		"MIA": "MIA", // Miami Heat
		"MIL": "MIL", // Milwaukee Bucks
		"MIN": "MIN", // Minnesota Timberwolves
		"NOP": "NOP", // New Orleans Pelicans
		"NYK": "NYK", // New York Knicks
		"OKC": "OKC", // Oklahoma City Thunder
		"ORL": "ORL", // Orlando Magic
		"PHI": "PHI", // Philadelphia 76ers
		"PHX": "PHX", // Phoenix Suns
		"POR": "POR", // Portland Trail Blazers
		"SAC": "SAC", // Sacramento Kings
		"SAS": "SAS", // San Antonio Spurs
		"TOR": "TOR", // Toronto Raptors
		"UTA": "UTA", // Utah Jazz
		"WAS": "WAS", // Washington Wizards

		// Alternate abbreviations that might appear
		"NO":  "NOP", // New Orleans alternate
		"NY":  "NYK", // New York alternate
		"GS":  "GSW", // Golden State alternate
		"SA":  "SAS", // San Antonio alternate
		"PHO": "PHX", // Phoenix alternate
		"BRK": "BKN", // Brooklyn alternate
	}

	if mapped, ok := kalshiMap[abbrev]; ok {
		return mapped
	}

	// If not in map, return as-is if it's a 3-letter code
	if len(abbrev) == 3 {
		return abbrev
	}

	return ""
}

// IsMaintenanceWindow returns true if the given time falls within
// Kalshi's scheduled maintenance window (Thursday 3:00-5:00 AM ET)
func IsMaintenanceWindow(t time.Time) bool {
	et, err := time.LoadLocation("America/New_York")
	if err != nil {
		// If we can't load timezone, assume not in maintenance
		return false
	}

	tET := t.In(et)

	// Maintenance is Thursday 3:00-5:00 AM ET
	return tET.Weekday() == time.Thursday && tET.Hour() >= 3 && tET.Hour() < 5
}

// IsMaintenanceWindowNow returns true if the current time is in maintenance
func IsMaintenanceWindowNow() bool {
	return IsMaintenanceWindow(time.Now())
}
