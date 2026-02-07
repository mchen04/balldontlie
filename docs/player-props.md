# Player Props Analysis

This document explains how the bot analyzes player prop markets and finds +EV opportunities.

## Overview

Player props (e.g., "LeBron James over 25.5 points") are analyzed by:
1. Collecting odds from multiple sportsbooks
2. Removing vig to estimate true probabilities
3. Comparing against Kalshi prices
4. Using interpolation when lines differ

---

## Supported Prop Types

| Prop Type | Kalshi Ticker | Example |
|-----------|---------------|---------|
| Points | `KXNBAPTS` | Player over/under 25.5 points |
| Rebounds | `KXNBAREB` | Player over/under 10.5 rebounds |
| Assists | `KXNBAAST` | Player over/under 8.5 assists |
| Threes | `KXNBA3PT` | Player over/under 3.5 three-pointers |
| Blocks | `KXNBABLK` | Player over/under 2.5 blocks |
| Steals | `KXNBASTL` | Player over/under 1.5 steals |

---

## Consensus Calculation

### Step 1: Collect Odds

Fetch odds from multiple sportsbooks (DraftKings, FanDuel, BetMGM, etc.):

```
DraftKings:  Over 25.5 (-115), Under 25.5 (-105)
FanDuel:    Over 25.5 (-110), Under 25.5 (-110)
BetMGM:     Over 25.5 (-120), Under 25.5 (+100)
```

### Step 2: Remove Vig (Power Method)

For each book, remove vig using the Power method to account for favorite-longshot bias:

```
DraftKings: Over 53.5%, Under 46.5%
FanDuel:    Over 50.0%, Under 50.0%
BetMGM:     Over 54.5%, Under 45.5%
```

### Step 3: Log-Linear Consensus

Probabilities are averaged in **logit space** (log-linear opinion pool) with vendor-specific weights (FanDuel 1.5x, DraftKings 1.2x, BetMGM 0.7x). Outliers are winsorized at ±2σ when 3+ books contribute.

```
logit(p) = log(p / (1-p))

avgLogit = Σ(logit(prob_i) × weight_i) / Σ(weight_i)
consensus = 1 / (1 + exp(-avgLogit))
```

This amplifies confident signals compared to arithmetic averaging.

---

## Line Interpolation

### The Problem

Sportsbooks and Kalshi often have different lines:
- BallDontLie: Player over 22.5 points
- Kalshi: Player over 20 points, over 25 points

We can't directly compare a 22.5-line probability to a 20-line price.

### The Solution: Negative Binomial Distribution

Player counting stats (points, rebounds, etc.) follow a **Negative Binomial distribution**, which models:
- A player's "true" scoring rate (mean)
- Game-to-game variance

### Interpolation Process

1. **Estimate player's expected stat** from the consensus line and probability
2. **Calculate probability at Kalshi's line** using the distribution

### Example

```
Consensus: Over 22.5 points at 55% true probability
Kalshi offers: Over 20 points at 70¢

Step 1: From 55% over 22.5, estimate expected points ≈ 23.5

Step 2: Calculate P(over 20) using Negative Binomial:
        With mean=23.5 and typical variance, P(>20) ≈ 72%

Step 3: Compare to Kalshi:
        True prob: 72%
        Kalshi price: 70%
        EV = 72% - 70% = +2%
```

### Distribution Models

**Points** use a normal distribution with CV calibrated by scoring volume:
```
mean > 25:  σ = 0.38 × mean
15-25:      σ = 0.35 × mean
≤ 15:       σ = 0.40 × mean
```

**Counting stats** (rebounds, assists, threes, blocks, steals) use a **Negative Binomial** distribution with prop-type-specific dispersion:
```
rebounds: r = 3.3 × mean  (Var ≈ 1.30 × mean)
assists:  r = 2.5 × mean  (Var ≈ 1.40 × mean)
threes:   r = 2.0 × mean  (Var ≈ 1.50 × mean)
steals:   r = 2.0 × mean  (Var ≈ 1.50 × mean)
blocks:   r = 1.5 × mean  (Var ≈ 1.67 × mean)
```

A **two-pass estimation** refines the SD/dispersion parameter: the first pass uses the book line as a proxy for the mean, then the inferred mean from pass 1 is used to recalibrate the dispersion for pass 2.

**CDF computation** for the negative binomial uses `P(X ≥ k) = 1 - I_p(r, k)` via the regularized incomplete beta function — O(1) instead of O(k) PMF summation.

### Multi-Line Logit Averaging

When multiple book lines are available for the same prop, each is independently shifted to Kalshi's line, then averaged in **logit space** for consistency with the consensus pooling method:

```
shiftedProb_i = EstimateProbabilityAtLine(bdlLine_i, bdlProb_i, kalshiLine)
result = sigmoid( Σ logit(shiftedProb_i) / count )
```

---

## Kalshi Ticker Format

Player prop tickers follow this pattern:
```
KXNBA{PROP}-{DATE}{AWAY}{HOME}-{TEAM}{PLAYER}{NUMBER}-{LINE}
```

### Components

| Part | Description | Example |
|------|-------------|---------|
| `KXNBA{PROP}` | Prop type | `KXNBAPTS` (points) |
| `{DATE}` | Game date | `26FEB05` (Feb 5, 2026) |
| `{AWAY}{HOME}` | Team matchup | `GSWPHX` (Warriors @ Suns) |
| `{TEAM}` | Player's team | `GSW` |
| `{PLAYER}` | Player code | `DGREEN23` |
| `{LINE}` | Stat line | `10` (over/under 10) |

### Full Example

```
KXNBAPTS-26FEB05GSWPHX-GSWDGREEN23-10

Decoded:
- Points prop
- Feb 5, 2026
- Warriors @ Suns
- Draymond Green (#23, Warriors)
- Over/Under 10 points
```

---

## Matching BallDontLie to Kalshi

### Player Name Matching

BallDontLie provides player IDs, Kalshi uses abbreviated names:

```go
// Normalize names for matching
"LeBron James" → "LJAMES"
"Giannis Antetokounmpo" → "GANTETOKOUNMPO"
```

### Line Matching Strategy

1. **Exact match**: If BDL line = Kalshi line, compare directly
2. **Interpolation**: If lines differ, use distribution to estimate probability at Kalshi's line
3. **Multiple Kalshi lines**: Compare against each available line, take best +EV

---

## EV Calculation for Props

Same formula as game markets:

```
rawEV = trueProb - kalshiPrice
fee = 0.07 × kalshiPrice × (1 - kalshiPrice), capped at 0.0175
adjustedEV = rawEV - fee
```

### Bayesian Shrinkage & Scaled Threshold

When fewer than 6 books contribute to the prop consensus, the same power-law shrinkage used for game markets is applied:

```
weight = (bookCount / 6) ^ 1.5
shrunk = weight × consensus + (1 - weight) × kalshiPrice
```

The EV threshold also scales: +1% per missing book below 6.

### Minimum Requirements

- At least 4 sportsbooks in consensus (`MinBookCount`)
- Adjusted EV ≥ scaled threshold (3% base + 1% per missing book below 6)
- Kalshi has liquidity at the line

---

## Example: Full Analysis

```
Player: Stephen Curry
Prop: Three-pointers

BallDontLie Consensus (6 books):
  Over 4.5 threes: 48% true probability

Kalshi Markets:
  Over 4 threes: 62¢
  Over 5 threes: 38¢

Interpolation:
  From 48% at 4.5, estimate Curry's expected 3PT ≈ 4.3

  P(over 4) using NegBinom(mean=4.3): 54%
  P(over 5) using NegBinom(mean=4.3): 35%

EV Calculations:
  Over 4: EV = 54% - 62% = -8% ❌
  Over 5: EV = 35% - 38% = -3% ❌

No +EV opportunity found.
```

---

## Risk Factors

1. **Injury news**: Prop odds move fast on injury reports
2. **Lineup changes**: Rest days, load management affect props
3. **Kalshi liquidity**: Player props often have thin books
4. **Model assumptions**: Distribution fits vary by player/stat

---

## References

- [Negative Binomial Distribution](https://en.wikipedia.org/wiki/Negative_binomial_distribution)
- [Player Props Betting Guide](https://www.actionnetwork.com/education/player-props)
