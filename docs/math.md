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

### Fat-Tailed Distribution (Student's t)

NBA ATS margins exhibit excess kurtosis (~3.7 vs. normal's 3.0), meaning extreme outcomes occur more often than a normal distribution predicts. We use **Student's t-distribution** to account for these fat tails:

- **Spreads**: df = 7 (`NBASpreadDF`)
- **Totals**: df = 9 (`NBATotalDF`)

### Context-Dependent Standard Deviation

Standard deviation varies with the magnitude of the line:

**Spreads** (`spreadSD`):
```
|spread| ≤ 3:  σ = 10.5 (close games, tighter)
|spread| ≤ 7:  σ = 11.5 (standard)
|spread| > 7:  σ = 12.5 (blowouts, more variable)
```

**Totals** (`totalSD`):
```
totalLine < 215:       σ = 15.5 (low-scoring, tighter)
215 ≤ totalLine ≤ 230: σ = 17.0 (standard)
totalLine > 230:       σ = 18.5 (high-pace, more variable)
```

### Normalization Formula

To convert probability at line L1 to probability at line L2:

```
T     = Student's t CDF (df = 7 for spreads, 9 for totals)
T⁻¹   = Student's t inverse CDF
σ     = context-dependent SD (see above)

For spreads:
  bookT = T⁻¹(prob_at_L1, df)
  targetT = bookT + (L2 - L1) / σ
  prob_at_L2 = T(targetT, df)

For totals (sign flipped — higher line is harder to go over):
  bookT = T⁻¹(prob_at_L1, df)
  targetT = bookT - (L2 - L1) / σ
  prob_at_L2 = T(targetT, df)
```

### Example

```
Book offers: Home -5.5 at 52% cover probability
Kalshi offers: Home -6.0
σ = 11.5 (|spread| ≤ 7)

T⁻¹(0.52, df=7) ≈ 0.0502
lineDiff = -6.0 - (-5.5) = -0.5
targetT = 0.0502 + (-0.5 / 11.5) = 0.0502 - 0.0435 = 0.0067
T(0.0067, df=7) ≈ 0.503

Normalized probability at -6.0: 50.3% (lower, as expected)
```

---

## Consensus Line Building

Multiple sportsbooks are combined using a **log-linear opinion pool** (averaging in logit space) to form a "true probability" consensus. Each vendor has a market-type-aware weight based on empirical closing-line accuracy (Pikkit 2025 data, Data Golf studies).

**Game markets** (moneyline/spread/total): DraftKings 1.5x, Bet365 1.3x, BetMGM 0.7x, others 1.0x.
**Player props**: FanDuel 1.5x, DraftKings 1.2x, BetMGM 0.7x, others 1.0x.

### Log-Linear Opinion Pool

```
logit(p) = log(p / (1 - p))
sigmoid(x) = 1 / (1 + exp(-x))

consensus = sigmoid( Σ(logit(prob_i) × weight_i) / Σ(weight_i) )
```

Averaging in logit space amplifies confident signals — a book posting 90% has more influence than one posting 55%, compared to arithmetic averaging.

### Winsorized Outlier Capping

When 3+ books contribute, logits are **winsorized** at ±2σ using a robust estimator:

```
1. Find median of logits (robust center)
2. Calculate MAD = mean(|logit_i - median|)
3. Robust σ = 1.2533 × MAD
4. Clamp each logit to [median - 2σ, median + 2σ]
```

This prevents a single outlier book from distorting the consensus.

### Bayesian Shrinkage

When fewer than 6 books contribute, the consensus is **shrunk toward Kalshi's implied probability** using power-law decay:

```
weight = (bookCount / 6) ^ 1.5
shrunk = weight × consensus + (1 - weight) × kalshiPrice
```

| Books | Weight on Consensus |
|-------|-------------------|
| 6+ | 100% (no shrinkage) |
| 5 | 76.0% |
| 4 | 54.4% |
| 3 | 35.4% |
| 2 | 19.2% |
| 1 | 6.8% |

### Scaled EV Threshold

When fewer books contribute, the EV threshold is raised by +1% per missing book below 6:

```
threshold = baseEV + 0.01 × max(0, 6 - bookCount)
```

At 4 books: 3% + 2% = 5% required EV. This prevents marginal trades based on unreliable consensus.

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
