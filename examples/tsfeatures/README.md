[中文](README_cn.md)

# Time-series feature examples

Small `engine: time_series` strategies showcasing banbot features added between v0.7.4 and v0.7.8. They are teaching examples, not tuned systems. All are registered in the group `tsfeatures`, so `run_policy[].name` is `tsfeatures:<name>`.

| Strategy | File | Feature shown |
|---|---|---|
| `tsfeatures:risk_sizing` | [risk_sizing.go](risk_sizing.go) | `EnterReq.RiskRate` sizing from the stop distance, `s.AccountEquity()`, `s.AvailableBalance()`, `s.AccountDrawdown()` pause/resume guard |
| `tsfeatures:bracket` | [bracket.go](bracket.go) | Stop-limit entries (`Stop`+`Limit`), `OCOGroup`, `TakeProfits` ladder with `MoveSLToBreakeven`, `ActivationPrice`+`CallbackPct` trailing stop, `StopBars` |
| `tsfeatures:orb` | [orb.go](orb.go) | `s.OpeningRange` no-lookahead range, `btime.SessionUSCash`, `btime.IsNearSessionEnd` |
| `tsfeatures:calendar_dca` | [calendar_dca.go](calendar_dca.go) | `btime.IsFirstBarOfPeriodUTC` (timeframe-independent monthly trigger), `btime.InMonthRangeUTC` season window |
| `tsfeatures:deriv_filter` | [derivatives.go](derivatives.go) | Built-in `funding_rate`, `open_interest`, `premium_kline` sources subscribed through `DataSub` |
| `tsfeatures:batch_rebalance` | [rebalance.go](rebalance.go) | `BatchInOut` + `OnBatchJobs`, `strat.RebalanceBatchWeights`, neutral `strat.BarPriority` ordering |

Every strategy keeps per-symbol state on `StratJob.More` (the `TradeStrat` is shared by all symbols) and computes indicators on every bar before any early return.

## Run

```bash
go build -o bot .
./bot backtest --config your-base.yml --config examples/tsfeatures/<snippet>.yml
```

`your-base.yml` holds the database, exchange, pairs and time range. Put a snippet below into its own overlay file. Backtests never place real orders; `bot trade` with `env: dry_run` simulates live trading.

## Configuration snippets

### risk_sizing
```yaml
market_type: linear
wallet_amounts: {USDT: 10000}
run_policy:
  - name: tsfeatures:risk_sizing
    run_timeframes: [1h]
    params: {fast: 10, slow: 30, riskRate: 0.01, ddPause: 0.10, ddResume: 0.05}
```

### bracket
```yaml
market_type: linear
run_policy:
  - name: tsfeatures:bracket
    run_timeframes: [1h]
    refine_tf: 5m        # pending stop-limit and ladder orders are matched on 5m bars
    max_simul_open: 4    # pending entries count as open orders
    params: {period: 20, callback: 1.5}
```

### orb
```yaml
market_type: linear
run_policy:
  - name: tsfeatures:orb
    run_timeframes: [15m]   # must be <= rangeMin; 1d and coarser are not applicable
    refine_tf: 1m
    params: {rangeMin: 30, rr: 2}
```

### calendar_dca
```yaml
market_type: spot            # long-only, works on spot
stake_amount: 100
run_policy:
  - name: tsfeatures:calendar_dca
    run_timeframes: [1d]     # 4h / 1h also fire exactly once per month
    params: {startMonth: 11, endMonth: 4, maxUnits: 6}
```

### deriv_filter
```yaml
market_type: linear
run_policy:
  - name: tsfeatures:deriv_filter
    run_timeframes: [1h]
    params: {period: 20, maxFunding: 0.0003, maxPremium: 0.002, oiLookback: 24}
```

### batch_rebalance
```yaml
market_type: linear
run_policy:
  - name: tsfeatures:batch_rebalance
    run_timeframes: [1d]
    params: {lookback: 30, basket: 2, gross: 1.0, drift: 0.03, stopRate: 0.15}
pairs: [BTC/USDT:USDT, ETH/USDT:USDT, SOL/USDT:USDT, BNB/USDT:USDT, XRP/USDT:USDT, DOGE/USDT:USDT]
```

## Caveats

- `deriv_filter` uses the built-in derivatives sources; nothing is registered by the strategy. Download the history before a backtest, for example `./bot series down -tables funding_rate,open_interest,premium_kline ...` (see `./bot series list`). Open interest history is only available for a limited recent range on some exchanges. Entries stay blocked until all three readings (and a non-NULL open interest) have arrived.
- `RiskRate` needs a stop (`StopLoss`/`StopLossVal` or a default stop) and is capped by leverage and `max_stake_amt`. `AccountEquity`, `AvailableBalance` and `AccountDrawdown` fall back to configured capital during warmup; the drawdown is measured from the high-water mark of the run, per account.
- `bracket`: the trailing stop is simulated locally in backtest and `dry_run`; in `prod` a `CallbackPct` beyond the exchange limit falls back to local monitoring. Pending entries need `max_open_orders`/`max_simul_open` headroom. OCO only cancels unfilled entries of the same `(account, symbol, strategy, group)`.
- `orb` and the session constants are UTC anchors; `OpeningRange` returns `Applicable=false` when the timeframe does not divide the range, so the strategy simply stays idle.
- `calendar_dca` needs warmup-safe bar tracking: the previous bar time is stored on every bar, including warmup bars.
- `batch_rebalance` lets `RebalanceBatchWeights` close, reduce and open orders; symbols missing from the target map are flattened. Orders below `core.MinStakeAmount` are skipped. Needs a perpetual market for the short side.
- These strategies are for education only; check fees, slippage and liquidity before using any of them with real money.

## Tests

```bash
GOWORK=off go test ./examples/tsfeatures/...
```
