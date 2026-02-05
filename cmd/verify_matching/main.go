package main

import (
	"fmt"
	"sports-betting-bot/internal/kalshi"
)

func main() {
	fmt.Println("Testing PlayerNamesMatch function:")
	fmt.Println()

	tests := []struct {
		name1, name2 string
		shouldMatch  bool
	}{
		// Should match
		{"LeBron James", "LeBron James", true},
		{"Jaren Jackson Jr.", "Jaren Jackson", true},
		{"CJ McCollum", "C.J. McCollum", true},
		{"Nicolas Claxton", "Nic Claxton", true},

		// Should NOT match (different players)
		{"Jeff Green", "Draymond Green", false},
		{"Bronny James", "LeBron James", false},
		{"Ausar Thompson", "Amen Thompson", false},
		{"Jalen Johnson", "Keldon Johnson", false},
		{"Josh Green", "Jalen Green", false},
		{"Harrison Barnes", "Scottie Barnes", false},
	}

	allPassed := true
	for _, t := range tests {
		result := kalshi.PlayerNamesMatch(t.name1, t.name2)
		status := "✓"
		if result != t.shouldMatch {
			status = "✗ WRONG"
			allPassed = false
		}
		fmt.Printf("  %-20s vs %-20s: match=%v (expected %v) %s\n",
			t.name1, t.name2, result, t.shouldMatch, status)
	}

	fmt.Println()
	if allPassed {
		fmt.Println("✅ All name matching tests PASSED")
	} else {
		fmt.Println("❌ Some name matching tests FAILED")
	}
}
