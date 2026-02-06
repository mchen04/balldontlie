package alerts

import (
	"testing"
	"time"

	"sports-betting-bot/internal/analysis"
	"sports-betting-bot/internal/odds"
)

func TestCheckCooldownSuppresses(t *testing.T) {
	n := NewNotifier(1 * time.Second)

	// First call should not suppress
	if n.checkCooldown("test-key") {
		t.Error("first call should not be suppressed")
	}

	// Immediate second call should suppress
	if !n.checkCooldown("test-key") {
		t.Error("second call within cooldown should be suppressed")
	}
}

func TestCheckCooldownExpires(t *testing.T) {
	n := NewNotifier(10 * time.Millisecond)

	if n.checkCooldown("test-key") {
		t.Error("first call should not be suppressed")
	}

	time.Sleep(15 * time.Millisecond)

	if n.checkCooldown("test-key") {
		t.Error("call after cooldown should not be suppressed")
	}
}

func TestCheckCooldownDifferentKeys(t *testing.T) {
	n := NewNotifier(1 * time.Second)

	if n.checkCooldown("key-a") {
		t.Error("first call for key-a should not be suppressed")
	}

	// Different key should not be suppressed
	if n.checkCooldown("key-b") {
		t.Error("first call for key-b should not be suppressed")
	}

	// Same key should be suppressed
	if !n.checkCooldown("key-a") {
		t.Error("second call for key-a should be suppressed")
	}
}

func TestAlertOpportunityCooldown(t *testing.T) {
	n := NewNotifier(1 * time.Second)

	opp := analysis.Opportunity{
		GameID:      1,
		MarketType:  odds.MarketMoneyline,
		Side:        "home",
		HomeTeam:    "LAL",
		AwayTeam:    "BOS",
		TrueProb:    0.6,
		KalshiPrice: 0.55,
		AdjustedEV:  0.04,
		KellyStake:  0.05,
	}

	// Should not panic and should log the first time
	n.AlertOpportunity(opp)

	// Second call should be suppressed (no log)
	n.AlertOpportunity(opp)
}

func TestCleanupOldAlerts(t *testing.T) {
	n := NewNotifier(1 * time.Hour)

	// Manually insert an old alert
	n.mu.Lock()
	n.lastAlerts["old-key"] = time.Now().Add(-2 * time.Hour)
	n.lastAlerts["fresh-key"] = time.Now()
	n.mu.Unlock()

	n.CleanupOldAlerts()

	n.mu.Lock()
	defer n.mu.Unlock()

	if _, ok := n.lastAlerts["old-key"]; ok {
		t.Error("old alert should have been cleaned up")
	}
	if _, ok := n.lastAlerts["fresh-key"]; !ok {
		t.Error("fresh alert should not have been cleaned up")
	}
}
