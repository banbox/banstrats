[English](README.md)

# 时序新特性示例

基于 `engine: time_series` 的小型示例，展示 banbot v0.7.4 至 v0.7.8 新增的能力。仅作教学演示，未经调优。全部注册在 `tsfeatures` 组下，`run_policy[].name` 写作 `tsfeatures:<策略名>`。

| 策略 | 文件 | 展示的特性 |
|---|---|---|
| `tsfeatures:risk_sizing` | [risk_sizing.go](risk_sizing.go) | `EnterReq.RiskRate` 按止损距离定仓，`s.AccountEquity()`、`s.AvailableBalance()`、`s.AccountDrawdown()` 回撤暂停/恢复保护 |
| `tsfeatures:bracket` | [bracket.go](bracket.go) | 止损限价入场（`Stop`+`Limit`）、`OCOGroup`、带 `MoveSLToBreakeven` 的 `TakeProfits` 阶梯止盈、`ActivationPrice`+`CallbackPct` 追踪止损、`StopBars` |
| `tsfeatures:orb` | [orb.go](orb.go) | `s.OpeningRange` 无前视开盘区间、`btime.SessionUSCash`、`btime.IsNearSessionEnd` |
| `tsfeatures:calendar_dca` | [calendar_dca.go](calendar_dca.go) | `btime.IsFirstBarOfPeriodUTC`（与周期无关的每月触发）、`btime.InMonthRangeUTC` 季节窗口 |
| `tsfeatures:deriv_filter` | [derivatives.go](derivatives.go) | 通过 `DataSub` 订阅内置 `funding_rate`、`open_interest`、`premium_kline` 数据源 |
| `tsfeatures:batch_rebalance` | [rebalance.go](rebalance.go) | `BatchInOut` + `OnBatchJobs`、`strat.RebalanceBatchWeights`、中性 `strat.BarPriority` 排序 |

所有策略都把逐品种状态放在 `StratJob.More`（`TradeStrat` 被所有品种共用），并且在任何提前返回之前先计算指标。

## 运行

```bash
go build -o bot .
./bot backtest --config your-base.yml --config examples/tsfeatures/<片段>.yml
```

`your-base.yml` 包含数据库、交易所、品种与时间范围。把下面的片段保存为单独的覆盖文件。回测不会真实下单；`bot trade` 配合 `env: dry_run` 可模拟实盘。

## 配置片段

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
    refine_tf: 5m        # 挂单与阶梯止盈在 5m 子K线上撮合
    max_simul_open: 4    # 挂单同样占用开单名额
    params: {period: 20, callback: 1.5}
```

### orb
```yaml
market_type: linear
run_policy:
  - name: tsfeatures:orb
    run_timeframes: [15m]   # 周期必须不大于 rangeMin；1d 及更粗周期不适用
    refine_tf: 1m
    params: {rangeMin: 30, rr: 2}
```

### calendar_dca
```yaml
market_type: spot            # 只做多，可用于现货
stake_amount: 100
run_policy:
  - name: tsfeatures:calendar_dca
    run_timeframes: [1d]     # 4h / 1h 同样每月精确触发一次
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

## 注意事项

- `deriv_filter` 使用框架内置衍生品数据源，策略本身不注册任何数据源。回测前先下载历史数据，例如 `./bot series down -tables funding_rate,open_interest,premium_kline ...`（可用 `./bot series list` 查看）。部分交易所的持仓量历史只覆盖较近的区间。三类读数（且持仓量非 NULL）到齐之前不会入场。
- `RiskRate` 需要有止损（`StopLoss`/`StopLossVal` 或默认止损），并受杠杆与 `max_stake_amt` 封顶。`AccountEquity`、`AvailableBalance`、`AccountDrawdown` 在预热期回退到配置资金；回撤相对本次运行的净值高水位，按账户统计。
- `bracket`：追踪止损在回测与 `dry_run` 中本地模拟；实盘 `prod` 下 `CallbackPct` 超出交易所上限时自动降级为本地监控。挂单需要 `max_open_orders`/`max_simul_open` 有余量。OCO 只撤销同一 `(账户, 品种, 策略, 组名)` 内尚未成交的入场单。
- `orb` 与时段常量均为 UTC 锚点；周期无法整除区间时 `OpeningRange` 返回 `Applicable=false`，策略保持空闲。
- `calendar_dca` 在每根K线（含预热）上记录上一根K线时间，以保证首根判定正确。
- `batch_rebalance` 由 `RebalanceBatchWeights` 平仓、减仓、开仓；不在目标权重表中的品种会被清仓，低于 `core.MinStakeAmount` 的碎单会被跳过。做空腿需要永续合约市场。
- 这些策略仅用于教学，真实资金使用前请自行核对手续费、滑点与流动性。

## 测试

```bash
GOWORK=off go test ./examples/tsfeatures/...
```
