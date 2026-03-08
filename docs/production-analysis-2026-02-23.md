# Production Data Analysis — Mar 8, 2026

> **Data sources:** Fly.io SQLite (`/data/positions.db`, 1784 positions) + Kalshi portfolio API
> (`/portfolio/settlements`, `/portfolio/positions`, `/portfolio/fills`).

---

## 1. Executive Summary

The bot has been live since **Feb 5, 2026** — roughly 31 days of trading. It has placed **1,784
positions** and **1,758 have settled**.

| Metric | Value |
|---|---|
| Positions placed | 1,784 |
| Settled | 1,758 |
| Open | 28 |
| **Realized P&L** | **-$207.33** |
| Capital deployed (settled) | $7,110.72 |
| **ROI on settled bets** | **-2.9%** |
| Win/Loss | 758W / 1000L (43.1%) |
| Total contracts traded | 25,710 |
| Account balance | $391.58 available + $179.32 portfolio = $570.90 |

---

## 2. Market-Type Breakdown

| market_type | W/L | win% | cost | revenue | P&L | ROI |
|---|---|---|---|---|---|---|
| prop_assists | 92/183 | 33.5% | $739.55 | $828.89 | **+$89.34** | **+12.1%** |
| prop_points | 523/335 | 61.0% | $4,658.67 | $4,546.73 | -$111.94 | -2.4% |
| prop_rebounds | 60/65 | 48.0% | $480.48 | $454.75 | -$25.73 | -5.4% |
| prop_threes | 83/416 | 16.6% | $1,228.54 | $1,073.02 | -$155.52 | -12.7% |
| moneyline | 0/1 | 0% | $3.48 | $0.00 | -$3.48 | -100% |

### Key observations

**prop_assists is the only profitable market** at +12.1% ROI (+$89.34).

**prop_points has 61% win rate but negative ROI** — the bot is overpaying for favorites.
High win rate + negative ROI means probability estimates are 2-3% too high.

**prop_threes is the biggest loser** at -12.7% ROI. The bot was buying many cheap longshot
contracts (avg 15.7¢) that rarely hit. The 16.6% win rate tracks the entry price, meaning
zero edge over the market before fees.

---

## 3. Daily P&L Trajectory

Two distinct eras are visible:

**Feb 5-25 (small positions):** Roughly breakeven to slightly positive. Cumulative P&L
peaked at +$37.14 on Feb 25.

**Feb 26 onwards (larger positions):** High variance with big swings. Feb 26 (-$89), Mar 2
(-$242), and Mar 7 (-$115) wiped out all earlier gains. The negative edge got amplified
when position sizes increased.

---

## 4. Changes Made (Mar 8, 2026)

Based on this analysis, three changes were deployed:

### 4.1 Longshot Filter (NEW)
Bets where true probability < 15% or > 85% are now rejected. This eliminates the unprofitable
prop_threes longshot bets and similar extreme-probability positions where fee drag exceeds
any plausible edge.

### 4.2 Prop Staleness Filter (NEW)
Player props now have the same staleness filtering as game odds. Props older than
`MaxOddsAgeSec` (default 30s) are excluded from consensus. Previously, props had **zero
staleness filtering** despite game odds having it.

### 4.3 Tightened Staleness Window
`DefaultMaxOddsAgeSec` reduced from 1800 (30 minutes) to 30 seconds. This ensures the bot
only trades on genuinely fresh data. Given BDL's empirical update frequency (30-60+ min for
game odds, 1-3 hours for props), this will significantly reduce trade volume but improve
quality.

Additionally, unparseable timestamps now default to stale (previously assumed fresh).

---

## 5. Book Sharpness Analysis

Analysis of 709 settled positions with `book_sources` data (per-book vig-removed probabilities):

| Rank | Book | Brier Score | N |
|---|---|---|---|
| 1 | BetMGM | 0.2624 | 503 |
| 2 | FanDuel | 0.2628 | 628 |
| 3 | DraftKings | 0.2630 | 646 |
| 4 | Caesars | 0.2633 | 677 |
| 5 | BetRivers | 0.2652 | 668 |
| 6 | Betway | 0.2659 | 676 |
| 7 | BetParx | 0.2663 | 665 |

**Note:** The Brier score differences (0.004 between best and worst) are within statistical
noise at these sample sizes. ~5,000-10,000 settled positions per book would be needed to
reliably distinguish this gap. No vendor weight changes are justified from this data.

---

## 6. BDL API Freshness

Empirical measurements of BDL `UpdatedAt` timestamps:

- **Game odds (live games):** Vendors updated 30-60 minutes ago
- **Game odds (finished games):** 2-6 hours stale
- **Player props:** Only ~3 vendors return data; timestamps 60-210 minutes old
- **Polymarket:** Near-real-time (~4 seconds) — the only fresh source

BDL claims "real-time" but does not publish a specific update interval. Their core product
is stats (with known 10-minute livescore delay), not odds.
