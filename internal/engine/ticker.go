package engine

import (
	"log/slog"
	"time"

	"sports-betting-bot/internal/analysis"
	"sports-betting-bot/internal/kalshi"
)

// MapToKalshiTicker maps a BallDontLie game opportunity to a Kalshi market ticker.
// Kalshi NBA tickers follow this format:
// - KXNBAGAME-26FEB04MEMSAC (moneyline: Memphis @ Sacramento on Feb 4, 2026)
// - KXNBASPREAD-26FEB04MEMSAC (spread)
// - KXNBATOTAL-26FEB04NOPMIL (total)
func MapToKalshiTicker(opp analysis.Opportunity) string {
	series := kalshi.GetSeriesForMarketType(string(opp.MarketType))
	if series == "" {
		return ""
	}

	gameDate, err := time.Parse("2006-01-02", opp.GameDate)
	if err != nil {
		slog.Error("Bad game date", "date", opp.GameDate, "err", err)
		return ""
	}

	ticker := kalshi.BuildNBATicker(series, gameDate, opp.AwayTeam, opp.HomeTeam)
	if ticker == "" {
		slog.Error("Build ticker failed", "away", opp.AwayTeam, "home", opp.HomeTeam, "date", opp.GameDate)
	}

	return ticker
}

// MapToPlayerPropTicker maps a player prop opportunity to a Kalshi ticker.
func MapToPlayerPropTicker(opp analysis.PlayerPropOpportunity) string {
	propType := kalshi.PropTypeFromBallDontLie(opp.PropType)
	if propType == "" {
		return ""
	}

	gameDate, err := time.Parse("2006-01-02", opp.GameDate)
	if err != nil {
		slog.Error("Bad game date", "date", opp.GameDate, "err", err)
		return ""
	}

	ticker := kalshi.BuildPlayerPropTicker(propType, gameDate, opp.AwayTeam, opp.HomeTeam)
	if ticker == "" {
		slog.Error("Build prop ticker failed",
			"player", opp.PlayerName, "propType", opp.PropType, "date", opp.GameDate)
	}

	return ticker
}
