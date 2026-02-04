package kalshi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// OrderConfig holds configuration for order execution
type OrderConfig struct {
	MaxSlippagePct        float64 // Maximum acceptable slippage (default 0.02 = 2%)
	MinLiquidityContracts int     // Minimum depth required (default 10)
	DryRun                bool    // If true, don't actually place orders

	// EV verification at execution time
	TrueProb    float64 // Consensus probability (0-1), set to 0 to skip EV check
	EVThreshold float64 // Minimum adjusted EV required (e.g., 0.03 = 3%)
	FeePct      float64 // Kalshi fee percentage (e.g., 0.012 = 1.2%)
}

// DefaultOrderConfig returns sensible defaults
func DefaultOrderConfig() OrderConfig {
	return OrderConfig{
		MaxSlippagePct:        MaxSlippagePct,
		MinLiquidityContracts: MinLiquidityContracts,
		DryRun:                false,
	}
}

// PlaceOrder executes an order with full safeguards:
// 1. Check exchange status
// 2. Fetch order book, verify liquidity
// 3. Calculate slippage
// 4. Place order if all checks pass
func (c *KalshiClient) PlaceOrder(
	ticker string,
	side Side,
	action OrderAction,
	contracts int,
	config OrderConfig,
) (*ExecutionResult, error) {
	// Step 1: Check exchange status
	tradingActive, err := c.IsTradingActive()
	if err != nil {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    fmt.Sprintf("failed to check exchange status: %v", err),
		}, nil
	}
	if !tradingActive {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    "exchange is not active for trading",
		}, nil
	}

	// Step 2: Check market is not about to close
	market, err := c.GetMarketByTicker(ticker)
	if err != nil {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    fmt.Sprintf("failed to fetch market info: %v", err),
		}, nil
	}
	if market.ClosesWithin(10 * time.Second) {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    "market closes within 10 seconds, skipping",
		}, nil
	}
	if market.Status != "open" && market.Status != "active" {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    fmt.Sprintf("market status is %s, not open", market.Status),
		}, nil
	}

	// Step 3: Fetch order book
	book, err := c.GetOrderBook(ticker)
	if err != nil {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    fmt.Sprintf("failed to fetch order book: %v", err),
		}, nil
	}

	// Step 4: Check liquidity
	liquidity := c.CheckLiquidity(book, side, action, contracts)
	if liquidity.Available < config.MinLiquidityContracts {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    fmt.Sprintf("insufficient liquidity: %d available, need %d minimum", liquidity.Available, config.MinLiquidityContracts),
		}, nil
	}

	// Step 5: Calculate slippage
	slippage := c.CalculateSlippage(book, side, action, contracts)
	if !slippage.Acceptable || slippage.SlippagePct > config.MaxSlippagePct {
		// Try to find optimal size within slippage
		optimalSize := c.GetOptimalSize(book, side, action, config.MaxSlippagePct)
		if optimalSize < config.MinLiquidityContracts {
			return &ExecutionResult{
				Success:            false,
				RequestedContracts: contracts,
				RejectionReason:    fmt.Sprintf("slippage %.2f%% exceeds max %.2f%%, optimal size %d too small", slippage.SlippagePct*100, config.MaxSlippagePct*100, optimalSize),
			}, nil
		}
		// Reduce size to optimal
		contracts = optimalSize
		slippage = c.CalculateSlippage(book, side, action, contracts)
	}

	if slippage.FillableContracts < contracts {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			FilledContracts:    0,
			RejectionReason:    fmt.Sprintf("only %d of %d contracts fillable", slippage.FillableContracts, contracts),
		}, nil
	}

	// Step 6: Re-verify EV at actual execution price (if TrueProb is set)
	if config.TrueProb > 0 {
		actualPrice := slippage.AverageFillPrice / 100.0 // Convert cents to probability
		rawEV := (config.TrueProb * (1 - actualPrice)) - ((1 - config.TrueProb) * actualPrice)
		adjustedEV := rawEV - (actualPrice * config.FeePct)

		if adjustedEV < config.EVThreshold {
			return &ExecutionResult{
				Success:            false,
				RequestedContracts: contracts,
				FilledContracts:    0,
				RejectionReason:    fmt.Sprintf("EV dropped below threshold at execution price: %.2f%% < %.2f%% (price moved from opportunity to %.0f¢)", adjustedEV*100, config.EVThreshold*100, slippage.AverageFillPrice),
			}, nil
		}
	}

	// Step 7: Calculate limit price (best price with small buffer for execution)
	limitPrice := slippage.BestPrice
	if action == ActionBuy {
		// For buying, set limit slightly above best ask to ensure fill
		limitPrice = min(slippage.BestPrice+1, 99) // Max 99 cents
	} else {
		// For selling, set limit slightly below best bid
		limitPrice = max(slippage.BestPrice-1, 1) // Min 1 cent
	}

	// Step 8: Dry run check
	if config.DryRun {
		return &ExecutionResult{
			Success:            true,
			RequestedContracts: contracts,
			FilledContracts:    contracts,
			AveragePrice:       slippage.AverageFillPrice,
			TotalCost:          int64(float64(contracts) * slippage.AverageFillPrice),
			RejectionReason:    "DRY_RUN: order not placed",
		}, nil
	}

	// Step 9: Place the order
	result, err := c.submitOrder(ticker, side, action, contracts, limitPrice)
	if err != nil {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    fmt.Sprintf("order submission failed: %v", err),
		}, nil
	}

	return result, nil
}

// submitOrder sends the order to Kalshi
// Uses IOC (Immediate-Or-Cancel) to avoid resting orders
func (c *KalshiClient) submitOrder(
	ticker string,
	side Side,
	action OrderAction,
	contracts int,
	limitPrice int,
) (*ExecutionResult, error) {
	// Generate unique client order ID
	clientOrderID := uuid.New().String()

	// Create order request with IOC time-in-force
	// Per Kalshi docs, time_in_force: "ioc" = immediate or cancel
	orderReq := CreateOrderRequest{
		Ticker:        ticker,
		ClientOrderID: clientOrderID,
		Side:          side,
		Action:        action,
		Count:         contracts,
		Type:          OrderTypeLimit,
		TimeInForce:   TimeInForceIOC, // Immediate-or-cancel
	}

	// Set price based on side
	if side == SideYes {
		orderReq.YesPrice = limitPrice
	} else {
		orderReq.NoPrice = limitPrice
	}

	jsonBody, err := json.Marshal(orderReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling order request: %w", err)
	}

	body, err := c.doAuthenticatedRequest(http.MethodPost, "/portfolio/orders", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("placing order: %w", err)
	}

	var resp OrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing order response: %w", err)
	}

	order := resp.Order

	// IOC orders execute immediately, so initial response should have fill info
	// But let's wait briefly and fetch updated status for accuracy
	time.Sleep(100 * time.Millisecond)

	// Fetch updated order status
	updatedOrder, err := c.GetOrder(order.OrderID)
	if err != nil {
		// Order placed but couldn't get status - return what we know
		avgPrice := order.AvgFillPrice()
		return &ExecutionResult{
			Success:            true,
			OrderID:            order.OrderID,
			RequestedContracts: contracts,
			FilledContracts:    order.FillCount,
			AveragePrice:       avgPrice,
			TotalCost:          int64(float64(order.FillCount) * avgPrice),
		}, nil
	}

	avgPrice := updatedOrder.AvgFillPrice()
	success := updatedOrder.FillCount > 0
	rejectionReason := ""
	if updatedOrder.FillCount == 0 {
		rejectionReason = fmt.Sprintf("order %s, no fills", updatedOrder.Status)
	} else if updatedOrder.FillCount < contracts {
		rejectionReason = fmt.Sprintf("partial fill: %d of %d", updatedOrder.FillCount, contracts)
	}

	return &ExecutionResult{
		Success:            success,
		OrderID:            updatedOrder.OrderID,
		RequestedContracts: contracts,
		FilledContracts:    updatedOrder.FillCount,
		AveragePrice:       avgPrice,
		TotalCost:          int64(float64(updatedOrder.FillCount) * avgPrice),
		RejectionReason:    rejectionReason,
	}, nil
}

// GetOrder fetches an order by ID
func (c *KalshiClient) GetOrder(orderID string) (*Order, error) {
	path := fmt.Sprintf("/portfolio/orders/%s", orderID)
	body, err := c.doAuthenticatedRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching order: %w", err)
	}

	var resp struct {
		Order Order `json:"order"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing order response: %w", err)
	}

	return &resp.Order, nil
}

// CancelOrder cancels an open order
func (c *KalshiClient) CancelOrder(orderID string) error {
	path := fmt.Sprintf("/portfolio/orders/%s", orderID)
	_, err := c.doAuthenticatedRequest(http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("canceling order: %w", err)
	}
	return nil
}

// PlaceMarketOrder places a market order (immediate execution at best available price)
func (c *KalshiClient) PlaceMarketOrder(
	ticker string,
	side Side,
	action OrderAction,
	contracts int,
	config OrderConfig,
) (*ExecutionResult, error) {
	// For market orders, we still do safeguards but use market order type
	// Step 1: Check exchange status
	tradingActive, err := c.IsTradingActive()
	if err != nil {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    fmt.Sprintf("failed to check exchange status: %v", err),
		}, nil
	}
	if !tradingActive {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    "exchange is not active for trading",
		}, nil
	}

	// Step 2: Check liquidity (still important for market orders)
	book, err := c.GetOrderBook(ticker)
	if err != nil {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    fmt.Sprintf("failed to fetch order book: %v", err),
		}, nil
	}

	liquidity := c.CheckLiquidity(book, side, action, contracts)
	if liquidity.Available < config.MinLiquidityContracts {
		return &ExecutionResult{
			Success:            false,
			RequestedContracts: contracts,
			RejectionReason:    fmt.Sprintf("insufficient liquidity for market order: %d available", liquidity.Available),
		}, nil
	}

	// Dry run
	if config.DryRun {
		slippage := c.CalculateSlippage(book, side, action, contracts)
		return &ExecutionResult{
			Success:            true,
			RequestedContracts: contracts,
			FilledContracts:    slippage.FillableContracts,
			AveragePrice:       slippage.AverageFillPrice,
			TotalCost:          int64(float64(slippage.FillableContracts) * slippage.AverageFillPrice),
			RejectionReason:    "DRY_RUN: market order not placed",
		}, nil
	}

	// Submit market order
	return c.submitMarketOrder(ticker, side, action, contracts)
}

func (c *KalshiClient) submitMarketOrder(
	ticker string,
	side Side,
	action OrderAction,
	contracts int,
) (*ExecutionResult, error) {
	clientOrderID := uuid.New().String()

	orderReq := CreateOrderRequest{
		Ticker:        ticker,
		ClientOrderID: clientOrderID,
		Side:          side,
		Action:        action,
		Count:         contracts,
		Type:          OrderTypeMarket,
	}

	jsonBody, err := json.Marshal(orderReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling order request: %w", err)
	}

	body, err := c.doAuthenticatedRequest(http.MethodPost, "/portfolio/orders", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("placing market order: %w", err)
	}

	var resp OrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing order response: %w", err)
	}

	order := resp.Order
	avgPrice := order.AvgFillPrice()

	return &ExecutionResult{
		Success:            order.FillCount > 0,
		OrderID:            order.OrderID,
		RequestedContracts: contracts,
		FilledContracts:    order.FillCount,
		AveragePrice:       avgPrice,
		TotalCost:          int64(float64(order.FillCount) * avgPrice),
	}, nil
}
