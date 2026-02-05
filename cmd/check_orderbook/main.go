package main

import (
	"fmt"
	"os"
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
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Get all player prop markets for today
	props, err := client.GetPlayerPropMarkets(time.Now())
	if err != nil {
		fmt.Printf("Error getting props: %v\n", err)
		return
	}

	fmt.Println("=== MARKETS WITH LIQUIDITY ===\n")

	// Check points markets
	withLiquidity := 0
	noLiquidity := 0

	for propType, markets := range props {
		for _, m := range markets {
			book, err := client.GetOrderBook(m.Ticker)
			if err != nil {
				continue
			}

			totalYes := 0
			for _, level := range book.OrderBook.Yes {
				totalYes += level[1]
			}
			totalNo := 0
			for _, level := range book.OrderBook.No {
				totalNo += level[1]
			}

			if totalYes > 0 || totalNo > 0 {
				withLiquidity++
				if withLiquidity <= 10 {
					fmt.Printf("%s - %s %.0f+ (%s):\n", m.PlayerName, propType, m.Line, m.Ticker)
					fmt.Printf("  YES: %d contracts, NO: %d contracts\n", totalYes, totalNo)
					fmt.Printf("  Yes Bid: %d¢, Yes Ask: %d¢\n", m.YesBid, m.YesAsk)
					fmt.Println()
				}
			} else {
				noLiquidity++
			}
		}
	}

	fmt.Printf("\n=== SUMMARY ===\n")
	fmt.Printf("Markets WITH liquidity: %d\n", withLiquidity)
	fmt.Printf("Markets with NO liquidity: %d\n", noLiquidity)
}
