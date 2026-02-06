# Sports Betting Bot - Architecture

## Overview

A 24/7 NBA betting analysis bot written in **Go** that identifies +EV opportunities on Kalshi by comparing prediction market prices against sportsbook consensus.

**Key constraint**: Only betting on Kalshi. Traditional sportsbooks are data sources only.

## Project Structure

```
sports-betting-bot/
├── cmd/bot/                    # Entry point (~155 lines)
│   └── main.go                 # Init, config, startup
├── internal/
│   ├── config/                 # Configuration management
│   │   ├── config.go           # Load, Validate, named constants
│   │   └── config_test.go      # Config tests
│   ├── engine/                 # Core orchestration
│   │   ├── engine.go           # Polling loop, scan cycle
│   │   ├── executor.go         # Unified trade execution
│   │   ├── executor_test.go    # Executor tests
│   │   └── ticker.go           # Ticker mapping
│   ├── api/                    # External API clients
│   │   ├── client.go           # Rate-limited HTTP client (600 req/min)
│   │   └── balldontlie.go      # Ball Don't Lie API integration
│   ├── kalshi/                 # Kalshi market integration
│   │   ├── client.go           # RSA-signed API client
│   │   ├── types.go            # Data structures & enums
│   │   ├── markets.go          # Market utilities
│   │   ├── orders.go           # Order execution
│   │   ├── orderbook.go        # Order book analysis
│   │   ├── ticker.go           # Ticker generation (KXNBA*)
│   │   └── arb.go              # Arbitrage detection
│   ├── odds/                   # Probability calculations
│   │   ├── consensus.go        # Multi-book consensus
│   │   ├── convert.go          # Odds format conversion
│   │   └── vig.go              # Vig removal
│   ├── analysis/               # +EV detection & sizing
│   │   ├── ev.go               # Opportunity finder
│   │   ├── kelly.go            # Kelly criterion
│   │   └── player_props.go     # Player prop analysis
│   ├── positions/              # Position management
│   │   ├── db.go               # SQLite storage
│   │   └── hedge.go            # Hedge detection
│   └── alerts/                 # Notification system
│       ├── notify.go           # Deduped console alerts
│       └── notify_test.go      # Alert tests
├── docs/                       # Documentation
├── Dockerfile                  # Multi-stage build
├── fly.toml                    # Fly.io deployment
└── go.mod                      # Go 1.25.6
```

## How It Works

### 1. Data Collection
- Polls balldontlie.io API at 600 requests/minute (GOAT tier)
- Fetches odds from 14+ sportsbooks including Kalshi
- Handles pagination for busy NBA days
- Automatic retry with exponential backoff on failures

### 2. Consensus Calculation
- Converts American odds to implied probabilities
- Removes vig using multiplicative method (probabilities sum to 100%)
- Averages vig-free probabilities across all books
- Normalizes spread/total probabilities to Kalshi's line using normal distribution (σ ≈ 11.5 for NBA)

### 3. Opportunity Detection
- Compares consensus "true" probability against Kalshi price
- Auto-detects Kalshi price format (American odds vs cents)
- Calculates fee-adjusted EV (accounts for Kalshi's dynamic fee: `0.07 * price * (1-price)`, capped at $0.0175)
- Filters opportunities by configurable EV threshold (default 3%)
- Computes Kelly criterion bet sizing (default quarter-Kelly)

### 4. Order Execution (optional)
- RSA-PSS signed authentication with Kalshi API
- Order book depth and liquidity checks
- Slippage calculation before execution
- Market and limit order support

### 5. Position Tracking & Hedging
- SQLite database stores Kalshi positions
- Monitors for arbitrage opportunities on held positions
- Alerts when hedging can lock in guaranteed profit

### 6. Duplicate Prevention
- Positions stored in SQLite **before** order placement
- Each position tracked by full Kalshi ticker + bet side (yes/no)
- Prevents duplicate bets across scans and across restarts
- Ticker includes date, so next day's bets are not blocked

### 7. Player Props Analysis
- Matches BallDontLie player props with Kalshi markets
- Uses interpolation to compare different lines (e.g., BDL 22.5 pts vs Kalshi 20 pts)
- Supports points, rebounds, assists, threes, blocks, steals
- Negative binomial distribution for counting stats modeling

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                        Fly.io (24/7)                             │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────┐    ┌────────────────────┐                 │
│  │  Rate Limiter    │───▶│  balldontlie.io    │                 │
│  │  (600 req/min)   │    │  API Client        │                 │
│  └──────────────────┘    └─────────┬──────────┘                 │
│                                    │                             │
│                                    ▼                             │
│  ┌─────────────────────────────────────────────────┐            │
│  │              Odds Processor                      │            │
│  │  • American → Implied probability                │            │
│  │  • Vig removal (multiplicative)                  │            │
│  │  • Line normalization (normal CDF)               │            │
│  │  • Consensus averaging                           │            │
│  └─────────────────────────┬───────────────────────┘            │
│                            │                                     │
│                            ▼                                     │
│  ┌─────────────────────────────────────────────────┐            │
│  │            Opportunity Finder                    │            │
│  │  • Kalshi vs consensus comparison                │            │
│  │  • Fee-adjusted EV calculation                   │            │
│  │  • Kelly criterion sizing                        │            │
│  │  • Arbitrage detection                           │            │
│  └─────────────────────────┬───────────────────────┘            │
│                            │                                     │
│         ┌──────────────────┼──────────────────┐                 │
│         ▼                  ▼                  ▼                  │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────────┐        │
│  │ Alert       │   │ SQLite DB   │   │ Kalshi Client   │        │
│  │ System      │   │ (Positions) │   │ (RSA Auth)      │        │
│  └─────────────┘   └─────────────┘   └─────────────────┘        │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

## Key Modules

### `internal/config` - Configuration
- **Config**: Loads from `.env` and environment variables with named constants for all defaults
- **Validate**: Range-checks all config values before startup

### `internal/engine` - Orchestration
- **Engine**: Main polling loop, scan cycle, shutdown handling (uses `log/slog`)
- **Executor**: Unified trade execution for both game and player prop opportunities
- **Ticker**: Maps opportunities to Kalshi market tickers

### `internal/api` - Data Sources
- **RateLimitedClient**: Token bucket rate limiting with exponential backoff
- **BallDontLieClient**: Fetches today's odds, player props, handles pagination

### `internal/kalshi` - Market Integration
- **Client**: RSA-PSS signed requests, balance/positions/orders
- **OrderBook**: Parses `[[price, count], ...]` format, calculates fill prices
- **Ticker**: Generates NBA tickers (`KXNBAGAME-26FEB04MEMSAC`)
- **Arb**: Detects and executes guaranteed-profit opportunities

### `internal/odds` - Probability Engine
- **Consensus**: Multi-book aggregation with line normalization
- **Convert**: American ↔ Decimal ↔ Implied probability
- **Vig**: Multiplicative vig removal

### `internal/analysis` - Decision Engine
- **EV**: Finds +EV opportunities across moneyline, spread, totals, player props
- **Kelly**: Conservative quarter-Kelly position sizing

### `internal/positions` - State Management
- **DB**: SQLite schema for position tracking
- **Hedge**: Monitors for profitable exit opportunities

## Key Algorithms

### Spread Line Normalization
When books have different spread lines (e.g., -5.5 vs -6.0), probabilities are normalized to Kalshi's line:
- NBA margin vs spread follows N(0, σ) where σ ≈ 11.5 points
- Each half-point adjustment ≈ 1.7% probability change
- Uses standard normal CDF for conversion

### EV Calculation
```
Raw EV = (trueProb × profit) - ((1 - trueProb) × stake)
Fee = 0.07 × price × (1 - price), capped at $0.0175
Adjusted EV = Raw EV - Fee
```

### Kelly Criterion
```
f* = (p × b - q) / b     [full Kelly]
f  = f* × 0.25           [quarter-Kelly]
```

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `EV_THRESHOLD` | 3% | Minimum adjusted EV to alert |
| `KELLY_FRACTION` | 25% | Fraction of full Kelly |
| `POLL_INTERVAL_MS` | 2000ms | Time between API polls |
| `AUTO_EXECUTE` | false | Auto-execute trades on Kalshi |
| `MAX_SLIPPAGE_PCT` | 2% | Max acceptable slippage |
| `MIN_LIQUIDITY_CONTRACTS` | 1 | Min order book depth |
| `MAX_BET_DOLLARS` | 0 | Max bet size per trade (0 = no cap) |
| `KALSHI_DEMO` | false | Use Kalshi demo API |

## Deployment

- **Platform**: Fly.io with persistent volume for SQLite
- **Instance**: shared-cpu-1x, 256MB RAM
- **Health check**: `/health` endpoint every 30s
- **Region**: Chicago (ord) - close to NBA action
- **Build**: Multi-stage Docker (Go build → Alpine runtime)

## Data Sources

| Source | Purpose | Rate Limit |
|--------|---------|-----------|
| **balldontlie.io** | 14+ sportsbook odds | 600 req/min |
| **Kalshi API** | Prices & execution | 10 write/sec, 100 read/sec |

**Supported Books**: DraftKings, FanDuel, BetMGM, Caesars, Pinnacle, Circa, Kalshi, and more

**Markets**: Moneyline, spread, totals, player props

## Risk Factors

1. **Odds movement** - Lines may move before execution
2. **API latency** - Real-time means seconds, not milliseconds
3. **Kalshi liquidity** - May not get filled at displayed price
4. **Model assumptions** - Normal distribution is approximation
5. **Sharp book availability** - Consensus quality depends on data

## Further Documentation

- **[math.md](math.md)** - Core mathematical foundations (EV, Kelly, vig removal)
- **[player-props.md](player-props.md)** - Player props analysis and line interpolation

## References

- [Boyd's Bets - NBA Key Numbers](https://www.boydsbets.com/nba-key-numbers/)
- [Boyd's Bets - Standard Deviations](https://www.boydsbets.com/ats-margin-standard-deviations-by-point-spread/)
- [Kalshi Fees](https://help.kalshi.com/trading/fees)
- [Kalshi API Docs](https://trading-api.readme.io/reference/getting-started)
- [Kelly Criterion (Wikipedia)](https://en.wikipedia.org/wiki/Kelly_criterion)
- [Negative Binomial Distribution](https://en.wikipedia.org/wiki/Negative_binomial_distribution)
