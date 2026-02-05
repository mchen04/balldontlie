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

### Multiplicative Method (Default)

Proportionally scales probabilities to sum to 100%:

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

### Power Method (For Player Props)

Accounts for favorite-longshot bias (longshots are systematically overbet).

Finds exponent `k` such that `p1^k + p2^k = 1`:

```
trueProb_A = implied_A^k
trueProb_B = implied_B^k
```

This deflates longshot probabilities more than favorites, which better reflects true odds in props markets.

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

Kalshi charges ~1.2% fee on the contract price:

```
adjustedEV = rawEV - (price × 0.012)
```

### Example Calculation

```
True probability: 55%
Kalshi price: 50¢

rawEV = 0.55 - 0.50 = 0.05 (5%)
fee = 0.50 × 0.012 = 0.006 (0.6%)
adjustedEV = 0.05 - 0.006 = 0.044 (4.4%)
```

This bet has +4.4% EV after fees.

---

## Kelly Criterion

Kelly determines the optimal bet size to maximize long-term growth.

### Formula

```
f* = (p × b - q) / b
```

Where:
- `f*` = fraction of bankroll to bet
- `p` = probability of winning
- `q` = probability of losing (1 - p)
- `b` = odds received (profit / stake)

### For Kalshi

```
b = (1 - price) / price

f* = (trueProb × b - (1 - trueProb)) / b
```

### Fractional Kelly

Full Kelly is aggressive and leads to high variance. We use **quarter Kelly** (25%):

```
f = f* × 0.25
```

This reduces variance while still capturing most of the growth.

### Example

```
True probability: 60%
Kalshi price: 50¢

b = (1 - 0.50) / 0.50 = 1.0
f* = (0.60 × 1.0 - 0.40) / 1.0 = 0.20 (20%)
f = 0.20 × 0.25 = 0.05 (5%)

With $100 bankroll: bet $5
```

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
N(0, σ) where σ ≈ 11.5 points
```

Each half-point of spread ≈ 1.7% probability change.

### Normalization Formula

To convert probability at line L1 to probability at line L2:

```
Φ = standard normal CDF
σ = 11.5 (NBA standard deviation)

prob_at_L2 = Φ(Φ⁻¹(prob_at_L1) + (L2 - L1) / σ)
```

### Example

```
Book offers: Team -5.5 at 52%
Kalshi offers: Team -6.0

Adjustment: (6.0 - 5.5) / 11.5 = 0.043 standard deviations

Φ⁻¹(0.52) ≈ 0.05
New z-score = 0.05 - 0.043 = 0.007
Φ(0.007) ≈ 0.503

Normalized probability at -6.0: 50.3%
```

---

## References

- [Kelly Criterion (Wikipedia)](https://en.wikipedia.org/wiki/Kelly_criterion)
- [Boyd's Bets - NBA Key Numbers](https://www.boydsbets.com/nba-key-numbers/)
- [Boyd's Bets - Standard Deviations](https://www.boydsbets.com/ats-margin-standard-deviations-by-point-spread/)
