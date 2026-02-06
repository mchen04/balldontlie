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

	log.Printf("+EV GAME: %s %s (%s@%s) | prob=%.1f%%/%dbk kalshi=$%.2f ev=%.2f%% kelly=%.1f%%",
		sideDesc, opp.MarketType, opp.AwayTeam, opp.HomeTeam,
		opp.TrueProb*100, opp.BookCount,
		opp.KalshiPrice, opp.AdjustedEV*100, opp.KellyStake*100,
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

	log.Printf("+EV PROP: %s %s %.0f %s (%s@%s) | prob=%.1f%%/%dbk kalshi=$%.2f ev=%.2f%% kelly=%.1f%%",
		opp.PlayerName, strings.ToUpper(opp.Side), opp.Line, opp.PropType,
		opp.AwayTeam, opp.HomeTeam,
		opp.TrueProb*100, opp.BookCount,
		opp.KalshiPrice, opp.AdjustedEV*100, opp.KellyStake*100,
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

	log.Printf("%s HEDGE: %s %s %s (%s@%s) entry=$%.2f×%d cur=$%.2f action=%s | %s",
		emoji,
		hedge.Position.MarketType, hedge.Position.Side,
		strings.ToUpper(hedge.Action),
		hedge.Position.AwayTeam, hedge.Position.HomeTeam,
		hedge.Position.EntryPrice, hedge.Position.Contracts,
		hedge.CurrentPrice,
		strings.ToUpper(hedge.Action),
		hedge.Description,
	)
}

// LogScanWithProps logs a scan completion with player props
func (n *Notifier) LogScanWithProps(gamesScanned, gameOpps, propOpps int) {
	log.Printf("Scan complete: %d games, %d game opps, %d prop opps", gamesScanned, gameOpps, propOpps)
}

// LogError logs an error
func (n *Notifier) LogError(context string, err error) {
	log.Printf("ERROR [%s]: %v", context, err)
}

// LogStartup logs bot startup
func (n *Notifier) LogStartup(config string) {
	log.Printf("Bot started |%s", config)
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
