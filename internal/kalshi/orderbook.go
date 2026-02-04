package kalshi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

const (
	// MaxSlippagePct is the maximum acceptable slippage (2%)
	MaxSlippagePct = 0.02

	// MinLiquidityContracts is the minimum depth required to trade
	MinLiquidityContracts = 10
)

// GetOrderBook fetches the order book for a market
func (c *KalshiClient) GetOrderBook(ticker string) (*OrderBookResponse, error) {
	path := fmt.Sprintf("/markets/%s/orderbook", ticker)
	body, err := c.doAuthenticatedRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching order book: %w", err)
	}

	var resp OrderBookResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing order book: %w", err)
	}

	resp.Ticker = ticker
	return &resp, nil
}

// CheckLiquidity analyzes available liquidity for a given side and size
// side: "yes" or "no"
// action: "buy" or "sell"
// contracts: number of contracts we want to trade
func (c *KalshiClient) CheckLiquidity(book *OrderBookResponse, side Side, action OrderAction, contracts int) *LiquidityCheck {
	levels := getLevelsForTrade(book, side, action)
	if len(levels) == 0 {
		return &LiquidityCheck{
			Available:  0,
			Sufficient: false,
		}
	}

	// Sort levels by price (best first)
	// For buying: lowest ask first
	// For selling: highest bid first
	sortLevels(levels, action)

	totalAvailable := 0
	totalCost := 0.0
	bestPrice := levels[0].Price

	for _, level := range levels {
		totalAvailable += level.Count
		// Calculate weighted cost
		totalCost += float64(level.Price * level.Count)
	}

	avgPrice := 0.0
	if totalAvailable > 0 {
		avgPrice = totalCost / float64(totalAvailable)
	}

	return &LiquidityCheck{
		Available:    totalAvailable,
		BestPrice:    bestPrice,
		AveragePrice: avgPrice,
		TotalDepth:   totalAvailable,
		Sufficient:   totalAvailable >= MinLiquidityContracts,
	}
}

// CalculateSlippage computes the slippage for a given trade size
func (c *KalshiClient) CalculateSlippage(book *OrderBookResponse, side Side, action OrderAction, contracts int) *SlippageResult {
	levels := getLevelsForTrade(book, side, action)
	if len(levels) == 0 {
		return &SlippageResult{
			RequestedContracts: contracts,
			FillableContracts:  0,
			Acceptable:         false,
		}
	}

	sortLevels(levels, action)

	bestPrice := levels[0].Price
	remaining := contracts
	totalCost := 0
	filledContracts := 0

	for _, level := range levels {
		if remaining <= 0 {
			break
		}

		fillAtLevel := min(remaining, level.Count)
		totalCost += fillAtLevel * level.Price
		filledContracts += fillAtLevel
		remaining -= fillAtLevel
	}

	if filledContracts == 0 {
		return &SlippageResult{
			RequestedContracts: contracts,
			FillableContracts:  0,
			Acceptable:         false,
		}
	}

	avgPrice := float64(totalCost) / float64(filledContracts)
	slippagePct := 0.0
	if action == ActionBuy {
		// For buying, slippage = (avgPrice - bestPrice) / bestPrice
		slippagePct = (avgPrice - float64(bestPrice)) / float64(bestPrice)
	} else {
		// For selling, slippage = (bestPrice - avgPrice) / bestPrice
		slippagePct = (float64(bestPrice) - avgPrice) / float64(bestPrice)
	}

	// Ensure non-negative (rounding errors)
	if slippagePct < 0 {
		slippagePct = 0
	}

	return &SlippageResult{
		RequestedContracts: contracts,
		FillableContracts:  filledContracts,
		AverageFillPrice:   avgPrice,
		BestPrice:          bestPrice,
		SlippagePct:        slippagePct,
		Acceptable:         slippagePct <= MaxSlippagePct,
	}
}

// GetFillPrice calculates the average fill price for a given size
// Returns price in cents
func (c *KalshiClient) GetFillPrice(book *OrderBookResponse, side Side, action OrderAction, contracts int) (float64, error) {
	result := c.CalculateSlippage(book, side, action, contracts)
	if result.FillableContracts == 0 {
		return 0, fmt.Errorf("no liquidity available")
	}
	if result.FillableContracts < contracts {
		return result.AverageFillPrice, fmt.Errorf("only %d of %d contracts fillable", result.FillableContracts, contracts)
	}
	return result.AverageFillPrice, nil
}

// GetOptimalSize returns the maximum contracts tradeable within slippage limits
func (c *KalshiClient) GetOptimalSize(book *OrderBookResponse, side Side, action OrderAction, maxSlippage float64) int {
	levels := getLevelsForTrade(book, side, action)
	if len(levels) == 0 {
		return 0
	}

	sortLevels(levels, action)

	// Binary search for max size within slippage
	low, high := 0, 0
	for _, l := range levels {
		high += l.Count
	}

	optimalSize := 0
	for low <= high {
		mid := (low + high) / 2
		result := c.CalculateSlippage(book, side, action, mid)

		if result.FillableContracts == mid && result.SlippagePct <= maxSlippage {
			optimalSize = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	// Also consider: we want at least best level contracts
	if optimalSize == 0 && len(levels) > 0 && levels[0].Count > 0 {
		return min(levels[0].Count, MinLiquidityContracts)
	}

	// Ensure at least best level if within slippage
	if optimalSize < levels[0].Count {
		result := c.CalculateSlippage(book, side, action, levels[0].Count)
		if result.SlippagePct <= maxSlippage {
			return levels[0].Count
		}
	}

	return optimalSize
}

// getLevelsForTrade returns the relevant order book levels for a trade
// Per Kalshi docs: order book returns "yes bids and no bids only (no asks)"
// In Kalshi's order book (nested under orderbook.yes and orderbook.no):
// - Yes bids are people wanting to buy YES (we'd sell to them)
// - No bids are people wanting to buy NO (equivalent to selling YES, so we'd buy YES from them)
func getLevelsForTrade(book *OrderBookResponse, side Side, action OrderAction) []OrderBookLevel {
	// Parse raw API format to our internal format
	yesBids := ParseLevels(book.OrderBook.Yes)
	noBids := ParseLevels(book.OrderBook.No)

	if side == SideYes {
		if action == ActionBuy {
			// Buying YES: match against NO bids (they're selling YES implicitly)
			// A NO bid at price P means they'll sell YES at (100 - P)
			return convertNoBidsToYesOffers(noBids)
		} else {
			// Selling YES: match against YES bidders
			return yesBids
		}
	} else { // SideNo
		if action == ActionBuy {
			// Buying NO: match against YES bids (they're selling NO implicitly)
			return convertYesBidsToNoOffers(yesBids)
		} else {
			// Selling NO: match against NO bidders
			return noBids
		}
	}
}

// convertNoBidsToYesOffers converts NO bids to equivalent YES offers
// A NO bid at price P means they'll sell YES at (100 - P)
func convertNoBidsToYesOffers(noBids []OrderBookLevel) []OrderBookLevel {
	offers := make([]OrderBookLevel, len(noBids))
	for i, bid := range noBids {
		offers[i] = OrderBookLevel{
			Price: 100 - bid.Price, // Convert NO price to YES price
			Count: bid.Count,
		}
	}
	return offers
}

// convertYesBidsToNoOffers converts YES bids to equivalent NO offers
func convertYesBidsToNoOffers(yesBids []OrderBookLevel) []OrderBookLevel {
	offers := make([]OrderBookLevel, len(yesBids))
	for i, bid := range yesBids {
		offers[i] = OrderBookLevel{
			Price: 100 - bid.Price,
			Count: bid.Count,
		}
	}
	return offers
}

// sortLevels sorts price levels by best price first
func sortLevels(levels []OrderBookLevel, action OrderAction) {
	if action == ActionBuy {
		// For buying, best = lowest price (ascending)
		sort.Slice(levels, func(i, j int) bool {
			return levels[i].Price < levels[j].Price
		})
	} else {
		// For selling, best = highest price (descending)
		sort.Slice(levels, func(i, j int) bool {
			return levels[i].Price > levels[j].Price
		})
	}
}
