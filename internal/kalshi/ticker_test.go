package kalshi

import (
	"testing"
	"time"
)

func TestBuildNBATicker(t *testing.T) {
	tests := []struct {
		name       string
		series     KalshiSeries
		gameDate   time.Time
		awayTeam   string
		homeTeam   string
		wantTicker string
	}{
		{
			name:       "moneyline - Memphis @ Sacramento Feb 4 2026",
			series:     SeriesMoneyline,
			gameDate:   time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC),
			awayTeam:   "MEM",
			homeTeam:   "SAC",
			wantTicker: "KXNBAGAME-26FEB04MEMSAC",
		},
		{
			name:       "spread - Memphis @ Sacramento Feb 4 2026",
			series:     SeriesSpread,
			gameDate:   time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC),
			awayTeam:   "MEM",
			homeTeam:   "SAC",
			wantTicker: "KXNBASPREAD-26FEB04MEMSAC",
		},
		{
			name:       "total - New Orleans @ Milwaukee Feb 4 2026",
			series:     SeriesTotal,
			gameDate:   time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC),
			awayTeam:   "NOP",
			homeTeam:   "MIL",
			wantTicker: "KXNBATOTAL-26FEB04NOPMIL",
		},
		{
			name:       "player points - Philadelphia @ Golden State Feb 3 2026",
			series:     SeriesPlayerPoints,
			gameDate:   time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
			awayTeam:   "PHI",
			homeTeam:   "GSW",
			wantTicker: "KXNBAPTS-26FEB03PHIGSW",
		},
		{
			name:       "player rebounds - Lakers @ Nets Feb 3 2026",
			series:     SeriesPlayerRebounds,
			gameDate:   time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
			awayTeam:   "LAL",
			homeTeam:   "BKN",
			wantTicker: "KXNBAREB-26FEB03LALBKN",
		},
		{
			name:       "spread - December game",
			series:     SeriesSpread,
			gameDate:   time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC),
			awayTeam:   "BOS",
			homeTeam:   "LAL",
			wantTicker: "KXNBASPREAD-26DEC25BOSLAL",
		},
		{
			name:       "alternate team abbrev - GS maps to GSW",
			series:     SeriesMoneyline,
			gameDate:   time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			awayTeam:   "GS",
			homeTeam:   "LAL",
			wantTicker: "KXNBAGAME-26JAN15GSWLAL",
		},
		{
			name:       "invalid team returns empty",
			series:     SeriesSpread,
			gameDate:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			awayTeam:   "XXX",
			homeTeam:   "YY", // Only 2 chars - invalid
			wantTicker: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildNBATicker(tt.series, tt.gameDate, tt.awayTeam, tt.homeTeam)
			if got != tt.wantTicker {
				t.Errorf("BuildNBATicker() = %q, want %q", got, tt.wantTicker)
			}
		})
	}
}

func TestBuildPlayerPropTicker(t *testing.T) {
	tests := []struct {
		name       string
		propType   PropType
		gameDate   time.Time
		awayTeam   string
		homeTeam   string
		wantTicker string
	}{
		{
			name:       "points prop",
			propType:   PropPoints,
			gameDate:   time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
			awayTeam:   "DEN",
			homeTeam:   "DET",
			wantTicker: "KXNBAPTS-26FEB03DENDET",
		},
		{
			name:       "rebounds prop",
			propType:   PropRebounds,
			gameDate:   time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
			awayTeam:   "BOS",
			homeTeam:   "DAL",
			wantTicker: "KXNBAREB-26FEB03BOSDAL",
		},
		{
			name:       "assists prop",
			propType:   PropAssists,
			gameDate:   time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC),
			awayTeam:   "MIA",
			homeTeam:   "CHI",
			wantTicker: "KXNBAAST-26FEB04MIACHI",
		},
		{
			name:       "threes prop",
			propType:   PropThrees,
			gameDate:   time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC),
			awayTeam:   "GSW",
			homeTeam:   "NYK",
			wantTicker: "KXNBA3PT-26FEB04GSWNYK",
		},
		{
			name:       "invalid prop type",
			propType:   "",
			gameDate:   time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC),
			awayTeam:   "GSW",
			homeTeam:   "NYK",
			wantTicker: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildPlayerPropTicker(tt.propType, tt.gameDate, tt.awayTeam, tt.homeTeam)
			if got != tt.wantTicker {
				t.Errorf("BuildPlayerPropTicker() = %q, want %q", got, tt.wantTicker)
			}
		})
	}
}

func TestMapTeamToKalshi(t *testing.T) {
	tests := []struct {
		abbrev string
		want   string
	}{
		// Standard mappings
		{"LAL", "LAL"},
		{"BKN", "BKN"},
		{"GSW", "GSW"},
		{"NOP", "NOP"},
		{"PHX", "PHX"},

		// Alternate mappings
		{"GS", "GSW"},
		{"NO", "NOP"},
		{"NY", "NYK"},
		{"SA", "SAS"},
		{"PHO", "PHX"},
		{"BRK", "BKN"},

		// Invalid
		{"XX", ""},   // 2 chars
		{"XXXX", ""}, // 4 chars
	}

	for _, tt := range tests {
		t.Run(tt.abbrev, func(t *testing.T) {
			got := MapTeamToKalshi(tt.abbrev)
			if got != tt.want {
				t.Errorf("MapTeamToKalshi(%q) = %q, want %q", tt.abbrev, got, tt.want)
			}
		})
	}
}

func TestIsMaintenanceWindow(t *testing.T) {
	et, _ := time.LoadLocation("America/New_York")

	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{
			name: "Thursday 3:00 AM ET - in maintenance",
			t:    time.Date(2026, 2, 5, 3, 0, 0, 0, et), // Thursday
			want: true,
		},
		{
			name: "Thursday 4:30 AM ET - in maintenance",
			t:    time.Date(2026, 2, 5, 4, 30, 0, 0, et),
			want: true,
		},
		{
			name: "Thursday 2:59 AM ET - before maintenance",
			t:    time.Date(2026, 2, 5, 2, 59, 0, 0, et),
			want: false,
		},
		{
			name: "Thursday 5:00 AM ET - after maintenance",
			t:    time.Date(2026, 2, 5, 5, 0, 0, 0, et),
			want: false,
		},
		{
			name: "Wednesday 4:00 AM ET - wrong day",
			t:    time.Date(2026, 2, 4, 4, 0, 0, 0, et), // Wednesday
			want: false,
		},
		{
			name: "Thursday 12:00 PM ET - wrong time",
			t:    time.Date(2026, 2, 5, 12, 0, 0, 0, et),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMaintenanceWindow(tt.t)
			if got != tt.want {
				t.Errorf("IsMaintenanceWindow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSeriesForMarketType(t *testing.T) {
	tests := []struct {
		marketType string
		want       KalshiSeries
	}{
		{"moneyline", SeriesMoneyline},
		{"spread", SeriesSpread},
		{"total", SeriesTotal},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.marketType, func(t *testing.T) {
			got := GetSeriesForMarketType(tt.marketType)
			if got != tt.want {
				t.Errorf("GetSeriesForMarketType(%q) = %q, want %q", tt.marketType, got, tt.want)
			}
		})
	}
}

func TestGetSeriesForPropType(t *testing.T) {
	tests := []struct {
		propType PropType
		want     KalshiSeries
	}{
		{PropPoints, SeriesPlayerPoints},
		{PropRebounds, SeriesPlayerRebounds},
		{PropAssists, SeriesPlayerAssists},
		{PropThrees, SeriesPlayerThrees},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.propType), func(t *testing.T) {
			got := GetSeriesForPropType(tt.propType)
			if got != tt.want {
				t.Errorf("GetSeriesForPropType(%q) = %q, want %q", tt.propType, got, tt.want)
			}
		})
	}
}

func TestPropTypeFromBallDontLie(t *testing.T) {
	tests := []struct {
		bdlType string
		want    PropType
	}{
		{"points", PropPoints},
		{"rebounds", PropRebounds},
		{"assists", PropAssists},
		{"threes", PropThrees},
		{"blocks", ""},         // Not supported on Kalshi
		{"steals", ""},         // Not supported on Kalshi
		{"double_double", ""},  // Not supported on Kalshi
		{"triple_double", ""},  // Not supported on Kalshi
	}

	for _, tt := range tests {
		t.Run(tt.bdlType, func(t *testing.T) {
			got := PropTypeFromBallDontLie(tt.bdlType)
			if got != tt.want {
				t.Errorf("PropTypeFromBallDontLie(%q) = %q, want %q", tt.bdlType, got, tt.want)
			}
		})
	}
}

func TestIsKalshiSupportedProp(t *testing.T) {
	supported := []string{"points", "rebounds", "assists", "threes"}
	unsupported := []string{"blocks", "steals", "double_double", "triple_double", "points_rebounds", ""}

	for _, prop := range supported {
		if !IsKalshiSupportedProp(prop) {
			t.Errorf("IsKalshiSupportedProp(%q) = false, want true", prop)
		}
	}

	for _, prop := range unsupported {
		if IsKalshiSupportedProp(prop) {
			t.Errorf("IsKalshiSupportedProp(%q) = true, want false", prop)
		}
	}
}

func TestGetAllGameSeries(t *testing.T) {
	series := GetAllGameSeries()
	if len(series) != 3 {
		t.Errorf("GetAllGameSeries() returned %d series, want 3", len(series))
	}

	expected := map[KalshiSeries]bool{
		SeriesMoneyline: true,
		SeriesSpread:    true,
		SeriesTotal:     true,
	}

	for _, s := range series {
		if !expected[s] {
			t.Errorf("Unexpected series: %s", s)
		}
	}
}

func TestGetAllPlayerPropSeries(t *testing.T) {
	series := GetAllPlayerPropSeries()
	if len(series) != 4 {
		t.Errorf("GetAllPlayerPropSeries() returned %d series, want 4", len(series))
	}

	expected := map[KalshiSeries]bool{
		SeriesPlayerPoints:   true,
		SeriesPlayerRebounds: true,
		SeriesPlayerAssists:  true,
		SeriesPlayerThrees:   true,
	}

	for _, s := range series {
		if !expected[s] {
			t.Errorf("Unexpected series: %s", s)
		}
	}
}

func TestFormatKalshiDate(t *testing.T) {
	tests := []struct {
		date time.Time
		want string
	}{
		{time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC), "26FEB04"},
		{time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC), "26DEC25"},
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "26JAN01"},
		{time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC), "25NOV15"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatKalshiDate(tt.date)
			if got != tt.want {
				t.Errorf("FormatKalshiDate() = %q, want %q", got, tt.want)
			}
		})
	}
}
