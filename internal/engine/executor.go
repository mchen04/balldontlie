package engine

import (
	"fmt"
	"log/slog"

	"sports-betting-bot/internal/analysis"
	"sports-betting-bot/internal/config"
	"sports-betting-bot/internal/kalshi"
	"sports-betting-bot/internal/positions"
)

// TradeParams captures the common fields needed for trade execution,
// regardless of whether the opportunity is a game market or player prop.
type TradeParams struct {
	Ticker       string
	Side         kalshi.Side
	BetSide      string
	TrueProb     float64
	KalshiPrice  float64
	AdjustedEV   float64
	KellyStake   float64
	GameID       int
	HomeTeam     string
	AwayTeam     string
	MarketType   string
	PositionSide string // e.g. "home", "away", "over", "under", or player-specific
	LogPrefix    string // "GAME" or "PROP"
}

// TradeParamsFromOpportunity builds TradeParams from a game opportunity.
func TradeParamsFromOpportunity(opp analysis.Opportunity) TradeParams {
	ticker := MapToKalshiTicker(opp)

	side := kalshi.SideYes
	if opp.Side == "away" || opp.Side == "under" {
		side = kalshi.SideNo
	}

	return TradeParams{
		Ticker:       ticker,
		Side:         side,
		BetSide:      string(side),
		TrueProb:     opp.TrueProb,
		KalshiPrice:  opp.KalshiPrice,
		AdjustedEV:   opp.AdjustedEV,
		KellyStake:   opp.KellyStake,
		GameID:       opp.GameID,
		HomeTeam:     opp.HomeTeam,
		AwayTeam:     opp.AwayTeam,
		MarketType:   string(opp.MarketType),
		PositionSide: opp.Side,
		LogPrefix:    "GAME",
	}
}

// TradeParamsFromPropOpportunity builds TradeParams from a player prop opportunity.
func TradeParamsFromPropOpportunity(opp analysis.PlayerPropOpportunity) TradeParams {
	side := kalshi.SideYes
	if opp.Side == "under" {
		side = kalshi.SideNo
	}

	return TradeParams{
		Ticker:       opp.KalshiTicker,
		Side:         side,
		BetSide:      string(side),
		TrueProb:     opp.TrueProb,
		KalshiPrice:  opp.KalshiPrice,
		AdjustedEV:   opp.AdjustedEV,
		KellyStake:   opp.KellyStake,
		GameID:       opp.GameID,
		HomeTeam:     opp.HomeTeam,
		AwayTeam:     opp.AwayTeam,
		MarketType:   fmt.Sprintf("prop_%s", opp.PropType),
		PositionSide: fmt.Sprintf("%s_%s_%.1f", opp.PlayerName, opp.Side, opp.Line),
		LogPrefix:    "PROP",
	}
}

// ExecuteOpportunity handles the full lifecycle of executing a game opportunity:
// ticker mapping, duplicate check, arb detection, and trade execution.
// Returns the dollar amount spent.
func ExecuteOpportunity(
	kalshiClient *kalshi.KalshiClient,
	opp analysis.Opportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg config.Config,
	db *positions.DB,
) float64 {
	tp := TradeParamsFromOpportunity(opp)
	if tp.Ticker == "" {
		slog.Error("No ticker for game", "gameID", opp.GameID)
		return 0
	}

	// Check database for existing position
	if db != nil {
		hasPosition, err := db.HasPositionOnTicker(tp.Ticker, tp.BetSide)
		if err != nil {
			slog.Error("Checking DB failed", "ticker", tp.Ticker, "err", err)
		} else if hasPosition {
			return 0
		}
	}

	// Check for existing position on Kalshi - prevent duplicate bets unless arb
	arbConfig := kalshi.ArbConfig{
		MinProfitCents: config.DefaultMinArbProfitCents,
		MinProfitPct:   config.DefaultMinArbProfitPct,
	}

	canAdd, isArb, arbOpp, err := kalshiClient.CheckCanAddToPosition(tp.Ticker, tp.Side, arbConfig)
	if err != nil {
		slog.Error("Checking position failed", "ticker", tp.Ticker, "err", err)
		return 0
	}
	if !canAdd {
		return 0
	}

	// If this is an arb opportunity, execute the arb instead of the +EV trade
	if isArb && arbOpp != nil {
		slog.Info("Arb detected", "ticker", tp.Ticker, "description", arbOpp.Description)
		return ExecuteArbitrage(kalshiClient, arbOpp, bankroll, execConfig, cfg, db, tp)
	}

	return ExecuteTrade(kalshiClient, tp, bankroll, execConfig, cfg, db)
}

// ExecutePropOpportunity handles the full lifecycle of executing a player prop opportunity.
// Returns the dollar amount spent.
func ExecutePropOpportunity(
	kalshiClient *kalshi.KalshiClient,
	opp analysis.PlayerPropOpportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg config.Config,
	db *positions.DB,
) float64 {
	tp := TradeParamsFromPropOpportunity(opp)
	if tp.Ticker == "" {
		slog.Error("No ticker for prop", "player", opp.PlayerName, "propType", opp.PropType)
		return 0
	}

	// Check database for existing position
	if db != nil {
		hasPosition, err := db.HasPositionOnTicker(tp.Ticker, tp.BetSide)
		if err != nil {
			slog.Error("Checking DB for prop failed", "ticker", tp.Ticker, "err", err)
		} else if hasPosition {
			return 0
		}
	}

	// Check for existing position on Kalshi - prevent duplicate bets
	arbConfig := kalshi.ArbConfig{
		MinProfitCents: config.DefaultMinArbProfitCents,
		MinProfitPct:   config.DefaultMinArbProfitPct,
	}

	canAdd, _, _, err := kalshiClient.CheckCanAddToPosition(tp.Ticker, tp.Side, arbConfig)
	if err != nil {
		slog.Error("Checking position for prop failed", "ticker", tp.Ticker, "err", err)
		return 0
	}
	if !canAdd {
		return 0
	}

	return ExecuteTrade(kalshiClient, tp, bankroll, execConfig, cfg, db)
}

// ExecuteTrade is the unified trade execution path for both game and prop opportunities.
// Returns the dollar amount spent.
func ExecuteTrade(
	kalshiClient *kalshi.KalshiClient,
	tp TradeParams,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg config.Config,
	db *positions.DB,
) float64 {
	// Calculate bet size using real bankroll
	priceInCents := int(tp.KalshiPrice * 100)
	contracts := analysis.CalculateKellyContracts(
		tp.TrueProb,
		tp.KalshiPrice,
		cfg.KellyFraction,
		bankroll,
		cfg.MaxBetDollars,
		priceInCents,
	)

	if contracts < execConfig.MinLiquidityContracts {
		return 0
	}

	// Fetch order book and check liquidity/slippage
	book, err := kalshiClient.GetOrderBook(tp.Ticker)
	if err != nil {
		slog.Error("Orderbook fetch failed", "ticker", tp.Ticker, "err", err)
		return 0
	}

	slippage := kalshiClient.CalculateSlippage(book, tp.Side, kalshi.ActionBuy, contracts)
	if !slippage.Acceptable {
		return 0
	}

	// Recompute Kelly at actual fill price and take the smaller size
	actualFillPrice := slippage.AverageFillPrice / 100.0
	adjustedContracts := analysis.CalculateKellyContracts(
		tp.TrueProb, actualFillPrice, cfg.KellyFraction,
		bankroll, cfg.MaxBetDollars, int(slippage.AverageFillPrice))
	if adjustedContracts < contracts {
		contracts = adjustedContracts
	}
	if contracts < execConfig.MinLiquidityContracts {
		return 0
	}

	// Recalculate EV with actual fill price
	_, adjustedEV := analysis.RecalculateEVWithSlippage(tp.TrueProb, actualFillPrice)

	if adjustedEV < cfg.EVThreshold {
		slog.Warn("EV degraded after slippage",
			"ticker", tp.Ticker, "before", tp.AdjustedEV*100, "after", adjustedEV*100)
		return 0
	}

	slog.Info("Executing trade",
		"type", tp.LogPrefix, "ticker", tp.Ticker, "side", tp.Side,
		"contracts", contracts, "price", slippage.AverageFillPrice,
		"ev", adjustedEV*100, "kelly", tp.KellyStake*100)

	// Store position in database BEFORE placing order (for duplicate prevention)
	if db != nil {
		pos := positions.Position{
			GameID:     fmt.Sprintf("%d", tp.GameID),
			HomeTeam:   tp.HomeTeam,
			AwayTeam:   tp.AwayTeam,
			MarketType: tp.MarketType,
			Side:       tp.PositionSide,
			Ticker:     tp.Ticker,
			BetSide:    tp.BetSide,
			EntryPrice: slippage.AverageFillPrice / 100,
			Contracts:  contracts,
		}
		id, err := db.AddPosition(pos)
		if err != nil {
			slog.Error("Storing position failed", "err", err)
		} else {
			slog.Info("Stored position", "id", id, "ticker", tp.Ticker, "side", tp.BetSide)
		}
	}

	// Add EV verification to execution config
	execConfigWithEV := execConfig
	execConfigWithEV.TrueProb = tp.TrueProb
	execConfigWithEV.EVThreshold = cfg.EVThreshold

	result, err := kalshiClient.PlaceOrder(tp.Ticker, tp.Side, kalshi.ActionBuy, contracts, execConfigWithEV)
	if err != nil {
		slog.Error("Order failed", "ticker", tp.Ticker, "err", err)
		return 0
	}

	if result.Success {
		slog.Info("Order filled",
			"type", tp.LogPrefix, "orderID", result.OrderID,
			"filled", result.FilledContracts, "requested", result.RequestedContracts,
			"avgPrice", result.AveragePrice, "cost", float64(result.TotalCost)/100)
		return float64(result.TotalCost) / 100
	}

	slog.Warn("Order rejected",
		"type", tp.LogPrefix, "ticker", tp.Ticker, "reason", result.RejectionReason)
	return 0
}

// ExecuteArbitrage executes an arbitrage opportunity.
// Returns the dollar amount spent.
func ExecuteArbitrage(
	kalshiClient *kalshi.KalshiClient,
	arb *kalshi.ArbOpportunity,
	bankroll float64,
	execConfig kalshi.OrderConfig,
	cfg config.Config,
	db *positions.DB,
	tp TradeParams,
) float64 {
	costPerContractCents := arb.TotalCost
	maxAffordable := int(bankroll * 100 / float64(costPerContractCents))

	contracts := min(arb.MaxContracts, maxAffordable)
	if contracts < execConfig.MinLiquidityContracts {
		slog.Info("Arb size below minimum, skipping", "contracts", contracts)
		return 0
	}

	if cfg.MaxBetDollars > 0 {
		maxFromLimit := int(cfg.MaxBetDollars * 100 / float64(costPerContractCents))
		contracts = min(contracts, maxFromLimit)
	}

	// Store arb position in database BEFORE placing order
	if db != nil {
		pos := positions.Position{
			GameID:     fmt.Sprintf("%d", tp.GameID),
			HomeTeam:   tp.HomeTeam,
			AwayTeam:   tp.AwayTeam,
			MarketType: "arb_" + tp.MarketType,
			Side:       "arb",
			Ticker:     tp.Ticker,
			BetSide:    tp.BetSide,
			EntryPrice: float64(arb.TotalCost) / 100,
			Contracts:  contracts,
		}
		id, err := db.AddPosition(pos)
		if err != nil {
			slog.Error("Storing arb position failed", "err", err)
		} else {
			slog.Info("Stored arb position", "id", id, "ticker", tp.Ticker, "side", tp.BetSide)
		}
	}

	if execConfig.DryRun {
		profit := kalshi.CalculateArbProfit(arb.YesPrice, arb.NoPrice, contracts)
		slog.Info("Dry run arb",
			"contracts", contracts, "yesPrice", arb.YesPrice, "noPrice", arb.NoPrice,
			"profit", profit/100)
		return 0
	}

	slog.Info("Executing arb",
		"ticker", arb.Ticker, "contracts", contracts,
		"yesPrice", arb.YesPrice, "noPrice", arb.NoPrice)

	yesResult, noResult, err := kalshiClient.ExecuteArb(arb, contracts, execConfig)
	if err != nil {
		// With concurrent execution, one leg may have filled even on error
		slog.Error("Arb execution error", "err", err)
		if yesResult == nil && noResult == nil {
			return 0
		}
		slog.Warn("Partial arb fill",
			"yesFilled", yesResult != nil && yesResult.FilledContracts > 0,
			"noFilled", noResult != nil && noResult.FilledContracts > 0)
	}

	totalSpent := 0.0
	if yesResult != nil && yesResult.FilledContracts > 0 {
		totalSpent += float64(yesResult.TotalCost) / 100
	}
	if noResult != nil && noResult.FilledContracts > 0 {
		totalSpent += float64(noResult.TotalCost) / 100
	}

	if yesResult != nil && noResult != nil && yesResult.FilledContracts > 0 && noResult.FilledContracts > 0 {
		matchedContracts := min(yesResult.FilledContracts, noResult.FilledContracts)
		profit := kalshi.CalculateArbProfit(
			int(yesResult.AveragePrice),
			int(noResult.AveragePrice),
			matchedContracts,
		)
		slog.Info("Arb filled",
			"matched", matchedContracts, "yesAvg", yesResult.AveragePrice,
			"noAvg", noResult.AveragePrice, "profit", profit/100)
	}

	return totalSpent
}
