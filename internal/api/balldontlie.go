package api

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"
)

const (
	baseURL            = "https://api.balldontlie.io/v1"
	baseURLV2          = "https://api.balldontlie.io/v2"
	requestsPerMinute  = 600
	requestTimeout     = 10 * time.Second
	maxRetries         = 3
)

// BallDontLieClient handles API communication with balldontlie.io
type BallDontLieClient struct {
	apiKey string
	client *RateLimitedClient

	// Cache for game info (refreshed every 5 minutes)
	gamesCache     map[int]GameInfo
	gamesCacheDate string
	gamesCacheTime time.Time
}

// NewBallDontLieClient creates a new API client
func NewBallDontLieClient(apiKey string) *BallDontLieClient {
	return &BallDontLieClient{
		apiKey:     apiKey,
		client:     NewRateLimitedClient(requestsPerMinute, requestTimeout, maxRetries),
		gamesCache: make(map[int]GameInfo),
	}
}

// OddsResponseV2 represents the v2 API response (flat format, one record per vendor)
type OddsResponseV2 struct {
	Data []OddsRecordV2 `json:"data"`
	Meta Meta           `json:"meta"`
}

// OddsRecordV2 represents a single odds record from v2 API (flat format)
type OddsRecordV2 struct {
	ID               int     `json:"id"`
	GameID           int     `json:"game_id"`
	Vendor           string  `json:"vendor"`
	SpreadHomeValue  string  `json:"spread_home_value"`
	SpreadHomeOdds   int     `json:"spread_home_odds"`
	SpreadAwayValue  string  `json:"spread_away_value"`
	SpreadAwayOdds   int     `json:"spread_away_odds"`
	MoneylineHomeOdds *int   `json:"moneyline_home_odds"`
	MoneylineAwayOdds *int   `json:"moneyline_away_odds"`
	TotalValue       string  `json:"total_value"`
	TotalOverOdds    *int    `json:"total_over_odds"`
	TotalUnderOdds   *int    `json:"total_under_odds"`
	UpdatedAt        string  `json:"updated_at"`
}

// Meta contains pagination info
type Meta struct {
	NextCursor  int `json:"next_cursor"`
	PerPage     int `json:"per_page"`
	TotalCount  int `json:"total_count"`
}

// GameOdds contains odds for a single game (grouped format for internal use)
type GameOdds struct {
	ID        int       `json:"id"`
	GameID    int       `json:"game_id"`
	Game      Game      `json:"game"`
	Vendors   []Vendor  `json:"vendors"`
	UpdatedAt string    `json:"updated_at"`
}

// Game contains basic game info
type Game struct {
	ID            int    `json:"id"`
	Date          string `json:"date"`
	DateTime      string `json:"datetime"` // ISO 8601 timestamp (e.g., "2025-01-05T23:00:00.000Z")
	HomeTeam      Team   `json:"home_team"`
	VisitorTeam   Team   `json:"visitor_team"`
	HomeTeamScore int    `json:"home_team_score"`
	VisitorScore  int    `json:"visitor_team_score"`
	Status        string `json:"status"`
}

// GameStartsWithin checks if the game starts within the given duration
// Returns true if game is about to start (within duration) or has already started
func (g *Game) StartsWithin(d time.Duration) bool {
	if g.DateTime == "" {
		return false // Can't determine, assume safe
	}

	startTime, err := time.Parse(time.RFC3339, g.DateTime)
	if err != nil {
		// Try alternate format without milliseconds
		startTime, err = time.Parse("2006-01-02T15:04:05Z", g.DateTime)
		if err != nil {
			return false // Can't parse, assume safe
		}
	}

	timeUntilStart := time.Until(startTime)
	return timeUntilStart <= d
}

// Team represents an NBA team
type Team struct {
	ID           int    `json:"id"`
	Abbreviation string `json:"abbreviation"`
	City         string `json:"city"`
	Name         string `json:"name"`
	FullName     string `json:"full_name"`
}

// Vendor represents a sportsbook's odds
type Vendor struct {
	Name      string     `json:"name"`
	Moneyline *Moneyline `json:"moneyline,omitempty"`
	Spread    *Spread    `json:"spread,omitempty"`
	Total     *Total     `json:"total,omitempty"`
}

// Moneyline odds
type Moneyline struct {
	Home int `json:"home"`
	Away int `json:"away"`
}

// Spread odds
type Spread struct {
	HomeSpread float64 `json:"home_spread"`
	HomeOdds   int     `json:"home_odds"`
	AwaySpread float64 `json:"away_spread"`
	AwayOdds   int     `json:"away_odds"`
}

// Total (over/under) odds
type Total struct {
	Line     float64 `json:"line"`
	OverOdds int     `json:"over_odds"`
	UnderOdds int    `json:"under_odds"`
}

// GamesResponse represents the API response for games
type GamesResponse struct {
	Data []GameInfo `json:"data"`
	Meta Meta       `json:"meta"`
}

// GameInfo represents game details from the games endpoint
type GameInfo struct {
	ID              int    `json:"id"`
	Date            string `json:"date"`
	DateTime        string `json:"datetime"`
	Status          string `json:"status"`
	HomeTeam        Team   `json:"home_team"`
	VisitorTeam     Team   `json:"visitor_team"`
	HomeTeamScore   int    `json:"home_team_score"`
	VisitorTeamScore int   `json:"visitor_team_score"`
}

// GetGames fetches NBA games for a specific date
func (c *BallDontLieClient) GetGames(date time.Time) (map[int]GameInfo, error) {
	dateStr := date.Format("2006-01-02")
	headers := map[string]string{
		"Authorization": c.apiKey,
	}

	url := fmt.Sprintf("%s/games?dates[]=%s", baseURL, dateStr)
	body, err := c.client.Get(url, headers)
	if err != nil {
		return nil, fmt.Errorf("fetching games: %w", err)
	}

	var resp GamesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing games response: %w", err)
	}

	// Build map by game ID for quick lookup
	gameMap := make(map[int]GameInfo)
	for _, g := range resp.Data {
		gameMap[g.ID] = g
	}

	return gameMap, nil
}

// GetOdds fetches NBA odds for a specific date, handling pagination
// Converts v2 flat format (one record per vendor) to grouped format (one record per game)
func (c *BallDontLieClient) GetOdds(date time.Time) ([]GameOdds, error) {
	dateStr := date.Format("2006-01-02")
	headers := map[string]string{
		"Authorization": c.apiKey,
	}

	// Use cached game info if available and fresh (within 5 minutes for same date)
	games := c.gamesCache
	cacheExpired := time.Since(c.gamesCacheTime) > 5*time.Minute
	dateChanged := c.gamesCacheDate != dateStr

	if cacheExpired || dateChanged || len(games) == 0 {
		var err error
		games, err = c.GetGames(date)
		if err != nil {
			log.Printf("Warning: could not fetch game details: %v", err)
			games = make(map[int]GameInfo)
		} else {
			// Update cache
			c.gamesCache = games
			c.gamesCacheDate = dateStr
			c.gamesCacheTime = time.Now()
		}
	}

	var allRecords []OddsRecordV2
	cursor := 0

	for {
		url := fmt.Sprintf("%s/odds?dates[]=%s&per_page=100", baseURLV2, dateStr)
		if cursor > 0 {
			url = fmt.Sprintf("%s&cursor=%d", url, cursor)
		}

		body, err := c.client.Get(url, headers)
		if err != nil {
			return nil, fmt.Errorf("fetching odds: %w", err)
		}

		var resp OddsResponseV2
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing odds response: %w", err)
		}

		allRecords = append(allRecords, resp.Data...)

		// Check if there are more pages
		if resp.Meta.NextCursor == 0 {
			break
		}
		cursor = resp.Meta.NextCursor
	}

	// Group records by game_id and convert to GameOdds format
	return groupOddsByGame(allRecords, games), nil
}

// groupOddsByGame converts flat v2 records to grouped GameOdds format
func groupOddsByGame(records []OddsRecordV2, games map[int]GameInfo) []GameOdds {
	gameMap := make(map[int]*GameOdds)

	for _, rec := range records {
		game, exists := gameMap[rec.GameID]
		if !exists {
			game = &GameOdds{
				ID:        rec.ID,
				GameID:    rec.GameID,
				UpdatedAt: rec.UpdatedAt,
				Vendors:   []Vendor{},
			}
			// Populate game details if available
			if gameInfo, ok := games[rec.GameID]; ok {
				game.Game = Game{
					ID:          gameInfo.ID,
					Date:        gameInfo.Date,
					DateTime:    gameInfo.DateTime,
					Status:      gameInfo.Status,
					HomeTeam:    gameInfo.HomeTeam,
					VisitorTeam: gameInfo.VisitorTeam,
					HomeTeamScore: gameInfo.HomeTeamScore,
					VisitorScore:  gameInfo.VisitorTeamScore,
				}
			}
			gameMap[rec.GameID] = game
		}

		// Convert flat record to Vendor struct
		vendor := Vendor{
			Name: rec.Vendor,
		}

		// Parse moneyline (if available)
		if rec.MoneylineHomeOdds != nil && rec.MoneylineAwayOdds != nil {
			vendor.Moneyline = &Moneyline{
				Home: *rec.MoneylineHomeOdds,
				Away: *rec.MoneylineAwayOdds,
			}
		}

		// Parse spread (if available)
		if rec.SpreadHomeValue != "" && rec.SpreadHomeOdds != 0 {
			homeSpread, _ := strconv.ParseFloat(rec.SpreadHomeValue, 64)
			awaySpread, _ := strconv.ParseFloat(rec.SpreadAwayValue, 64)
			vendor.Spread = &Spread{
				HomeSpread: homeSpread,
				HomeOdds:   rec.SpreadHomeOdds,
				AwaySpread: awaySpread,
				AwayOdds:   rec.SpreadAwayOdds,
			}
		}

		// Parse total (if available)
		if rec.TotalValue != "" && rec.TotalOverOdds != nil && rec.TotalUnderOdds != nil {
			line, _ := strconv.ParseFloat(rec.TotalValue, 64)
			vendor.Total = &Total{
				Line:      line,
				OverOdds:  *rec.TotalOverOdds,
				UnderOdds: *rec.TotalUnderOdds,
			}
		}

		game.Vendors = append(game.Vendors, vendor)

		// Update timestamp if newer
		if rec.UpdatedAt > game.UpdatedAt {
			game.UpdatedAt = rec.UpdatedAt
		}
	}

	// Convert map to slice
	result := make([]GameOdds, 0, len(gameMap))
	for _, game := range gameMap {
		result = append(result, *game)
	}

	return result
}

// GetTodaysOdds fetches odds for today's NBA games
func (c *BallDontLieClient) GetTodaysOdds() ([]GameOdds, error) {
	return c.GetOdds(time.Now())
}

// IsKalshi checks if a vendor is Kalshi
func IsKalshi(vendorName string) bool {
	return vendorName == "Kalshi" || vendorName == "kalshi"
}

// IsSharpBook checks if a vendor is considered a sharp book for consensus
func IsSharpBook(vendorName string) bool {
	sharpBooks := map[string]bool{
		"Pinnacle":    true,
		"Circa":       true,
		"BetCRIS":     true,
		"Bookmaker":   true,
		"5Dimes":      true,
		"Heritage":    true,
	}
	return sharpBooks[vendorName]
}

// ===============================
// Player Props API (V2 endpoint)
// ===============================

// PlayerPropsResponse represents the API response for player props
type PlayerPropsResponse struct {
	Data []PlayerProp `json:"data"`
	Meta Meta         `json:"meta"`
}

// PlayerProp represents a single player prop bet
type PlayerProp struct {
	ID        int              `json:"id"`
	GameID    int              `json:"game_id"`
	PlayerID  int              `json:"player_id"`
	Vendor    string           `json:"vendor"`
	PropType  string           `json:"prop_type"` // points, rebounds, assists, threes, etc.
	LineStr   string           `json:"line_value"` // API returns string
	Market    PlayerPropMarket `json:"market"`
	UpdatedAt string           `json:"updated_at"`
}

// Line returns the line value as float64
func (p *PlayerProp) Line() float64 {
	val, _ := strconv.ParseFloat(p.LineStr, 64)
	return val
}

// Player represents a player in the API
type Player struct {
	ID        int    `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Position  string `json:"position"`
	TeamID    int    `json:"team_id"`
}

// PlayerPropMarket represents the market odds for a player prop
type PlayerPropMarket struct {
	Type      string `json:"type"` // "over_under" or "milestone"
	OverOdds  int    `json:"over_odds,omitempty"`
	UnderOdds int    `json:"under_odds,omitempty"`
	Odds      int    `json:"odds,omitempty"` // For milestone props
}

// GetPlayerProps fetches player props for a specific game
// BallDontLie V2 endpoint: GET /v2/odds/player_props?game_id=X
// Note: Player prop data is LIVE and not stored historically
func (c *BallDontLieClient) GetPlayerProps(gameID int) ([]PlayerProp, error) {
	headers := map[string]string{
		"Authorization": c.apiKey,
	}

	url := fmt.Sprintf("%s/odds/player_props?game_id=%d", baseURLV2, gameID)

	body, err := c.client.Get(url, headers)
	if err != nil {
		return nil, fmt.Errorf("fetching player props: %w", err)
	}

	var resp PlayerPropsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing player props response: %w", err)
	}

	return resp.Data, nil
}

// GetPlayerPropsFiltered fetches player props with optional filters
// propType: "points", "rebounds", "assists", "threes", etc.
// vendors: filter by specific sportsbooks
func (c *BallDontLieClient) GetPlayerPropsFiltered(gameID int, propType string, vendors []string) ([]PlayerProp, error) {
	headers := map[string]string{
		"Authorization": c.apiKey,
	}

	url := fmt.Sprintf("%s/odds/player_props?game_id=%d", baseURLV2, gameID)
	if propType != "" {
		url = fmt.Sprintf("%s&prop_type=%s", url, propType)
	}
	for _, v := range vendors {
		url = fmt.Sprintf("%s&vendors[]=%s", url, v)
	}

	body, err := c.client.Get(url, headers)
	if err != nil {
		return nil, fmt.Errorf("fetching player props: %w", err)
	}

	var resp PlayerPropsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing player props response: %w", err)
	}

	return resp.Data, nil
}

// GetTodaysPlayerProps fetches player props for all of today's games
func (c *BallDontLieClient) GetTodaysPlayerProps() (map[int][]PlayerProp, error) {
	// First get today's games
	games, err := c.GetTodaysOdds()
	if err != nil {
		return nil, fmt.Errorf("fetching today's games: %w", err)
	}

	// Fetch props for each game
	propsByGame := make(map[int][]PlayerProp)
	for _, game := range games {
		if game.Game.Status != "scheduled" && game.Game.Status != "" {
			continue // Skip games that have started
		}

		props, err := c.GetPlayerProps(game.GameID)
		if err != nil {
			log.Printf("Warning: failed to fetch player props for game %d: %v", game.GameID, err)
			continue
		}
		if len(props) > 0 {
			propsByGame[game.GameID] = props
		}
	}

	return propsByGame, nil
}

// Supported prop types that Kalshi offers (subset of what BallDontLie has)
var KalshiSupportedPropTypes = []string{
	"points",
	"rebounds",
	"assists",
	"threes",
}

// IsKalshiSupportedPropType returns true if the prop type is available on Kalshi
func IsKalshiSupportedPropType(propType string) bool {
	for _, t := range KalshiSupportedPropTypes {
		if t == propType {
			return true
		}
	}
	return false
}
