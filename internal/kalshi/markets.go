package kalshi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

// MarketStatus represents the status of a Kalshi market
type MarketStatus string

const (
	MarketStatusUnopened MarketStatus = "unopened"
	MarketStatusOpen     MarketStatus = "open"
	MarketStatusPaused   MarketStatus = "paused"
	MarketStatusClosed   MarketStatus = "closed"
	MarketStatusSettled  MarketStatus = "settled"
)

// KalshiMarket represents a market from the Kalshi API
type KalshiMarket struct {
	Ticker          string  `json:"ticker"`
	EventTicker     string  `json:"event_ticker"`
	SeriesTicker    string  `json:"series_ticker,omitempty"`
	MarketType      string  `json:"market_type"` // "binary" or "scalar"
	Title           string  `json:"title"`
	Subtitle        string  `json:"subtitle,omitempty"`
	Status          string  `json:"status"`
	YesBid          int     `json:"yes_bid"`
	YesAsk          int     `json:"yes_ask"`
	NoBid           int     `json:"no_bid"`
	NoAsk           int     `json:"no_ask"`
	LastPrice       int     `json:"last_price"`
	Volume          int     `json:"volume"`
	Volume24h       int     `json:"volume_24h"`
	OpenInterest    int     `json:"open_interest"`
	CloseTime       string  `json:"close_time"`
	ExpirationTime  string  `json:"expiration_time,omitempty"`
	// Dollar-formatted fields (fixed-point 4 decimals)
	YesBidDollars   string  `json:"yes_bid_dollars,omitempty"`
	YesAskDollars   string  `json:"yes_ask_dollars,omitempty"`
	LastPriceDollars string `json:"last_price_dollars,omitempty"`
}

// ClosesWithin checks if the market closes within the given duration
func (m *KalshiMarket) ClosesWithin(d time.Duration) bool {
	if m.CloseTime == "" {
		return false // Can't determine, assume safe
	}

	closeTime, err := time.Parse(time.RFC3339, m.CloseTime)
	if err != nil {
		return false // Can't parse, assume safe
	}

	timeUntilClose := time.Until(closeTime)
	return timeUntilClose <= d
}

// MarketsResponse represents the API response for markets
type MarketsResponse struct {
	Markets []KalshiMarket `json:"markets"`
	Cursor  string         `json:"cursor,omitempty"`
}

// GetMarkets fetches markets with optional filters
// seriesTicker: filter by series (e.g., "KXNBASPREAD")
// status: filter by status (open, closed, settled, etc.)
// limit: max results per page (default 100, max 1000)
func (c *KalshiClient) GetMarkets(seriesTicker string, status MarketStatus, limit int) ([]KalshiMarket, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var allMarkets []KalshiMarket
	cursor := ""

	for {
		params := url.Values{}
		params.Set("limit", fmt.Sprintf("%d", limit))
		if seriesTicker != "" {
			params.Set("series_ticker", seriesTicker)
		}
		if status != "" {
			params.Set("status", string(status))
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}

		path := "/markets?" + params.Encode()
		body, err := c.doAuthenticatedRequest(http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("fetching markets: %w", err)
		}

		var resp MarketsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parsing markets response: %w", err)
		}

		allMarkets = append(allMarkets, resp.Markets...)

		if resp.Cursor == "" {
			break
		}
		cursor = resp.Cursor
	}

	return allMarkets, nil
}

// GetMarketByTicker fetches a specific market by its ticker
func (c *KalshiClient) GetMarketByTicker(ticker string) (*KalshiMarket, error) {
	path := fmt.Sprintf("/markets/%s", ticker)
	body, err := c.doAuthenticatedRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching market: %w", err)
	}

	var resp struct {
		Market KalshiMarket `json:"market"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing market response: %w", err)
	}

	return &resp.Market, nil
}

// GetOpenNBAMarkets fetches all open NBA game markets
func (c *KalshiClient) GetOpenNBAMarkets() (map[KalshiSeries][]KalshiMarket, error) {
	result := make(map[KalshiSeries][]KalshiMarket)

	// Fetch all game series
	for _, series := range GetAllGameSeries() {
		markets, err := c.GetMarkets(string(series), MarketStatusOpen, 200)
		if err != nil {
			log.Printf("WARN %s markets: %v", series, err)
			continue
		}
		if len(markets) > 0 {
			result[series] = markets
		}
	}

	return result, nil
}

// GetOpenPlayerPropMarkets fetches all open NBA player prop markets
func (c *KalshiClient) GetOpenPlayerPropMarkets() (map[KalshiSeries][]KalshiMarket, error) {
	result := make(map[KalshiSeries][]KalshiMarket)

	// Fetch all player prop series
	for _, series := range GetAllPlayerPropSeries() {
		markets, err := c.GetMarkets(string(series), MarketStatusOpen, 200)
		if err != nil {
			log.Printf("WARN %s prop markets: %v", series, err)
			continue
		}
		if len(markets) > 0 {
			result[series] = markets
		}
	}

	return result, nil
}

// FindMarketForGame searches for a specific market by game and series
func (c *KalshiClient) FindMarketForGame(series KalshiSeries, gameDate time.Time, awayTeam, homeTeam string) (*KalshiMarket, error) {
	// Build expected ticker
	ticker := BuildNBATicker(series, gameDate, awayTeam, homeTeam)
	if ticker == "" {
		return nil, fmt.Errorf("could not build ticker for %s @ %s", awayTeam, homeTeam)
	}

	// Try to fetch the market directly
	market, err := c.GetMarketByTicker(ticker)
	if err != nil {
		return nil, err
	}

	return market, nil
}

// MarketExists checks if a market exists and is open
func (c *KalshiClient) MarketExists(ticker string) (bool, error) {
	market, err := c.GetMarketByTicker(ticker)
	if err != nil {
		return false, nil // Market doesn't exist
	}
	return market.Status == "open" || market.Status == "active", nil
}

// GetMarketPrices returns the current bid/ask for a market
type MarketPrices struct {
	Ticker    string
	YesBid    int // Best bid for YES in cents
	YesAsk    int // Best ask for YES in cents
	NoBid     int // Best bid for NO in cents
	NoAsk     int // Best ask for NO in cents
	LastPrice int
	Volume24h int
}

func (c *KalshiClient) GetMarketPrices(ticker string) (*MarketPrices, error) {
	market, err := c.GetMarketByTicker(ticker)
	if err != nil {
		return nil, err
	}

	return &MarketPrices{
		Ticker:    market.Ticker,
		YesBid:    market.YesBid,
		YesAsk:    market.YesAsk,
		NoBid:     market.NoBid,
		NoAsk:     market.NoAsk,
		LastPrice: market.LastPrice,
		Volume24h: market.Volume24h,
	}, nil
}

// GetAllOpenNBAMarkets fetches all open NBA markets (games + props)
func (c *KalshiClient) GetAllOpenNBAMarkets() ([]KalshiMarket, error) {
	var allMarkets []KalshiMarket

	// Game markets
	gameMarkets, err := c.GetOpenNBAMarkets()
	if err != nil {
		return nil, err
	}
	for _, markets := range gameMarkets {
		allMarkets = append(allMarkets, markets...)
	}

	// Player prop markets
	propMarkets, err := c.GetOpenPlayerPropMarkets()
	if err != nil {
		return nil, err
	}
	for _, markets := range propMarkets {
		allMarkets = append(allMarkets, markets...)
	}

	return allMarkets, nil
}
