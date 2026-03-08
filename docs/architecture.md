# Sports Betting Bot - Architecture

## Overview

A 24/7 NBA betting analysis bot written in **Go** that identifies +EV opportunities on Kalshi by comparing prediction market prices against sportsbook consensus.

**Key constraint**: Only betting on Kalshi. Traditional sportsbooks are data sources only.

## Project Structure

```
sports-betting-bot/
├── cmd/bot/                    # Entry point
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
│   │   ├── consensus.go        # Multi-book consensus with staleness filtering
│   │   ├── convert.go          # Odds format conversion
│   │   └── vig.go              # Vig removal
│   ├── analysis/               # +EV detection & sizing
│   │   ├── ev.go               # Opportunity finder, longshot filter
│   │   ├── kelly.go            # Kelly criterion
│   │   ├── player_props.go     # Player prop analysis with staleness filtering
│   │   └── distribution.go     # Distribution interpolation
│   ├── mathutil/               # Math primitives
│   │   ├── logit.go            # Logit/sigmoid, winsorization
│   │   └── tdist.go            # Student's t, beta functions
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

### 2. Staleness Filtering
- **Game odds**: Vendors whose `UpdatedAt` is older than `MaxOddsAgeSec` (default 30s) are excluded from consensus via `isVendorFresh()`
- **Player props**: `filterFreshProps()` removes all props older than `MaxOddsAgeSec` before consensus calculation
- Unparseable timestamps default to **stale** (excluded)
- This ensures the bot only trades on genuinely fresh data

### 3. Consensus Calculation
- Converts American odds to implied probabilities
- Removes vig using Power method (accounts for favorite-longshot bias)
- Combines vig-free probabilities via log-linear opinion pool (logit-space averaging) with winsorized outlier capping
- Normalizes spread/total probabilities to Kalshi's line using Student's t-distribution with context-dependent SD
- Applies Bayesian shrinkage toward Kalshi prior when book count < 6

### 4. Opportunity Detection
- Compares consensus "true" probability against Kalshi price
- **Longshot filter**: Skips any bet where true probability < 15% or > 85%
- Calculates fee-adjusted EV (accounts for Kalshi's dynamic fee: `0.07 * price * (1-price)`, capped at $0.0175)
- Filters opportunities by configurable EV threshold (default 3%)
- Computes Kelly criterion bet sizing (default quarter-Kelly)

### 5. Order Execution (optional)
- RSA-PSS signed authentication with Kalshi API
- Order book depth and liquidity checks
- Slippage calculation before execution
- Market and limit order support

### 6. Position Tracking & Hedging
- SQLite database stores Kalshi positions
- Monitors for arbitrage opportunities on held positions
- Alerts when hedging can lock in guaranteed profit

### 7. Duplicate Prevention
- In-flight TTL lock (30s) prevents race conditions between concurrent poll cycles
- Positions stored in SQLite **after** successful order fill (prevents stale entries from failed orders)
- Each position tracked by full Kalshi ticker + bet side (yes/no)
- Prevents duplicate bets across scans and across restarts

### 8. Player Props Analysis
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
│  │              Staleness Filter                    │            │
│  │  • Reject vendor odds older than 30s             │            │
│  │  • Reject props with unparseable timestamps      │            │
│  └─────────────────────────┬───────────────────────┘            │
│                            │                                     │
│                            ▼                                     │
│  ┌─────────────────────────────────────────────────┐            │
│  │              Odds Processor                      │            │
│  │  • American → Implied probability                │            │
│  │  • Vig removal (Power method)                    │            │
│  │  • Line normalization (t-distribution CDF)       │            │
│  │  • Log-linear consensus with winsorization       │            │
│  └─────────────────────────┬───────────────────────┘            │
│                            │                                     │
│                            ▼                                     │
│  ┌─────────────────────────────────────────────────┐            │
│  │            Opportunity Finder                    │            │
│  │  • Longshot filter (15-85% probability range)    │            │
│  │  • Fee-adjusted EV calculation                   │            │
│  │  • Bayesian shrinkage + scaled threshold         │            │
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

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `EV_THRESHOLD` | 3% | Minimum adjusted EV to alert |
| `KELLY_FRACTION` | 25% | Fraction of full Kelly |
| `POLL_INTERVAL_MS` | 2000ms | Time between API polls |
| `MAX_ODDS_AGE_SEC` | 30 | Max vendor odds age in seconds |
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

**Supported Books**: DraftKings, FanDuel, BetMGM, Caesars, BetRivers, Betway, Fanatics, BetParx, Kalshi, Polymarket, and more

**Markets**: Moneyline, spread, totals, player props

## Risk Factors

1. **BDL data freshness** — BDL claims "real-time" but empirically updates every 30-60+ minutes; 30s staleness filter means the bot only trades when it catches freshly-updated data
2. **Odds movement** — Lines may move before execution
3. **Kalshi liquidity** — May not get filled at displayed price
4. **Model assumptions** — Normal/NegBin distributions are approximations
5. **Longshot bias** — Props below 15% true probability are filtered out due to systematic overpricing

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
