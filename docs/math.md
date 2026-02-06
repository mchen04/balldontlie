# Mathematical Foundations

This document explains the core mathematical concepts used by the sports betting bot.

## Table of Contents
1. [Odds Conversion](#odds-conversion)
2. [Vig Removal](#vig-removal)
3. [Expected Value (EV)](#expected-value-ev)
4. [Kelly Criterion](#kelly-criterion)
5. [Line Normalization](#line-normalization)

---

## Odds Conversion

### American to Implied Probability

American odds come in two forms:

**Favorites (negative odds):**
```
impliedProb = |odds| / (|odds| + 100)

Example: -150 → 150 / 250 = 0.60 (60%)
```

**Underdogs (positive odds):**
```
impliedProb = 100 / (odds + 100)

Example: +130 → 100 / 230 = 0.435 (43.5%)
```

### Kalshi Price Format

Kalshi displays prices in cents (1-99). These directly represent probability:
```
price = 65 → 65% implied probability
```

---

## Vig Removal

Sportsbooks build in a profit margin (vig/juice) by making both sides sum to >100%.

### Example with Vig
```
Home: -110 → 52.4% implied
Away: -110 → 52.4% implied
Total: 104.8% (4.8% is the vig)
```

### Power Method (Default)

Accounts for favorite-longshot bias (longshots are systematically overbet). Used for all markets: moneyline, spread, total, and player props.

Finds exponent `k` such that `p1^k + p2^k = 1`:

```
trueProb_A = implied_A^k
trueProb_B = implied_B^k
```

This deflates longshot probabilities more than favorites. Empirically the best-performing devig method per Clarke & Kovalchik (2017), outperforming multiplicative, additive, and Shin methods across multiple datasets.

**Example:**
```
implied_A = 0.80 (favorite)
implied_B = 0.30 (longshot)
total = 1.10

Using bisection to find k ≈ 1.15:
trueProb_A = 0.80^1.15 = 0.77
trueProb_B = 0.30^1.15 = 0.23
Sum = 1.00 ✓
```

### Multiplicative Method (Alternative)

Proportionally scales probabilities to sum to 100%. Simpler but less accurate — over-adjusts longshots and under-adjusts favorites.

```
trueProb_A = implied_A / (implied_A + implied_B)
trueProb_B = implied_B / (implied_A + implied_B)
```

**Example:**
```
implied_A = 0.524, implied_B = 0.524
total = 1.048

trueProb_A = 0.524 / 1.048 = 0.50 (50%)
trueProb_B = 0.524 / 1.048 = 0.50 (50%)
```

---

## Expected Value (EV)

EV measures the average profit/loss per dollar wagered over time.

### Basic Formula

```
EV = (prob_win × profit) - (prob_lose × stake)
```

### For Kalshi Contracts

On Kalshi, you buy contracts at price `p` that pay $1 if the event happens:
- **Stake:** `p` (the price you pay)
- **Profit if win:** `1 - p`
- **Probability of win:** `trueProb`

```
rawEV = (trueProb × (1 - price)) - ((1 - trueProb) × price)
```

Simplified:
```
rawEV = trueProb - price
```

### Fee-Adjusted EV

Kalshi charges a dynamic taker fee based on the contract price:

```
fee = 0.07 × price × (1 - price), capped at $0.0175 per contract
```

This means fees are highest at 50¢ (max $0.0175) and decrease towards 0¢ or 100¢.

```
adjustedEV = rawEV - fee
```

### Example Calculation

```
True probability: 55%
Kalshi price: 50¢

rawEV = 0.55 - 0.50 = 0.05 (5%)
fee = 0.07 × 0.50 × 0.50 = 0.0175 (1.75%)
adjustedEV = 0.05 - 0.0175 = 0.0325 (3.25%)
```

This bet has +3.25% EV after fees.

---

## Kelly Criterion

Kelly determines the optimal bet size to maximize long-term growth.

### Fee-Adjusted Formula

The Kelly formula is adjusted for Kalshi's taker fee to avoid overbetting:

```
f* = (p × bNet - q) / bNet
```

Where:
- `f*` = fraction of bankroll to bet
- `p` = probability of winning
- `q` = probability of losing (1 - p)
- `bNet` = fee-adjusted odds = effectiveProfit / effectiveStake

### For Kalshi

```
fee = 0.07 × price × (1 - price), capped at $0.0175
effectiveStake = price + fee
effectiveProfit = 1 - price - fee
bNet = effectiveProfit / effectiveStake

f* = (trueProb × bNet - (1 - trueProb)) / bNet
```

### Fractional Kelly

Full Kelly is aggressive and leads to high variance. We use **quarter Kelly** (25%):

```
f = f* × 0.25
```

This reduces variance while still capturing most of the growth.

### Example

```
True probability: 55%
Kalshi price: 50¢

fee = 0.07 × 0.50 × 0.50 = 0.0175
effectiveStake = 0.50 + 0.0175 = 0.5175
effectiveProfit = 1.0 - 0.50 - 0.0175 = 0.4825
bNet = 0.4825 / 0.5175 = 0.9324

f* = (0.55 × 0.9324 - 0.45) / 0.9324 = 0.0674 (6.74%)
f = 0.0674 × 0.25 = 0.0168 (1.68%)

With $100 bankroll: bet $1.68
```

Without fee adjustment, Kelly would suggest 2.5% (raw b=1.0), a 49% overbet.

### Converting to Contracts

```
betSize = bankroll × kellyFraction
contracts = (betSize × 100) / priceInCents

Example:
betSize = $5
price = 50¢
contracts = 500 / 50 = 10 contracts
```

---

## Line Normalization

When sportsbooks offer different lines (e.g., spread -5.5 vs -6.0), we normalize probabilities to compare against Kalshi's line.

### NBA Point Spread Distribution

NBA margin of victory vs. the spread follows approximately:
```
N(0, σ) where σ ≈ 11.5 points (spreads)
```

For totals (over/under), a wider standard deviation is used:
```
σ ≈ 17.0 points (totals, based on Boyd's Bets O/U margin data, empirical range 15-21)
```

Each half-point of spread ≈ 1.7% probability change near 50% (0.5/11.5 ≈ 0.043 SDs). The effect is smaller near extreme probabilities.

### Normalization Formula

To convert probability at line L1 to probability at line L2:

```
Φ = standard normal CDF
σ = 11.5 (spreads) or 17.0 (totals)

prob_at_L2 = Φ(Φ⁻¹(prob_at_L1) + (L2 - L1) / σ)
```

### Example

```
Book offers: Home -5.5 at 52% cover probability
Kalshi offers: Home -6.0

L1 = -5.5 (book line), L2 = -6.0 (Kalshi line)
L2 - L1 = -6.0 - (-5.5) = -0.5

Adjustment: -0.5 / 11.5 = -0.043 standard deviations
(Negative because -6.0 is harder to cover than -5.5)

Φ⁻¹(0.52) ≈ 0.05
New z-score = 0.05 + (-0.043) = 0.007
Φ(0.007) ≈ 0.503

Normalized probability at -6.0: 50.3% (lower, as expected)
```

---

## Consensus Line Building

Multiple sportsbooks are averaged to form a "true probability" consensus. Sharp books (Pinnacle, Circa, BetCRIS, Bookmaker, 5Dimes, Heritage) receive 3x weight versus soft books (1x), since sharp lines are empirically closer to closing lines.

```
weighted_avg = Σ(prob_i × weight_i) / Σ(weight_i)

Sharp book weight: 3.0
Soft book weight:  1.0
```

BookCount remains the raw number of contributing books (not weighted).

---

## Arbitrage Execution

Arb opportunities (YES + NO < $1 after fees) are executed with both legs placed concurrently to minimize the window for price movement. Partial fills (one leg succeeds, the other fails) are logged as warnings and result in an unhedged position.

---

## References

- [Kelly Criterion (Wikipedia)](https://en.wikipedia.org/wiki/Kelly_criterion)
- [Boyd's Bets - NBA Key Numbers](https://www.boydsbets.com/nba-key-numbers/)
- [Boyd's Bets - Standard Deviations](https://www.boydsbets.com/ats-margin-standard-deviations-by-point-spread/)
- [Clarke & Kovalchik 2017 - Adjusting Bookmaker's Odds](https://www.sciencepublishinggroup.com/article/10.11648/j.ajss.20170506.12)
