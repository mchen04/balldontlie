package kalshi

// TimeInForce options for orders
// Per docs: https://docs.kalshi.com/api-reference/orders/create-order
type TimeInForce string

const (
	TimeInForceGTC TimeInForce = "good_till_canceled"   // Default
	TimeInForceIOC TimeInForce = "immediate_or_cancel"  // Take available, cancel rest
	TimeInForceFOK TimeInForce = "fill_or_kill"         // Fill entire order or nothing
)

// Account & Portfolio

// BalanceResponse from GET /portfolio/balance
// Per docs: https://docs.kalshi.com/api-reference/portfolio/get-balance
type BalanceResponse struct {
	Balance        int64 `json:"balance"`         // Available balance in cents
	PortfolioValue int64 `json:"portfolio_value"` // Portfolio value in cents
	UpdatedTs      int64 `json:"updated_ts"`      // Unix timestamp of last update
}

// MarketPosition represents a single market position
// Per docs: https://docs.kalshi.com/api-reference/portfolio/get-positions
type MarketPosition struct {
	Ticker               string `json:"ticker"`
	TotalTraded          int    `json:"total_traded"`
	TotalTradedDollars   string `json:"total_traded_dollars"`   // Fixed-point 4 decimals
	Position             int    `json:"position"`               // Positive = yes, negative = no
	PositionFp           string `json:"position_fp"`            // Fixed-point 2 decimals
	MarketExposure       int    `json:"market_exposure"`        // In cents
	MarketExposureDollars string `json:"market_exposure_dollars"`
	RealizedPnl          int    `json:"realized_pnl"`           // In cents
	RealizedPnlDollars   string `json:"realized_pnl_dollars"`
	RestingOrdersCount   int    `json:"resting_orders_count"`
	FeesPaid             int    `json:"fees_paid"`              // In cents
	FeesPaidDollars      string `json:"fees_paid_dollars"`
	LastUpdatedTs        string `json:"last_updated_ts"`        // ISO datetime
}

// EventPosition represents positions aggregated by event
type EventPosition struct {
	EventTicker          string `json:"event_ticker"`
	TotalCost            int    `json:"total_cost"`
	TotalCostDollars     string `json:"total_cost_dollars"`
	TotalCostShares      int64  `json:"total_cost_shares"`
	TotalCostSharesFp    string `json:"total_cost_shares_fp"`
	EventExposure        int    `json:"event_exposure"`
	EventExposureDollars string `json:"event_exposure_dollars"`
	RealizedPnl          int    `json:"realized_pnl"`
	RealizedPnlDollars   string `json:"realized_pnl_dollars"`
	FeesPaid             int    `json:"fees_paid"`
	FeesPaidDollars      string `json:"fees_paid_dollars"`
}

// PositionsResponse from GET /portfolio/positions
type PositionsResponse struct {
	MarketPositions []MarketPosition `json:"market_positions"`
	EventPositions  []EventPosition  `json:"event_positions"`
	Cursor          string           `json:"cursor"`
}

// Order Book

// OrderBookResponse from GET /markets/{ticker}/orderbook
// Per docs: https://docs.kalshi.com/api-reference/market/get-market-orderbook
// Note: Each level is a 2-element array [price, quantity]
type OrderBookResponse struct {
	Ticker    string         `json:"-"` // Not in response, set by caller
	OrderBook OrderBookInner `json:"orderbook"`
}

// OrderBookInner is the nested orderbook structure
// Per Kalshi docs: levels are arrays [price, count], not objects
type OrderBookInner struct {
	Yes [][2]int `json:"yes"` // Bids for YES: [[price, count], ...]
	No  [][2]int `json:"no"`  // Bids for NO: [[price, count], ...]
}

// OrderBookLevel represents a single price level (for internal use after parsing)
type OrderBookLevel struct {
	Price int // Price in cents (1-99)
	Count int // Number of contracts available
}

// ParseLevels converts raw API format [][2]int to []OrderBookLevel
func ParseLevels(raw [][2]int) []OrderBookLevel {
	levels := make([]OrderBookLevel, len(raw))
	for i, level := range raw {
		levels[i] = OrderBookLevel{
			Price: level[0],
			Count: level[1],
		}
	}
	return levels
}

// Orders

// Side represents order side (yes/no)
type Side string

const (
	SideYes Side = "yes"
	SideNo  Side = "no"
)

// OrderAction represents buy/sell
type OrderAction string

const (
	ActionBuy  OrderAction = "buy"
	ActionSell OrderAction = "sell"
)

// OrderType represents order type
type OrderType string

const (
	OrderTypeMarket OrderType = "market"
	OrderTypeLimit  OrderType = "limit"
)

// CreateOrderRequest for POST /portfolio/orders
// Based on official Kalshi API docs: https://docs.kalshi.com/api-reference/orders/create-order
type CreateOrderRequest struct {
	Ticker        string      `json:"ticker"`
	ClientOrderID string      `json:"client_order_id,omitempty"`
	Side          Side        `json:"side"`
	Action        OrderAction `json:"action"`
	Count         int         `json:"count,omitempty"`    // Number of contracts (min: 1)
	Type          OrderType   `json:"type,omitempty"`     // "limit" or "market"
	YesPrice      int         `json:"yes_price,omitempty"` // Limit price for YES in cents (1-99)
	NoPrice       int         `json:"no_price,omitempty"`  // Limit price for NO in cents (1-99)
	TimeInForce   TimeInForce `json:"time_in_force,omitempty"` // gtc, ioc, or fok
	BuyMaxCost    int64       `json:"buy_max_cost,omitempty"`  // Max total cost in cents (triggers FOK)
	PostOnly      bool        `json:"post_only,omitempty"`     // Only add liquidity
	ReduceOnly    bool        `json:"reduce_only,omitempty"`   // Only reduce position
}

// OrderResponse from POST /portfolio/orders
type OrderResponse struct {
	Order Order `json:"order"`
}

// Order represents an order
// Based on official Kalshi API docs
type Order struct {
	OrderID        string      `json:"order_id"`
	ClientOrderID  string      `json:"client_order_id"`
	UserID         string      `json:"user_id"`
	Ticker         string      `json:"ticker"`
	Status         OrderStatus `json:"status"`
	Side           Side        `json:"side"`
	Action         OrderAction `json:"action"`
	Type           OrderType   `json:"type"`
	YesPrice       int         `json:"yes_price"`
	NoPrice        int         `json:"no_price"`
	CreatedTime    string      `json:"created_time"`
	ExpirationTime string      `json:"expiration_time"`
	LastUpdateTime string      `json:"last_update_time"`

	// Count fields
	InitialCount   int `json:"initial_count"`
	RemainingCount int `json:"remaining_count"`
	FillCount      int `json:"fill_count"` // Filled contracts

	// Cost/fee fields (in cents)
	TakerFees        int   `json:"taker_fees"`
	MakerFees        int   `json:"maker_fees"`
	TakerFillCost    int64 `json:"taker_fill_cost"`    // Total cost paid as taker
	MakerFillCost    int64 `json:"maker_fill_cost"`    // Total cost paid as maker
}

// AvgFillPrice calculates average fill price from fill cost and count
func (o *Order) AvgFillPrice() float64 {
	if o.FillCount == 0 {
		return 0
	}
	totalCost := o.TakerFillCost + o.MakerFillCost
	return float64(totalCost) / float64(o.FillCount)
}

// OrderStatus represents order status
type OrderStatus string

const (
	OrderStatusResting   OrderStatus = "resting"
	OrderStatusCanceled  OrderStatus = "canceled" // Note: Kalshi uses "canceled" not "cancelled"
	OrderStatusExecuted  OrderStatus = "executed"
	OrderStatusPending   OrderStatus = "pending"
)

// Exchange Status

// ExchangeStatusResponse from GET /exchange/status
type ExchangeStatusResponse struct {
	ExchangeActive             bool   `json:"exchange_active"`
	TradingActive              bool   `json:"trading_active"`
	ExchangeEstimatedResumeTime string `json:"exchange_estimated_resume_time,omitempty"`
}

// Market

// MarketResponse from GET /markets/{ticker}
type MarketResponse struct {
	Market Market `json:"market"`
}

// Market represents a Kalshi market
type Market struct {
	Ticker         string `json:"ticker"`
	EventTicker    string `json:"event_ticker"`
	Title          string `json:"title"`
	Status         string `json:"status"` // "active", "finalized", etc.
	YesBid         int    `json:"yes_bid"`
	YesAsk         int    `json:"yes_ask"`
	NoBid          int    `json:"no_bid"`
	NoAsk          int    `json:"no_ask"`
	LastPrice      int    `json:"last_price"`
	Volume         int    `json:"volume"`
	Volume24h      int    `json:"volume_24h"`
	OpenInterest   int    `json:"open_interest"`
	ExpirationTime string `json:"expiration_time"`
	CloseTime      string `json:"close_time"`
}

// Error Response

// APIError represents a Kalshi API error
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Execution Results (internal types for order execution flow)

// LiquidityCheck result from CheckLiquidity
type LiquidityCheck struct {
	Available    int     // Contracts available at acceptable slippage
	BestPrice    int     // Best available price in cents
	AveragePrice float64 // Average fill price for requested size
	TotalDepth   int     // Total contracts in order book
	Sufficient   bool    // Whether MIN_LIQUIDITY threshold is met
}

// SlippageResult from CalculateSlippage
type SlippageResult struct {
	RequestedContracts int
	FillableContracts  int
	AverageFillPrice   float64 // Average price in cents
	BestPrice          int     // Best price in cents
	SlippagePct        float64 // (avgPrice - bestPrice) / bestPrice
	Acceptable         bool    // SlippagePct <= MAX_SLIPPAGE_PCT
}

// ExecutionResult from PlaceOrder
type ExecutionResult struct {
	Success            bool
	OrderID            string
	RequestedContracts int
	FilledContracts    int
	AveragePrice       float64 // In cents
	TotalCost          int64   // In cents
	RejectionReason    string  // If not successful
}
