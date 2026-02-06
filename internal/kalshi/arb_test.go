package kalshi

import (
	"math"
	"testing"
)

func TestAnalyzeArbFromBook(t *testing.T) {
	config := DefaultArbConfig()

	tests := []struct {
		name       string
		book       *OrderBookResponse
		wantArb    bool
		wantProfit float64 // approximate cents per contract
	}{
		{
			name: "clear arb opportunity",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					// NO bid at 55 = YES ask at 45
					No: [][2]int{{55, 100}},
					// YES bid at 60 = NO ask at 40
					Yes: [][2]int{{60, 100}},
				},
			},
			wantArb:    true,
			wantProfit: 11.6, // 100 - 45 - 40 - TakerFee(45) - TakerFee(40) ≈ 11.6 cents
		},
		{
			name: "no arb - prices too high",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					// NO bid at 45 = YES ask at 55
					No: [][2]int{{45, 100}},
					// YES bid at 40 = NO ask at 60
					Yes: [][2]int{{40, 100}},
				},
			},
			wantArb:    false,
			wantProfit: 0,
		},
		{
			name: "marginal arb eaten by fees",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					// NO bid at 51 = YES ask at 49
					No: [][2]int{{51, 100}},
					// YES bid at 50 = NO ask at 50
					Yes: [][2]int{{50, 100}},
				},
			},
			wantArb:    false, // 100 - 49 - 50 - fees < 0.5
			wantProfit: 0,
		},
		{
			name: "empty order book",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					No:  [][2]int{},
					Yes: [][2]int{},
				},
			},
			wantArb:    false,
			wantProfit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arb := AnalyzeArbFromBook(tt.book, "TEST-TICKER", config)

			if tt.wantArb {
				if arb == nil {
					t.Fatal("Expected arb opportunity, got nil")
				}
				if math.Abs(arb.GuaranteedProfit-tt.wantProfit) > 1.0 {
					t.Errorf("Profit = %.2f, want ~%.2f", arb.GuaranteedProfit, tt.wantProfit)
				}
			} else {
				if arb != nil {
					t.Errorf("Expected no arb, got %+v", arb)
				}
			}
		})
	}
}

func TestCalculateArbProfit(t *testing.T) {
	tests := []struct {
		name      string
		yesPrice  int
		noPrice   int
		contracts int
		wantProfit float64
	}{
		{
			name:       "basic arb 45+40",
			yesPrice:   45,
			noPrice:    40,
			contracts:  10,
			// Fee: TakerFeeCents(45) = 0.07*0.45*0.55*100 = 1.7325, TakerFeeCents(40) = 0.07*0.40*0.60*100 = 1.68
			// Total fees = (1.7325 + 1.68) * 10 = 34.125
			// Profit = 1000 - 450 - 400 - 34.125 = 115.875
			wantProfit: 1000 - 450 - 400 - (TakerFeeCents(45)+TakerFeeCents(40))*10,
		},
		{
			name:       "no profit arb 55+50",
			yesPrice:   55,
			noPrice:    50,
			contracts:  10,
			wantProfit: 1000 - 550 - 500 - (TakerFeeCents(55)+TakerFeeCents(50))*10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profit := CalculateArbProfit(tt.yesPrice, tt.noPrice, tt.contracts)
			if math.Abs(profit-tt.wantProfit) > 1.0 {
				t.Errorf("Profit = %.2f, want %.2f", profit, tt.wantProfit)
			}
		})
	}
}

func TestGetBestAskFromNoBids(t *testing.T) {
	tests := []struct {
		name      string
		noBids    []OrderBookLevel
		wantAsk   int
		wantDepth int
	}{
		{
			name:      "single bid",
			noBids:    []OrderBookLevel{{Price: 40, Count: 50}},
			wantAsk:   60, // 100 - 40
			wantDepth: 50,
		},
		{
			name: "multiple bids, take best",
			noBids: []OrderBookLevel{
				{Price: 40, Count: 50},
				{Price: 45, Count: 30}, // Best (highest NO bid = lowest YES ask)
				{Price: 35, Count: 100},
			},
			wantAsk:   55, // 100 - 45
			wantDepth: 30,
		},
		{
			name:      "empty",
			noBids:    []OrderBookLevel{},
			wantAsk:   0,
			wantDepth: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ask, depth := getBestAskFromNoBids(tt.noBids)
			if ask != tt.wantAsk {
				t.Errorf("ask = %d, want %d", ask, tt.wantAsk)
			}
			if depth != tt.wantDepth {
				t.Errorf("depth = %d, want %d", depth, tt.wantDepth)
			}
		})
	}
}

func TestDetectPositionArb(t *testing.T) {
	config := DefaultArbConfig()

	tests := []struct {
		name       string
		entryPrice float64
		entrySide  Side
		book       *OrderBookResponse
		wantArb    bool
	}{
		{
			name:       "arb exists - bought YES cheap, NO is cheap too",
			entryPrice: 0.40, // Bought YES at 40 cents
			entrySide:  SideYes,
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					// YES bid at 65 = NO ask at 35
					Yes: [][2]int{{65, 50}},
				},
			},
			wantArb: true, // 40 + 35 = 75 < 100
		},
		{
			name:       "no arb - prices moved against us",
			entryPrice: 0.50, // Bought YES at 50 cents
			entrySide:  SideYes,
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					// YES bid at 45 = NO ask at 55
					Yes: [][2]int{{45, 50}},
				},
			},
			wantArb: false, // 50 + 55 = 105 > 100
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arb := DetectPositionArb(tt.entryPrice, tt.entrySide, tt.book, config)
			if tt.wantArb && arb == nil {
				t.Error("Expected arb, got nil")
			}
			if !tt.wantArb && arb != nil {
				t.Errorf("Expected no arb, got %+v", arb)
			}
		})
	}
}
