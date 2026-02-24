# Production Data Analysis — Feb 23, 2026

> **Data sources:** Fly.io SQLite snapshot (`positions.db` pulled Feb 23, 2026) + live Kalshi
> portfolio API (`/portfolio/fills`, `/portfolio/settlements`, `/portfolio/positions`).
> Analysis run: `go run ./cmd/analyze_results/ --db ./logs/positions_prod.db`

---

## 1. Executive Summary

The bot has been live since **Feb 5, 2026** — roughly 19 days of trading. It has placed **739
positions** across four NBA player-prop market types, with **705 now settled** and **34 still open**
(tonight's Feb 23 games).

| Metric | Value |
|---|---|
| Positions placed | 739 |
| Settled | 705 |
| Open (tonight) | 34 |
| Realized P&L | **+$31.93** |
| Capital deployed (settled 705) | $931.29 |
| Capital deployed (all 739) | $990.81 |
| **ROI on settled bets** | **+3.4%** |
| Average cost per settled bet | $1.32 |
| Raw win rate | 279 / 705 = 39.6% |

The 39.6% raw win rate is not alarming — the bot places many low-probability props (3–20¢) whose
expected win rate is similarly low. The meaningful number is **+3.4% ROI**, which means we are
extracting real edge over the market on net.

---

## 2. Market-Type Breakdown (settled positions only)

| market_type | bets | wins | win% | avg_entry | P&L |
|---|---|---|---|---|---|
| prop_points | 301 | 166 | **55%** | 55¢ | **+$19.44** |
| prop_assists | 130 | 53 | 41% | 40¢ | +$5.90 |
| prop_rebounds | 41 | 22 | **54%** | 57¢ | +$5.88 |
| prop_threes | 233 | 38 | 16% | 16¢ | +$0.71 |
| **TOTAL** | **705** | **279** | **40%** | | **+$31.93** |

Win rate verification: 53/130=40.8%≈41% ✓ · 166/301=55.1%≈55% ✓ · 22/41=53.7%≈54% ✓ ·
38/233=16.3%≈16% ✓ · P&L: 5.90+19.44+5.88+0.71=31.93 ✓

### Key observations

**prop_points is the workhorse.** 301 bets (43% of volume), 55% win rate, +$19.44 — accounting
for 61% of total profit. The model has genuine edge here.

**prop_rebounds is the efficiency leader.** Only 41 bets but +$5.88 profit and 54% win rate at
a higher avg price (57¢). Small sample but positive signal.

**prop_assists is a solid contributor.** 130 bets, 41% win rate at 40¢ avg — profitable.

**prop_threes is essentially a wash.** 233 bets, 16% win rate at 16¢ avg — the win rate
*exactly tracks the entry price*, which means the market is efficiently pricing these. After
Kalshi's fee (~6% drag at 15¢), this market type is near-zero EV.

---

## 3. Capital Deployment (all 739 positions)

| type | side | bets | total_capital | avg/bet | avg_price |
|---|---|---|---|---|---|
| prop_points | NO | 168 | **$433.23** | $2.58 | 65.7¢ |
| prop_points | YES | 149 | $197.94 | $1.33 | 43.7¢ |
| prop_assists | YES | 109 | $125.01 | $1.15 | 38.4¢ |
| prop_threes | YES | 214 | $114.67 | $0.54 | 15.4¢ |
| prop_rebounds | YES | 39 | $72.56 | $1.86 | 54.2¢ |
| prop_assists | NO | 25 | $19.62 | $0.78 | 43.4¢ |
| prop_threes | NO | 28 | $18.48 | $0.66 | 17.9¢ |
| prop_rebounds | NO | 7 | $9.30 | $1.33 | 42.9¢ |
| **TOTAL** | | **739** | **$990.81** | $1.34 | 39.3¢ |

Capital verification: 433.23+197.94+125.01+114.67+72.56+19.62+18.48+9.30 = $990.81 ✓

**prop_points NO dominates at $433.23 (44% of all capital).** These are high-confidence "player
scores UNDER extreme line" bets (e.g., Durant under 20 at NO=81¢, meaning we paid 19¢ per NO
contract). The 65.7¢ avg NO price means the market gives only 34.3¢ (34%) chance the player
does NOT hit the threshold — we're on the side that says they won't.

**prop_threes YES at $0.54/bet** is cheap per bet but adds up to $114.67 total (11.6% of all
capital). Combined with NO, prop_threes consumed $133.15 total — and produced only +$0.71 P&L.

---

## 4. Calibration Analysis

The correct benchmark for each position is its **entry price** — the price paid equals the
market's implied win probability. A perfectly calibrated market would show actual win rate =
avg entry price in each bucket.

| bucket | N (settled) | actual% | market-implied% | edge |
|---|---|---|---|---|
| <40¢ | 435 | 16% | **16.2%** (avg entry) | ~0pp — break-even with market |
| 40-49¢ | 11 | 36% | 44.5% | −9pp under (N=11, high variance) |
| 50-59¢ | 22 | 73% | **55.7%** | **+17pp — outperforming** |
| 60-69¢ | 41 | 68% | **65.0%** | +3pp — modest outperformance |
| 70+¢ | 196 | 82% | **83.7%** | −2pp — slight underperformance |

Market-implied% derived from avg entry price per bucket across all positions (95% settled),
not hardcoded bucket midpoints.

### The <40¢ bucket: market is efficiently priced

435 bets winning at 16% against a 16.2¢ average entry. The market is correctly pricing these
longshot props — we have no edge, and are just at break-even vs market before fees. After
Kalshi's fee formula (`0.07 × p × (1−p)`), at 15¢ avg: fee = `0.07 × 0.15 × 0.85 = 0.89¢`
per contract = **6% fee drag** on a 15¢ bet. This means the <40¢ bucket is effectively a small
systematic loser, neutralised largely by prop_threes' +$0.71 appearing positive due to rounding.

### The 50-69¢ range: where real edge lives

- **50-59¢: +17pp** (actual 73% vs expected 55.7%) — this is the bot's strongest signal
- **60-69¢: +3pp** (actual 68% vs expected 65.0%) — modest but consistent

Note: the 50-59¢ bucket only has N=22 settled positions, so the +17pp could partly be variance
(~1.7 standard deviations, not yet statistically conclusive at 95% confidence). But it aligns
directionally with the 60-69¢ result.

### The 70+¢ bucket: slight underperformance

196 positions, actual 82% vs market-implied 83.7% → −2pp. These are high-confidence bets
(player almost certainly hits or misses extreme line). The bot is NOT outperforming the market
here; the market is efficiently pricing these high-probability outcomes. The fee drag here is
low (`0.07 × 0.84 × 0.16 = 0.94¢/contract = 1.1%`), so losses are small.

---

## 5. Weekly Volume Trend

| week | bets | capital | avg_price |
|---|---|---|---|
| Week 1: Feb 5–9 | 269 | **$360.97** | 35.4¢ |
| Week 2: Feb 10–16 | 106 | **$177.45** | 36.2¢ |
| Week 3: Feb 17–23 | 364 | **$452.39** | 43.0¢ |

Capital verification: 360.97+177.45+452.39 = $990.81 ✓

**Week 2 drop is the NBA All-Star break** (Feb 14–17, no regular-season games) — expected and
healthy. The bot found few opportunities when there were fewer games.

**Week 3 ramp-up is notable:** 364 bets at a higher avg price of 43¢ vs 35¢ in week 1. This
could mean better-quality opportunities as the season matures, or mild over-activation.
Week 3 represents $452.39 of the $990.81 total capital (46%) in just 7 days.

---

## 6. Most-Bet Players

| player | type | bets | avg_price | contracts | capital |
|---|---|---|---|---|---|
| Anthony Edwards | prop_points | 13 | 17.8¢ | 71 | $10.67 |
| Jaylen Brown | prop_points | 13 | 18.9¢ | 58 | $9.51 |
| Victor Wembanyama | prop_threes | 13 | 10.4¢ | 86 | $6.44 |
| De'Aaron Fox | prop_threes | 12 | 9.8¢ | 88 | $5.19 |
| Jalen Brunson | prop_points | 12 | 19.7¢ | 40 | $7.52 |
| Nikola Jokic | prop_threes | 12 | 12.8¢ | 30 | $3.07 |
| Draymond Green | prop_points | 9 | 77.2¢ | 42 | $35.62 |
| DeMar DeRozan | prop_points | 8 | 81.3¢ | 22 | $18.23 |
| Russell Westbrook | prop_points | 8 | 78.0¢ | 36 | $30.31 |

**Edwards and Brown at 18–19¢ are longshot YES bets** (player scores 35+ or similar). 13 bets
each at ~$0.80/bet = ~$10 each in capital. At 18¢ entry, market says 18% win probability.
We'd need consistently >18% win rate to profit before fees (which cost ~5.8% at 18¢). Needs
monitoring.

**Wembanyama and Fox prop_threes at 9–10¢** cost $5–$6.44 in capital across 12–13 bets.
At 10¢ avg, fee drag is: `0.07 × 0.10 × 0.90 = 0.63¢/contract = 6.3%`. These are effectively
fee-subsidised lottery tickets with no expected edge.

**Draymond Green, DeRozan, Westbrook at 77–81¢** are high-confidence YES or NO bets — these
are in the 70+¢ bucket which runs at −2pp vs market on average, but the fee drag is only ~1%.
The $35.62 on Draymond alone is the single largest player-level capital concentration.

---

## 7. Open Positions (Tonight — Feb 23)

34 positions still live, all from tonight's games (SAS-DET, UTA-HOU, SAC-MEM). Notable by cost:

| ticker | market_type | side | contr | price | cost |
|---|---|---|---|---|---|
| SAS D.Harper under 20pts | prop_points | NO | 10 | 95¢ | **$9.50** |
| SAC Westbrook under 20pts | prop_points | NO | 10 | 91¢ | **$9.10** |
| HOU Sengun over 10pts | prop_points | YES | 9 | 89¢ | **$8.01** |
| HOU Amen Thompson over 10pts | prop_points | YES | 7 | 83¢ | $5.81 |
| SAC Keegan Murray under 25pts | prop_points | NO | 4 | 86¢ | $3.44 |
| HOU Jabari Smith over 15pts | prop_points | YES | 6 | 42¢ | $2.52 |
| HOU Sengun over 12 rebounds | prop_rebounds | YES | 27 | 6¢ | $1.62 |
| SAS De'Aaron Fox over 5 threes | prop_threes | YES | 16 | 4¢ | $0.64 |
| SAS Wembanyama over 5 threes | prop_threes | YES | 10 | 5¢ | $0.50 |

Costs verified: 10×0.95=$9.50 ✓ · 10×0.91=$9.10 ✓ · 9×0.89=$8.01 ✓ · 7×0.83=$5.81 ✓ ·
27×0.06=$1.62 ✓

The Harper and Westbrook NO bets ($9.50, $9.10) are the largest single-game exposures in the
dataset. Both are extreme-line NO bets at 95¢ and 91¢ — very high-confidence positions.

The 27-contract Sengun rebounds bet ($1.62 total) is low-dollar but Kelly over-sizing for a 6¢
longshot. This is the pattern that needs fixing.

---

## 8. Math Tuning Recommendations

### 8.1 Filter prop_threes bets below 20¢ (HIGH IMPACT)

**Finding:** 163 of the 214 prop_threes YES bets are below 20¢ at avg 11.2¢, representing $82.06
of $114.67 total prop_threes YES capital. At 11.2¢ avg, fee drag = `0.07 × 0.112 × 0.888 ×
100 = 0.70¢/contract = 6.2%`. The market prices these correctly (16% win on 16¢ entry) and
after fees they are net-negative EV.

**Fix:** In `internal/analysis/ev.go`, skip prop_threes bets below 20¢:

```go
if marketType == "prop_threes" && kalshiPrice < 0.20 {
    return  // fee drag eliminates any edge below 20¢
}
```

Or raise `ScaledEVThreshold` by +3pp for sub-20¢ bets. Either way, this eliminates ~163 bets
and frees ~$82 of capital per 19-day cycle for higher-edge opportunities.

### 8.2 Cap contracts at 3 for any bet below 20¢ (MEDIUM IMPACT)

**Finding:** Kelly is allocating 10–27 contracts to 4–10¢ bets (Fox 16@4¢, Wembanyama 10@5¢,
Sengun 27@6¢). At these prices, the edge over market is effectively zero, so Kelly's formula
is amplifying noise rather than signal.

**Fix:** In `CalculateKellyContracts`, hard-cap at 3 contracts when price < 0.20:

```go
if effectivePrice < 0.20 && contracts > 3 {
    contracts = 3
}
```

### 8.3 Hold prop_points, prop_rebounds, prop_assists settings (NO CHANGE)

**Finding:** These are all profitable. prop_points NO (65.7¢ avg) and prop_rebounds YES (54.2¢
avg) are in the 50-70¢ range where the bot shows +3 to +17pp outperformance vs market.
The 70+¢ bucket runs at −2pp vs market but with only 1.1% fee drag — not worth disrupting
settings that are working.

### 8.4 Hold Kelly fraction at 0.25 (NO CHANGE)

**Finding:** +3.4% ROI on $931 over 705 settled bets. The per-bet sizing is appropriate.

### 8.5 Hold shrinkage exponent at 1.5 (NO CHANGE)

**Finding:** The power-law shrinkage produces well-calibrated probabilities at 50–70¢. The
underperformance at <40¢ is market efficiency (not a model calibration problem), and the fix
is filtering those bets out rather than adjusting shrinkage.

---

## 9. Anomalies

1. **No duplicate tickers.** The in-flight lock and `HasPositionOnTicker` guard are working.
   Zero cases of the same ticker+side being bet twice.

2. **Feb 13 gap.** Only 5 bets on Feb 13, including a single prop_assists YES at 93¢ (10
   contracts = $9.30). May indicate partial-day run. Not a recurring issue.

3. **Week 3 acceleration.** 364 bets / $452 in 7 days vs 269 bets / $361 in the first 5 days.
   The bet rate is higher in week 3. Watch whether ROI holds at this pace.

4. **Largest single-player exposure: Draymond Green.** $35.62 across 9 prop_points bets at
   77.2¢ avg. These are in the 70+¢ bucket (−2pp vs market), so exposure is high-conviction
   but not generating clear alpha vs the market price.

---

## 10. Summary

| Finding | Correct? | Action |
|---|---|---|
| Overall P&L +$31.93, ROI +3.4% | ✓ | Monitor |
| prop_points driving 61% of profit | ✓ | No change |
| prop_threes breaks even after fees | ✓ | Filter <20¢ bets |
| <40¢ bucket: break-even vs market | ✓ (not underperforming) | See above |
| 50-59¢ bucket: +17pp vs market | ✓ (N=22, directional) | No change |
| 70+¢ bucket: −2pp vs market (not outperforming) | ✓ | No change needed |
| Kelly over-sizing at sub-5¢ longshots | ✓ | Cap at 3 contracts |
