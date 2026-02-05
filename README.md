# Sports Betting Bot

A 24/7 NBA betting analysis bot written in Go that identifies +EV opportunities on Kalshi by comparing prediction market prices against sportsbook consensus.

## How It Works

1. **Polls odds** from 14+ sportsbooks via balldontlie.io (600 req/min)
2. **Calculates consensus** probabilities with vig removal
3. **Compares against Kalshi** prices to find +EV opportunities
4. **Sizes bets** using Kelly criterion (quarter-Kelly default)
5. **Optionally executes** trades via Kalshi API with RSA authentication

## Features

- Multi-book consensus with multiplicative vig removal
- Spread/total line normalization using normal distribution (σ ≈ 11.5 for NBA)
- Fee-adjusted EV calculation (accounts for Kalshi's 1.2% fee)
- Order book depth and slippage analysis
- Arbitrage detection on existing positions
- SQLite position tracking with hedge alerts
- Deduped console alerts (5-min cooldown)

## Quick Start

```bash
# Clone
git clone https://github.com/mchen04/balldontlie.git
cd balldontlie

# Configure
cp .env.example .env
# Edit .env with your API keys

# Run
go run ./cmd/bot
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `BALLDONTLIE_API_KEY` | required | Ball Don't Lie API key |
| `KALSHI_API_KEY_ID` | optional | Kalshi API key ID |
| `KALSHI_API_KEY_PATH` | optional | Path to RSA private key (local dev) |
| `KALSHI_PRIVATE_KEY` | optional | RSA private key content (cloud deployment) |
| `EV_THRESHOLD` | 0.03 | Min EV to alert (3%) |
| `KELLY_FRACTION` | 0.25 | Fraction of full Kelly |
| `AUTO_EXECUTE` | false | Auto-execute trades |
| `MAX_BET_DOLLARS` | 0 | Max bet size (0 = no cap) |
| `KALSHI_DEMO` | false | Use demo API |

See [docs/architecture.md](docs/architecture.md) for full configuration options.

## Documentation

- **[Architecture](docs/architecture.md)** - System design and configuration
- **[Math](docs/math.md)** - EV calculation, Kelly criterion, vig removal
- **[Player Props](docs/player-props.md)** - Props analysis and line interpolation

## Project Structure

```
├── cmd/bot/           # Entry point
├── internal/
│   ├── api/           # balldontlie.io client
│   ├── kalshi/        # Kalshi API, orderbook, orders
│   ├── odds/          # Consensus, conversion, vig removal
│   ├── analysis/      # EV detection, Kelly sizing
│   ├── positions/     # SQLite tracking, hedge detection
│   └── alerts/        # Notification system
├── Dockerfile         # Multi-stage build
└── fly.toml           # Fly.io deployment
```

## Deployment

Deploy to Fly.io:

```bash
fly launch
fly secrets set BALLDONTLIE_API_KEY=xxx KALSHI_API_KEY_ID=xxx
fly deploy
```

## Data Sources

- **[balldontlie.io](https://balldontlie.io)** - GOAT tier ($39.99/mo) for 14+ sportsbook odds
- **[Kalshi](https://kalshi.com)** - Prediction market prices and execution

## License

MIT
