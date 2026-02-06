package engine

import (
	"testing"

	"sports-betting-bot/internal/analysis"
	"sports-betting-bot/internal/kalshi"
	"sports-betting-bot/internal/odds"
)

func TestTradeParamsFromOpportunity(t *testing.T) {
	opp := analysis.Opportunity{
		GameID:     123,
		GameDate:   "2026-02-05",
		HomeTeam:   "PHX",
		AwayTeam:   "GSW",
		MarketType: odds.MarketMoneyline,
		Side:       "home",
		TrueProb:   0.60,
		KalshiPrice: 0.55,
		AdjustedEV: 0.04,
		KellyStake: 0.05,
	}

	tp := TradeParamsFromOpportunity(opp)

	if tp.Side != kalshi.SideYes {
		t.Errorf("Side = %v, want %v (home = yes)", tp.Side, kalshi.SideYes)
	}
	if tp.BetSide != "yes" {
		t.Errorf("BetSide = %q, want %q", tp.BetSide, "yes")
	}
	if tp.LogPrefix != "GAME" {
		t.Errorf("LogPrefix = %q, want %q", tp.LogPrefix, "GAME")
	}
	if tp.MarketType != "moneyline" {
		t.Errorf("MarketType = %q, want %q", tp.MarketType, "moneyline")
	}
	if tp.PositionSide != "home" {
		t.Errorf("PositionSide = %q, want %q", tp.PositionSide, "home")
	}
}

func TestTradeParamsFromOpportunityAway(t *testing.T) {
	opp := analysis.Opportunity{
		GameID:      123,
		GameDate:    "2026-02-05",
		HomeTeam:    "PHX",
		AwayTeam:    "GSW",
		MarketType:  odds.MarketMoneyline,
		Side:        "away",
		TrueProb:    0.55,
		KalshiPrice: 0.50,
	}

	tp := TradeParamsFromOpportunity(opp)

	if tp.Side != kalshi.SideNo {
		t.Errorf("Side = %v, want %v (away = no)", tp.Side, kalshi.SideNo)
	}
}

func TestTradeParamsFromPropOpportunity(t *testing.T) {
	opp := analysis.PlayerPropOpportunity{
		GameID:       123,
		GameDate:     "2026-02-05",
		HomeTeam:     "PHX",
		AwayTeam:     "GSW",
		PlayerName:   "Stephen Curry",
		PropType:     "points",
		Line:         25.5,
		Side:         "over",
		TrueProb:     0.60,
		KalshiPrice:  0.55,
		AdjustedEV:   0.04,
		KalshiTicker: "KXNBAPTS-26FEB05GSWPHX-GSWSCURRY30-25",
	}

	tp := TradeParamsFromPropOpportunity(opp)

	if tp.Side != kalshi.SideYes {
		t.Errorf("Side = %v, want %v (over = yes)", tp.Side, kalshi.SideYes)
	}
	if tp.LogPrefix != "PROP" {
		t.Errorf("LogPrefix = %q, want %q", tp.LogPrefix, "PROP")
	}
	if tp.MarketType != "prop_points" {
		t.Errorf("MarketType = %q, want %q", tp.MarketType, "prop_points")
	}
	if tp.Ticker != opp.KalshiTicker {
		t.Errorf("Ticker = %q, want %q", tp.Ticker, opp.KalshiTicker)
	}
}

func TestTradeParamsFromPropOpportunityUnder(t *testing.T) {
	opp := analysis.PlayerPropOpportunity{
		Side:         "under",
		KalshiTicker: "KXNBAPTS-26FEB05GSWPHX-GSWSCURRY30-25",
	}

	tp := TradeParamsFromPropOpportunity(opp)

	if tp.Side != kalshi.SideNo {
		t.Errorf("Side = %v, want %v (under = no)", tp.Side, kalshi.SideNo)
	}
}
