package engine

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"sports-betting-bot/internal/alerts"
	"sports-betting-bot/internal/analysis"
	"sports-betting-bot/internal/api"
	"sports-betting-bot/internal/config"
	"sports-betting-bot/internal/kalshi"
	"sports-betting-bot/internal/odds"
	"sports-betting-bot/internal/positions"
)

// Engine is the main orchestrator that polls for odds, detects +EV opportunities,
// and executes trades.
type Engine struct {
	client       *api.BallDontLieClient
	kalshiClient *kalshi.KalshiClient
	notifier     *alerts.Notifier
	db           *positions.DB
	cfg          config.Config
	analysisCfg  analysis.Config
	execConfig   kalshi.OrderConfig

	lastMaintenanceLog time.Time
}

// New creates a new Engine with all dependencies.
func New(
	client *api.BallDontLieClient,
	kalshiClient *kalshi.KalshiClient,
	notifier *alerts.Notifier,
	db *positions.DB,
	cfg config.Config,
	analysisCfg analysis.Config,
	execConfig kalshi.OrderConfig,
) *Engine {
	return &Engine{
		client:       client,
		kalshiClient: kalshiClient,
		notifier:     notifier,
		db:           db,
		cfg:          cfg,
		analysisCfg:  analysisCfg,
		execConfig:   execConfig,
	}
}

// Run starts the main polling loop. It blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(config.DefaultCleanupInterval)
	defer cleanupTicker.Stop()

	slog.Info("Starting polling loop")

	for {
		select {
		case <-ctx.Done():
			slog.Info("Bot stopped gracefully")
			return

		case <-cleanupTicker.C:
			e.notifier.CleanupOldAlerts()

		case <-ticker.C:
			e.Scan()
		}
	}
}

// Scan performs a single scan cycle: fetch odds, find opportunities, execute trades.
func (e *Engine) Scan() {
	gameOdds, err := e.client.GetTodaysOdds()
	if err != nil {
		e.notifier.LogError("fetching odds", err)
		return
	}

	if len(gameOdds) == 0 {
		return
	}

	var allPositions []positions.Position
	if e.db != nil {
		allPositions, _ = e.db.GetAllPositions()
	}

	// Fetch current balance if Kalshi client available
	var bankroll float64
	var kalshiAvailable bool
	if e.kalshiClient != nil {
		if kalshi.IsMaintenanceWindowNow() {
			if time.Since(e.lastMaintenanceLog) > config.DefaultMaintenanceLogCooldown {
				slog.Warn("Kalshi maintenance window - skipping execution", "window", "Thu 3-5am ET")
				e.lastMaintenanceLog = time.Now()
			}
		} else {
			bankroll, err = e.kalshiClient.GetBalanceDollars()
			if err != nil {
				e.notifier.LogError("fetching Kalshi balance", err)
			} else {
				kalshiAvailable = true
			}
		}
	}

	var allGameOpps []analysis.Opportunity
	var allPropOpps []analysis.PlayerPropOpportunity

	// Fetch Kalshi player props once for all games
	var kalshiPlayerProps map[string][]kalshi.PlayerPropMarket
	if e.kalshiClient != nil {
		et, err := time.LoadLocation("America/New_York")
		if err != nil {
			et = time.FixedZone("ET", -5*60*60)
		}
		nowET := time.Now().In(et)
		kalshiPlayerProps, err = e.kalshiClient.GetPlayerPropMarkets(nowET)
		if err != nil {
			e.notifier.LogError("fetching Kalshi player props", err)
		}
	}

	for _, game := range gameOdds {
		status := game.Game.Status
		if status == "Final" || strings.Contains(status, "Qtr") || status == "Halftime" || status == "OT" {
			continue
		}

		if game.Game.StartsWithin(config.DefaultPreGameSkipWindow) {
			continue
		}

		consensus := odds.CalculateConsensus(game)

		opportunities := analysis.FindAllOpportunities(consensus, e.analysisCfg)
		allGameOpps = append(allGameOpps, opportunities...)

		playerProps, err := e.client.GetPlayerProps(game.GameID)
		if err == nil && len(playerProps) > 0 {
			if len(kalshiPlayerProps) > 0 {
				playerIDSet := make(map[int]bool)
				for _, prop := range playerProps {
					playerIDSet[prop.PlayerID] = true
				}
				playerIDs := make([]int, 0, len(playerIDSet))
				for id := range playerIDSet {
					playerIDs = append(playerIDs, id)
				}
				playerNames := e.client.GetPlayerNames(playerIDs)

				propOpportunities := analysis.FindPlayerPropOpportunitiesWithInterpolation(
					playerProps,
					kalshiPlayerProps,
					playerNames,
					game.Game.Date,
					game.Game.HomeTeam.Abbreviation,
					game.Game.VisitorTeam.Abbreviation,
					game.GameID,
					e.analysisCfg,
				)
				allPropOpps = append(allPropOpps, propOpportunities...)
			}
		}

		if e.db != nil && len(allPositions) > 0 {
			hedges := positions.FindHedgeOpportunities(allPositions, consensus)
			for _, hedge := range hedges {
				e.notifier.AlertHedge(hedge)
			}
		}
	}

	sort.Slice(allGameOpps, func(i, j int) bool {
		return allGameOpps[i].AdjustedEV > allGameOpps[j].AdjustedEV
	})

	sort.Slice(allPropOpps, func(i, j int) bool {
		return allPropOpps[i].AdjustedEV > allPropOpps[j].AdjustedEV
	})

	for _, opp := range allGameOpps {
		e.notifier.AlertOpportunity(opp)
		if kalshiAvailable && bankroll > 0 {
			spent := ExecuteOpportunity(e.kalshiClient, opp, bankroll, e.execConfig, e.cfg, e.db)
			bankroll -= spent
		}
	}

	for _, propOpp := range allPropOpps {
		e.notifier.AlertPlayerProp(propOpp)
		if kalshiAvailable && bankroll > 0 {
			spent := ExecutePropOpportunity(e.kalshiClient, propOpp, bankroll, e.execConfig, e.cfg, e.db)
			bankroll -= spent
		}
	}

	e.notifier.LogScanWithProps(len(gameOdds), len(allGameOpps), len(allPropOpps))
}
