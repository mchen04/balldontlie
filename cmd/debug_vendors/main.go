package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"sports-betting-bot/internal/api"
)

func main() {
	_ = godotenv.Load()
	client := api.NewBallDontLieClient(os.Getenv("BALLDONTLIE_API_KEY"))

	gameOdds, _ := client.GetTodaysOdds()

	if len(gameOdds) > 0 {
		game := gameOdds[0]
		fmt.Printf("Game: %s @ %s\n", game.Game.VisitorTeam.Abbreviation, game.Game.HomeTeam.Abbreviation)
		fmt.Println("\nVendors with GAME odds:")
		for _, v := range game.Vendors {
			hasML := v.Moneyline != nil
			hasSpread := v.Spread != nil
			hasTotal := v.Total != nil
			isKalshi := api.IsKalshi(v.Name)
			marker := ""
			if isKalshi {
				marker = " <-- KALSHI"
			}
			fmt.Printf("  %-15s ML=%v Spread=%v Total=%v%s\n", v.Name, hasML, hasSpread, hasTotal, marker)
		}
	}
}
