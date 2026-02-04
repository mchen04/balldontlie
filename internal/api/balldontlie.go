package api

import (
	"encoding/json"
	"fmt"
	"log"
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
}

// NewBallDontLieClient creates a new API client
func NewBallDontLieClient(apiKey string) *BallDontLieClient {
	return &BallDontLieClient{
		apiKey: apiKey,
		client: NewRateLimitedClient(requestsPerMinute, requestTimeout, maxRetries),
	}
}

// OddsResponse represents the API response for odds
type OddsResponse struct {
	Data []GameOdds `json:"data"`
	Meta Meta       `json:"meta"`
}

// Meta contains pagination info
type Meta struct {
	NextCursor  int `json:"next_cursor"`
	PerPage     int `json:"per_page"`
	TotalCount  int `json:"total_count"`
}

// GameOdds contains odds for a single game
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

// GetOdds fetches NBA odds for a specific date, handling pagination
func (c *BallDontLieClient) GetOdds(date time.Time) ([]GameOdds, error) {
	dateStr := date.Format("2006-01-02")
	headers := map[string]string{
		"Authorization": c.apiKey,
	}

	var allGames []GameOdds
	cursor := 0

	for {
		url := fmt.Sprintf("%s/odds?date=%s", baseURL, dateStr)
		if cursor > 0 {
			url = fmt.Sprintf("%s&cursor=%d", url, cursor)
		}

		body, err := c.client.Get(url, headers)
		if err != nil {
			return nil, fmt.Errorf("fetching odds: %w", err)
		}

		var resp OddsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing odds response: %w", err)
		}

		allGames = append(allGames, resp.Data...)

		// Check if there are more pages
		if resp.Meta.NextCursor == 0 {
			break
		}
		cursor = resp.Meta.NextCursor
	}

	return allGames, nil
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
	Player    Player           `json:"player"`
	Vendor    string           `json:"vendor"`
	PropType  string           `json:"prop_type"` // points, rebounds, assists, threes, etc.
	Line      float64          `json:"line_value"`
	Market    PlayerPropMarket `json:"market"`
	UpdatedAt string           `json:"updated_at"`
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
