# Lessons Learned - Sports Betting Bot

## Bugs Found & Fixed

### 1. Hedge Fee Calculation (hedge.go)
**Bug**: Fee was applied to both entry price AND hedge price, but entry fee was already paid.
```go
// WRONG
fees := (pos.EntryPrice + oppositePrice) * kalshiFee

// CORRECT
hedgeFee := oppositePrice * kalshiFee  // Only fee on new trade
```

### 2. Sell Profit Fee Calculation (hedge.go)
**Bug**: Used flat fee amount instead of percentage of sale price.
```go
// WRONG
sellProfit := currentValue - pos.EntryPrice - kalshiFee

// CORRECT
sellFee := currentValue * kalshiFee
sellProfit := currentValue - pos.EntryPrice - sellFee
```

## Edge Cases Handled

1. **No Kalshi vendor** - Returns empty opportunities (safe)
2. **Zero odds** - Returns 0 probability (handled in AmericanToImplied)
3. **Single book consensus** - Works correctly, BookCount=1
4. **Extreme odds (-1000)** - Probability calculation handles correctly
5. **Mixed market availability** - Books with partial data are included appropriately
6. **Games already started** - Filtered by status check in main loop

## Potential Issues to Monitor

### 1. Kalshi Odds Format
- **Risk**: Kalshi natively uses prices ($0.01-$0.99), not American odds
- **Current assumption**: balldontlie.io normalizes to American format
- **Monitor**: If opportunities seem off, verify API returns American odds for Kalshi

### 2. Spread Line Mismatch
- **Risk**: Comparing spread cover probabilities across different lines is invalid
- **Example**: -5.5 @ -110 vs -6 @ +100 are different bets
- **Mitigation**: Current code averages lines, but should probably filter to matching lines

### 3. API Pagination
- **Risk**: GetOdds doesn't handle pagination (Meta.NextCursor)
- **Impact**: May miss games if API paginates
- **TODO**: Add pagination handling for completeness

### 4. Clock Skew in Rate Limiter
- **Risk**: If system clock jumps backward (NTP sync), token bucket behaves unexpectedly
- **Impact**: Low - worst case is slightly over/under rate limiting briefly

### 5. Database Concurrency
- **Risk**: SQLite single-writer limitation
- **Impact**: Low for current single-goroutine design
- **Future**: If adding multiple workers, need connection pooling or different DB

## Testing Coverage

| Component | Unit Tests | Integration Tests |
|-----------|------------|-------------------|
| Odds conversion | ✓ | ✓ |
| Vig removal | ✓ | ✓ |
| Consensus | - | ✓ |
| EV calculation | ✓ | ✓ |
| Kelly criterion | ✓ | ✓ |
| Hedge detection | ✓ | ✓ |
| Position DB | - | ✓ |
| Rate limiter | - | - |
| API client | - | - |

## Recommended Improvements

1. **Add API mocking** for rate limiter and client tests
2. **Add benchmark tests** for hot path (odds processing)
3. **Consider weighted consensus** (sharp books weighted higher)
4. **Add minimum book count threshold** (e.g., require 3+ books)
5. **Log opportunities to DB** for backtesting/analysis
