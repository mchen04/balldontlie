package positions

import (
	"fmt"
	"sports-betting-bot/internal/kalshi"
	"sports-betting-bot/internal/odds"
)

// HedgeOpportunity represents an opportunity to hedge an existing position
type HedgeOpportunity struct {
	Position       Position
	CurrentPrice   float64 // Current price of the opposite side
	GuaranteedProfit float64 // Profit if hedging
	Action         string  // "hedge" or "close"
	Description    string
}

// FindHedgeOpportunities checks existing positions against current odds
// for hedging or arbitrage opportunities
func FindHedgeOpportunities(positions []Position, consensus odds.ConsensusOdds) []HedgeOpportunity {
	var opportunities []HedgeOpportunity

	gameIDStr := fmt.Sprintf("%d", consensus.GameID)

	for _, pos := range positions {
		if pos.GameID != gameIDStr {
			continue
		}

		opp := checkHedge(pos, consensus)
		if opp != nil {
			opportunities = append(opportunities, *opp)
		}
	}

	return opportunities
}

func checkHedge(pos Position, consensus odds.ConsensusOdds) *HedgeOpportunity {
	if consensus.KalshiOdds == nil {
		return nil
	}

	var oppositePrice float64

	switch pos.MarketType {
	case "moneyline":
		if consensus.KalshiOdds.Moneyline == nil {
			return nil
		}
		if pos.Side == "home" {
			// We have YES on home, check NO (away) price
			oppositePrice = odds.OddsToImplied(consensus.KalshiOdds.Moneyline.Away)
		} else {
			oppositePrice = odds.OddsToImplied(consensus.KalshiOdds.Moneyline.Home)
		}

	case "spread":
		if consensus.KalshiOdds.Spread == nil {
			return nil
		}
		if pos.Side == "home" {
			oppositePrice = odds.OddsToImplied(consensus.KalshiOdds.Spread.AwayOdds)
		} else {
			oppositePrice = odds.OddsToImplied(consensus.KalshiOdds.Spread.HomeOdds)
		}

	case "total":
		if consensus.KalshiOdds.Total == nil {
			return nil
		}
		if pos.Side == "over" {
			oppositePrice = odds.OddsToImplied(consensus.KalshiOdds.Total.UnderOdds)
		} else {
			oppositePrice = odds.OddsToImplied(consensus.KalshiOdds.Total.OverOdds)
		}

	default:
		return nil
	}

	if oppositePrice <= 0 {
		return nil
	}

	// Check for arbitrage opportunity
	// If entry_price + opposite_price < 1.0 (minus fees), guaranteed profit
	// Note: Entry fee was already paid, only count fee on the NEW hedge trade
	totalCost := pos.EntryPrice + oppositePrice
	hedgeFee := kalshi.TakerFee(oppositePrice) // Only fee on new trade

	// Guaranteed return is $1 per contract
	guaranteedProfit := 1.0 - totalCost - hedgeFee

	if guaranteedProfit > 0 {
		return &HedgeOpportunity{
			Position:         pos,
			CurrentPrice:     oppositePrice,
			GuaranteedProfit: guaranteedProfit * float64(pos.Contracts),
			Action:           "hedge",
			Description: fmt.Sprintf(
				"ARB: Buy %d contracts of opposite side at $%.2f. Entry was $%.2f. Guaranteed profit: $%.2f",
				pos.Contracts, oppositePrice, pos.EntryPrice, guaranteedProfit*float64(pos.Contracts),
			),
		}
	}

	// Check if selling current position is profitable
	// Current position value = 1 - opposite_price (implied by market)
	currentValue := 1.0 - oppositePrice
	sellFee := kalshi.TakerFee(currentValue)
	sellProfit := currentValue - pos.EntryPrice - sellFee

	if sellProfit > 0.05 { // Only suggest if profit > 5%
		return &HedgeOpportunity{
			Position:         pos,
			CurrentPrice:     currentValue,
			GuaranteedProfit: sellProfit * float64(pos.Contracts),
			Action:           "close",
			Description: fmt.Sprintf(
				"PROFIT: Sell %d contracts at ~$%.2f (entry $%.2f). Profit: $%.2f (%.1f%%)",
				pos.Contracts, currentValue, pos.EntryPrice, sellProfit*float64(pos.Contracts),
				(sellProfit/pos.EntryPrice)*100,
			),
		}
	}

	return nil
}

// CalculateHedgeSize calculates optimal hedge size for guaranteed profit
func CalculateHedgeSize(entryPrice, oppositePrice float64, contracts int) (hedgeContracts int, profit float64) {
	// For a perfect hedge with equal contracts:
	// Cost = entryPrice * contracts + oppositePrice * contracts
	// Hedge fee = TakerFee(oppositePrice) * contracts (entry fee already paid)
	// Return = $1 * contracts
	// Profit = Return - Cost - HedgeFee

	totalCost := (entryPrice + oppositePrice) * float64(contracts)
	hedgeFee := kalshi.TakerFee(oppositePrice) * float64(contracts)
	profit = float64(contracts) - totalCost - hedgeFee

	return contracts, profit
}
