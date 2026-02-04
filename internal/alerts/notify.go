package alerts

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"sports-betting-bot/internal/analysis"
	"sports-betting-bot/internal/positions"
)

// Notifier handles alert notifications
type Notifier struct {
	mu         sync.Mutex
	lastAlerts map[string]time.Time // Dedupe alerts
	cooldown   time.Duration        // Minimum time between same alerts
}

// NewNotifier creates a new notifier
func NewNotifier(cooldown time.Duration) *Notifier {
	return &Notifier{
		lastAlerts: make(map[string]time.Time),
		cooldown:   cooldown,
	}
}

// AlertOpportunity sends an alert for a +EV opportunity
func (n *Notifier) AlertOpportunity(opp analysis.Opportunity) {
	key := fmt.Sprintf("%d-%s-%s", opp.GameID, opp.MarketType, opp.Side)

	n.mu.Lock()
	// Check cooldown
	if lastTime, ok := n.lastAlerts[key]; ok {
		if time.Since(lastTime) < n.cooldown {
			n.mu.Unlock()
			return
		}
	}
	n.lastAlerts[key] = time.Now()
	n.mu.Unlock()

	// Format the alert
	var sideDesc string
	switch opp.Side {
	case "home":
		sideDesc = opp.HomeTeam
	case "away":
		sideDesc = opp.AwayTeam
	default:
		sideDesc = strings.ToUpper(opp.Side)
	}

	log.Printf(`
================================================================================
🎯 +EV OPPORTUNITY FOUND
================================================================================
Game:        %s @ %s
Market:      %s
Bet:         %s
True Prob:   %.1f%% (from %d books)
Kalshi:      $%.2f (implied %.1f%%)
Raw EV:      %.2f%%
Adj EV:      %.2f%% (after fees)
Kelly Stake: %.1f%% of bankroll
================================================================================
`,
		opp.AwayTeam, opp.HomeTeam,
		opp.MarketType,
		sideDesc,
		opp.TrueProb*100, opp.BookCount,
		opp.KalshiPrice, opp.KalshiPrice*100,
		opp.RawEV*100,
		opp.AdjustedEV*100,
		opp.KellyStake*100,
	)
}

// AlertPlayerProp sends an alert for a +EV player prop opportunity
func (n *Notifier) AlertPlayerProp(opp analysis.PlayerPropOpportunity) {
	key := fmt.Sprintf("prop-%d-%s-%s-%.1f-%s", opp.PlayerID, opp.PropType, opp.PlayerName, opp.Line, opp.Side)

	n.mu.Lock()
	// Check cooldown
	if lastTime, ok := n.lastAlerts[key]; ok {
		if time.Since(lastTime) < n.cooldown {
			n.mu.Unlock()
			return
		}
	}
	n.lastAlerts[key] = time.Now()
	n.mu.Unlock()

	log.Printf(`
================================================================================
🎯 +EV PLAYER PROP FOUND
================================================================================
Game:        %s @ %s
Player:      %s
Prop:        %s %s %.1f
True Prob:   %.1f%% (from %d books)
Kalshi:      $%.2f (implied %.1f%%)
Raw EV:      %.2f%%
Adj EV:      %.2f%% (after fees)
Kelly Stake: %.1f%% of bankroll
================================================================================
`,
		opp.AwayTeam, opp.HomeTeam,
		opp.PlayerName,
		strings.ToUpper(opp.Side), opp.PropType, opp.Line,
		opp.TrueProb*100, opp.BookCount,
		opp.KalshiPrice, opp.KalshiPrice*100,
		opp.RawEV*100,
		opp.AdjustedEV*100,
		opp.KellyStake*100,
	)
}

// AlertHedge sends an alert for a hedge opportunity
func (n *Notifier) AlertHedge(hedge positions.HedgeOpportunity) {
	key := fmt.Sprintf("hedge-%d-%s-%s", hedge.Position.ID, hedge.Position.MarketType, hedge.Position.Side)

	n.mu.Lock()
	if lastTime, ok := n.lastAlerts[key]; ok {
		if time.Since(lastTime) < n.cooldown {
			n.mu.Unlock()
			return
		}
	}
	n.lastAlerts[key] = time.Now()
	n.mu.Unlock()

	emoji := "🔒"
	if hedge.Action == "close" {
		emoji = "💰"
	}

	log.Printf(`
================================================================================
%s HEDGE/ARB ALERT
================================================================================
Position:    %s @ %s - %s %s
Entry:       $%.2f × %d contracts
Current:     $%.2f
Action:      %s
%s
================================================================================
`,
		emoji,
		hedge.Position.AwayTeam, hedge.Position.HomeTeam,
		hedge.Position.MarketType, hedge.Position.Side,
		hedge.Position.EntryPrice, hedge.Position.Contracts,
		hedge.CurrentPrice,
		strings.ToUpper(hedge.Action),
		hedge.Description,
	)
}

// LogScan logs a scan completion
func (n *Notifier) LogScan(gamesScanned int, opportunitiesFound int) {
	if opportunitiesFound > 0 {
		log.Printf("Scan complete: %d games, %d opportunities found", gamesScanned, opportunitiesFound)
	}
}

// LogScanWithProps logs a scan completion with player props
func (n *Notifier) LogScanWithProps(gamesScanned, gameOpps, propOpps int) {
	total := gameOpps + propOpps
	if total > 0 {
		log.Printf("Scan complete: %d games, %d game opps, %d prop opps", gamesScanned, gameOpps, propOpps)
	}
}

// LogError logs an error
func (n *Notifier) LogError(context string, err error) {
	log.Printf("ERROR [%s]: %v", context, err)
}

// LogStartup logs bot startup
func (n *Notifier) LogStartup(config string) {
	log.Printf(`
================================================================================
🏀 Sports Betting Bot Started
================================================================================
%s
================================================================================
`, config)
}

// CleanupOldAlerts removes stale alert records
func (n *Notifier) CleanupOldAlerts() {
	n.mu.Lock()
	defer n.mu.Unlock()
	cutoff := time.Now().Add(-1 * time.Hour)
	for key, t := range n.lastAlerts {
		if t.Before(cutoff) {
			delete(n.lastAlerts, key)
		}
	}
}
