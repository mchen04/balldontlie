package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"sports-betting-bot/internal/kalshi"
)

func main() {
	_ = godotenv.Load()
	
	client, err := kalshi.NewKalshiClientFromKey(
		os.Getenv("KALSHI_API_KEY_ID"),
		os.Getenv("KALSHI_PRIVATE_KEY"),
		false,
	)
	if err != nil {
		log.Fatalf("Kalshi client error: %v", err)
	}

	props, err := client.GetPlayerPropMarkets(time.Now())
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Println("=== KALSHI AUSTIN REAVES POINTS MARKETS ===")
	for propType, markets := range props {
		if propType != "points" {
			continue
		}
		for _, m := range markets {
			if strings.Contains(strings.ToLower(m.PlayerName), "reaves") {
				fmt.Printf("Player: %s\n", m.PlayerName)
				fmt.Printf("  Line: %.0f+\n", m.Line)
				fmt.Printf("  Yes Ask: %d¢ (price to buy OVER)\n", m.YesAsk)
				fmt.Printf("  No Ask: %d¢ (price to buy UNDER)\n", m.NoAsk)
				fmt.Printf("  Ticker: %s\n", m.Ticker)
				fmt.Println()
			}
		}
	}
}
