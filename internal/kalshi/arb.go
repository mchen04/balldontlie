package kalshi

import (
	"fmt"
)

// ArbOpportunity represents a guaranteed profit arbitrage opportunity
type ArbOpportunity struct {
	Ticker           string
	YesPrice         int     // Best YES ask in cents
	NoPrice          int     // Best NO ask in cents
	TotalCost        int     // YES + NO price in cents
	GuaranteedProfit float64 // Profit per contract after fees (cents)
	ProfitPct        float64 // Profit as percentage of cost
	MaxContracts     int     // Maximum contracts available at these prices
	Type             string  // "pure_arb" or "position_arb"
	Description      string
}

// ArbConfig holds configuration for arbitrage detection
type ArbConfig struct {
	KalshiFee       float64 // Fee percentage (e.g., 0.012 = 1.2%)
	MinProfitCents  float64 // Minimum profit per contract to consider (default: 0.5 cents)
	MinProfitPct    float64 // Minimum profit percentage (default: 0.5%)
}

// DefaultArbConfig returns sensible defaults
func DefaultArbConfig() ArbConfig {
	return ArbConfig{
		KalshiFee:      0.012,
		MinProfitCents: 0.5,
		MinProfitPct:   0.005, // 0.5%
	}
}

// DetectPureArb checks if a market has a pure arbitrage opportunity
// Pure arb exists when: YES_ask + NO_ask < 100 - fees
// Returns nil if no arb exists
func (c *KalshiClient) DetectPureArb(ticker string, config ArbConfig) (*ArbOpportunity, error) {
	book, err := c.GetOrderBook(ticker)
	if err != nil {
		return nil, fmt.Errorf("fetching order book: %w", err)
	}

	return AnalyzeArbFromBook(book, ticker, config), nil
}

// AnalyzeArbFromBook analyzes an order book for arbitrage opportunities
// This is the core arb detection logic
func AnalyzeArbFromBook(book *OrderBookResponse, ticker string, config ArbConfig) *ArbOpportunity {
	// Parse raw API format to our internal format
	yesBids := ParseLevels(book.OrderBook.Yes)
	noBids := ParseLevels(book.OrderBook.No)

	// Get best YES ask (from NO bids converted)
	// To BUY YES: look at NO bids, convert price (100 - NO_bid = YES_ask)
	yesBestAsk, yesDepth := getBestAskFromNoBids(noBids)
	if yesBestAsk == 0 {
		return nil
	}

	// Get best NO ask (from YES bids converted)
	// To BUY NO: look at YES bids, convert price (100 - YES_bid = NO_ask)
	noBestAsk, noDepth := getBestAskFromYesBids(yesBids)
	if noBestAsk == 0 {
		return nil
	}

	// Total cost to buy both sides
	totalCost := yesBestAsk + noBestAsk

	// Calculate fees on both legs
	// Fee is charged on the price paid
	yesFee := float64(yesBestAsk) * config.KalshiFee
	noFee := float64(noBestAsk) * config.KalshiFee
	totalFees := yesFee + noFee

	// Guaranteed return is $1 (100 cents) per contract
	// Profit = 100 - totalCost - totalFees
	profitCents := 100.0 - float64(totalCost) - totalFees

	// Check if profitable
	if profitCents < config.MinProfitCents {
		return nil
	}

	profitPct := profitCents / float64(totalCost)
	if profitPct < config.MinProfitPct {
		return nil
	}

	// Max contracts is limited by the side with less depth
	maxContracts := min(yesDepth, noDepth)

	return &ArbOpportunity{
		Ticker:           ticker,
		YesPrice:         yesBestAsk,
		NoPrice:          noBestAsk,
		TotalCost:        totalCost,
		GuaranteedProfit: profitCents,
		ProfitPct:        profitPct,
		MaxContracts:     maxContracts,
		Type:             "pure_arb",
		Description: fmt.Sprintf(
			"ARB: Buy YES@%d¢ + NO@%d¢ = %d¢ cost. Return=$1. Profit=%.1f¢ (%.2f%%). Max %d contracts.",
			yesBestAsk, noBestAsk, totalCost, profitCents, profitPct*100, maxContracts,
		),
	}
}

// DetectPositionArb checks if an existing position can be arbed
// This happens when the opposite side has moved favorably since entry
func DetectPositionArb(
	entryPrice float64, // Entry price as probability (0-1)
	entrySide Side,     // Which side we own (yes/no)
	book *OrderBookResponse,
	config ArbConfig,
) *ArbOpportunity {
	entryPriceCents := int(entryPrice * 100)

	// Parse raw API format
	yesBids := ParseLevels(book.OrderBook.Yes)
	noBids := ParseLevels(book.OrderBook.No)

	var oppositeAsk int
	var oppositeDepth int

	if entrySide == SideYes {
		// We own YES, need to buy NO to complete arb
		oppositeAsk, oppositeDepth = getBestAskFromYesBids(yesBids)
	} else {
		// We own NO, need to buy YES to complete arb
		oppositeAsk, oppositeDepth = getBestAskFromNoBids(noBids)
	}

	if oppositeAsk == 0 {
		return nil
	}

	// Total cost = what we paid + what we'd pay for opposite
	totalCost := entryPriceCents + oppositeAsk

	// Fee only on the NEW trade (entry fee already paid)
	newFee := float64(oppositeAsk) * config.KalshiFee

	// Profit = 100 - totalCost - newFee
	profitCents := 100.0 - float64(totalCost) - newFee

	if profitCents < config.MinProfitCents {
		return nil
	}

	profitPct := profitCents / float64(totalCost)

	oppositeSideStr := "NO"
	if entrySide == SideNo {
		oppositeSideStr = "YES"
	}

	return &ArbOpportunity{
		YesPrice:         entryPriceCents,
		NoPrice:          oppositeAsk,
		TotalCost:        totalCost,
		GuaranteedProfit: profitCents,
		ProfitPct:        profitPct,
		MaxContracts:     oppositeDepth,
		Type:             "position_arb",
		Description: fmt.Sprintf(
			"POSITION ARB: Entry@%d¢, buy %s@%d¢. Total=%d¢. Guaranteed profit=%.1f¢ (%.2f%%)",
			entryPriceCents, oppositeSideStr, oppositeAsk, totalCost, profitCents, profitPct*100,
		),
	}
}

// CheckCanAddToPosition determines if we should add to an existing position
// Returns: (canAdd bool, isArb bool, arbOpp *ArbOpportunity)
//
// Rules:
// 1. If no existing position → can add (normal +EV trade)
// 2. If existing position on SAME side → don't add (avoid overexposure)
// 3. If existing position on OPPOSITE side AND arb exists → add (arb opportunity)
// 4. If existing position on OPPOSITE side, no arb → don't add
func (c *KalshiClient) CheckCanAddToPosition(
	ticker string,
	proposedSide Side,
	config ArbConfig,
) (canAdd bool, isArb bool, arbOpp *ArbOpportunity, err error) {
	// Check existing position
	hasPosition, positionSize, err := c.HasPositionOnMarket(ticker)
	if err != nil {
		return false, false, nil, fmt.Errorf("checking position: %w", err)
	}

	// No existing position → can add normally
	if !hasPosition || positionSize == 0 {
		return true, false, nil, nil
	}

	// Determine existing side
	existingSide := SideYes
	if positionSize < 0 {
		existingSide = SideNo
	}

	// Same side → don't add more (avoid overexposure on same direction)
	if existingSide == proposedSide {
		return false, false, nil, nil
	}

	// Opposite side → check for arb
	book, err := c.GetOrderBook(ticker)
	if err != nil {
		return false, false, nil, fmt.Errorf("fetching order book: %w", err)
	}

	// We need to estimate our entry price
	// For now, check if pure arb exists (simpler and safer)
	arbOpp = AnalyzeArbFromBook(book, ticker, config)
	if arbOpp != nil {
		return true, true, arbOpp, nil
	}

	// No arb, don't add to opposite side
	return false, false, nil, nil
}

// getBestAskFromNoBids converts NO bids to YES asks and returns best price
// NO bid at P means seller will sell YES at (100-P)
func getBestAskFromNoBids(noBids []OrderBookLevel) (bestAsk int, depth int) {
	if len(noBids) == 0 {
		return 0, 0
	}

	// Find highest NO bid (which gives lowest YES ask)
	bestNoBid := 0
	for _, bid := range noBids {
		if bid.Price > bestNoBid {
			bestNoBid = bid.Price
			depth = bid.Count
		}
	}

	if bestNoBid == 0 {
		return 0, 0
	}

	// Convert: YES ask = 100 - NO bid
	return 100 - bestNoBid, depth
}

// getBestAskFromYesBids converts YES bids to NO asks and returns best price
func getBestAskFromYesBids(yesBids []OrderBookLevel) (bestAsk int, depth int) {
	if len(yesBids) == 0 {
		return 0, 0
	}

	// Find highest YES bid (which gives lowest NO ask)
	bestYesBid := 0
	for _, bid := range yesBids {
		if bid.Price > bestYesBid {
			bestYesBid = bid.Price
			depth = bid.Count
		}
	}

	if bestYesBid == 0 {
		return 0, 0
	}

	// Convert: NO ask = 100 - YES bid
	return 100 - bestYesBid, depth
}

// CalculateArbProfit calculates the profit from an arbitrage trade
// given specific prices and contract count
func CalculateArbProfit(yesPriceCents, noPriceCents, contracts int, feeRate float64) float64 {
	totalCost := float64(yesPriceCents+noPriceCents) * float64(contracts)
	fees := totalCost * feeRate
	returns := 100.0 * float64(contracts) // $1 per contract guaranteed
	return returns - totalCost - fees
}

// ExecuteArb executes an arbitrage opportunity by buying both sides
func (c *KalshiClient) ExecuteArb(arb *ArbOpportunity, contracts int, config OrderConfig) (*ExecutionResult, *ExecutionResult, error) {
	if contracts > arb.MaxContracts {
		contracts = arb.MaxContracts
	}

	// Execute YES leg
	yesResult, err := c.PlaceOrder(arb.Ticker, SideYes, ActionBuy, contracts, config)
	if err != nil {
		return nil, nil, fmt.Errorf("executing YES leg: %w", err)
	}

	if !yesResult.Success || yesResult.FilledContracts == 0 {
		return yesResult, nil, nil
	}

	// Execute NO leg with matched size
	noContracts := yesResult.FilledContracts
	noResult, err := c.PlaceOrder(arb.Ticker, SideNo, ActionBuy, noContracts, config)
	if err != nil {
		// YES leg executed but NO leg failed - we have a position now
		return yesResult, nil, fmt.Errorf("YES filled %d but NO leg failed: %w", yesResult.FilledContracts, err)
	}

	return yesResult, noResult, nil
}
