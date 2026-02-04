package kalshi

import (
	"math"
	"testing"
)

func TestCalculateSlippage(t *testing.T) {
	client := &KalshiClient{}

	tests := []struct {
		name       string
		book       *OrderBookResponse
		side       Side
		action     OrderAction
		contracts  int
		wantFilled int
		wantAvg    float64
		wantSlip   float64
		wantOK     bool
	}{
		{
			name: "single level, full fill, no slippage",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					Yes: [][2]int{},
					No:  [][2]int{{40, 100}}, // YES offer at 60
				},
			},
			side:       SideYes,
			action:     ActionBuy,
			contracts:  50,
			wantFilled: 50,
			wantAvg:    60.0,
			wantSlip:   0.0,
			wantOK:     true,
		},
		{
			name: "multiple levels, partial fill with slippage",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					Yes: [][2]int{},
					No: [][2]int{
						{40, 10}, // YES offer at 60
						{35, 10}, // YES offer at 65
						{30, 10}, // YES offer at 70
					},
				},
			},
			side:       SideYes,
			action:     ActionBuy,
			contracts:  20,
			wantFilled: 20,
			wantAvg:    62.5, // (10*60 + 10*65) / 20
			wantSlip:   0.0416, // (62.5 - 60) / 60 = 0.0416
			wantOK:     false, // > 2%
		},
		{
			name: "empty book",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					Yes: [][2]int{},
					No:  [][2]int{},
				},
			},
			side:       SideYes,
			action:     ActionBuy,
			contracts:  10,
			wantFilled: 0,
			wantAvg:    0,
			wantSlip:   0,
			wantOK:     false,
		},
		{
			name: "selling YES uses YES bids",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					Yes: [][2]int{{55, 100}},
					No:  [][2]int{},
				},
			},
			side:       SideYes,
			action:     ActionSell,
			contracts:  30,
			wantFilled: 30,
			wantAvg:    55.0,
			wantSlip:   0.0,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.CalculateSlippage(tt.book, tt.side, tt.action, tt.contracts)

			if result.FillableContracts != tt.wantFilled {
				t.Errorf("FillableContracts = %d, want %d", result.FillableContracts, tt.wantFilled)
			}

			if math.Abs(result.AverageFillPrice-tt.wantAvg) > 0.01 {
				t.Errorf("AverageFillPrice = %.2f, want %.2f", result.AverageFillPrice, tt.wantAvg)
			}

			if math.Abs(result.SlippagePct-tt.wantSlip) > 0.01 {
				t.Errorf("SlippagePct = %.4f, want %.4f", result.SlippagePct, tt.wantSlip)
			}

			if result.Acceptable != tt.wantOK {
				t.Errorf("Acceptable = %v, want %v", result.Acceptable, tt.wantOK)
			}
		})
	}
}

func TestCheckLiquidity(t *testing.T) {
	client := &KalshiClient{}

	tests := []struct {
		name        string
		book        *OrderBookResponse
		side        Side
		action      OrderAction
		contracts   int
		wantAvail   int
		wantSuff    bool
	}{
		{
			name: "sufficient liquidity",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					No: [][2]int{{40, 50}},
				},
			},
			side:      SideYes,
			action:    ActionBuy,
			contracts: 10,
			wantAvail: 50,
			wantSuff:  true,
		},
		{
			name: "insufficient liquidity",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					No: [][2]int{{40, 5}},
				},
			},
			side:      SideYes,
			action:    ActionBuy,
			contracts: 10,
			wantAvail: 5,
			wantSuff:  false,
		},
		{
			name: "empty book",
			book: &OrderBookResponse{
				OrderBook: OrderBookInner{
					Yes: [][2]int{},
					No:  [][2]int{},
				},
			},
			side:      SideYes,
			action:    ActionBuy,
			contracts: 10,
			wantAvail: 0,
			wantSuff:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.CheckLiquidity(tt.book, tt.side, tt.action, tt.contracts)

			if result.Available != tt.wantAvail {
				t.Errorf("Available = %d, want %d", result.Available, tt.wantAvail)
			}

			if result.Sufficient != tt.wantSuff {
				t.Errorf("Sufficient = %v, want %v", result.Sufficient, tt.wantSuff)
			}
		})
	}
}

func TestConvertNoBidsToYesOffers(t *testing.T) {
	noBids := []OrderBookLevel{
		{Price: 40, Count: 10}, // NO bid at 40 = YES offer at 60
		{Price: 30, Count: 20}, // NO bid at 30 = YES offer at 70
	}

	offers := convertNoBidsToYesOffers(noBids)

	if len(offers) != 2 {
		t.Fatalf("Expected 2 offers, got %d", len(offers))
	}

	if offers[0].Price != 60 || offers[0].Count != 10 {
		t.Errorf("First offer = {%d, %d}, want {60, 10}", offers[0].Price, offers[0].Count)
	}

	if offers[1].Price != 70 || offers[1].Count != 20 {
		t.Errorf("Second offer = {%d, %d}, want {70, 20}", offers[1].Price, offers[1].Count)
	}
}

func TestGetOptimalSize(t *testing.T) {
	client := &KalshiClient{}

	book := &OrderBookResponse{
		OrderBook: OrderBookInner{
			No: [][2]int{
				{40, 50},  // YES offer at 60
				{35, 50},  // YES offer at 65
				{30, 100}, // YES offer at 70
			},
		},
	}

	// With 2% max slippage starting from 60, we can fill some contracts at 60
	// but not too many at higher prices
	optimal := client.GetOptimalSize(book, SideYes, ActionBuy, 0.02)

	// Should get at least the first level
	if optimal < 50 {
		t.Errorf("Optimal size = %d, expected at least 50", optimal)
	}
}
