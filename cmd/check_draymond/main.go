package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"sports-betting-bot/internal/kalshi"
)

func main() {
	_ = godotenv.Load()
	client, _ := kalshi.NewKalshiClientFromKey(
		os.Getenv("KALSHI_API_KEY_ID"),
		os.Getenv("KALSHI_PRIVATE_KEY"),
		false,
	)

	props, _ := client.GetPlayerPropMarkets(time.Now())

	fmt.Println("Draymond Green - Full Market Data:")
	fmt.Println()

	for _, m := range props["points"] {
		if strings.Contains(m.PlayerName, "Draymond") {
			fmt.Printf("Line %.0f+:\n", m.Line)
			fmt.Printf("  Ticker: %s\n", m.Ticker)
			fmt.Printf("  Yes Bid: %d¢  Yes Ask: %d¢\n", m.YesBid, m.YesAsk)
			fmt.Printf("  No Bid:  %d¢  No Ask:  %d¢\n", m.NoBid, m.NoAsk)
			spread := 0
			if m.YesAsk > 0 && m.YesBid > 0 {
				spread = m.YesAsk - m.YesBid
			}
			fmt.Printf("  Spread: %d¢\n", spread)
			fmt.Println()
		}
	}
}
