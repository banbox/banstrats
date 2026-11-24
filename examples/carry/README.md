# 期现基差示例

`CarryBasis` 是一个同时使用现货和永续合约的 `RegisterPairs` 示例：Y 腿是永续（`BTC/USDT:USDT`、`ETH/USDT:USDT`），X 腿是同一交易所的现货（`BTC/USDT`、`ETH/USDT`）。当永续相对现货溢价（永续/现货比值的 z-score 高于入场阈值）时，卖出永续、买入等额现货，在价差回归时平仓，持有期间空头永续腿收取（或支付）资金费。

这不是持有到交割的期现套利，而是基差回归交易：收益来自基差收敛和空头永续腿的资金费，两者都可能为负。

## 实现要点

- 价差模型 `ratio`：对永续/现货比值做滚动布林带 z-score，对冲比固定为 1，即每对持有 `gross/2` 的空永续和 `gross/2` 的多现货。
- 只做空价差：`Band.ShortOnly` 禁止“买永续、卖现货”的反向价差，`LongOnlySpot` 再次拦截任何会让现货腿变成空头的入场（诊断 `pair-spot-short-blocked`）。账本本身也禁止裸做空现货。
- 多市场运行：基础配置需要 `market_type: linear` 和 `extra_markets: [spot]`，池里同时放现货 `BTC/USDT` 与永续 `BTC/USDT:USDT`。资金费只对永续腿结算，现货腿的资金费恒为 0。[carry.yml](carry.yml) 已包含这三项。

## 运行

```sh
go build -o bot .
./bot backtest --config your-runtime.yml --config examples/carry/carry.yml
```

数据库需要覆盖回测窗口内的现货 K 线、永续 K 线（1h 和 1m）以及 `funding_rate` 资金费数据；模板使用 `funding_policy: storage`，从内置资金费表读取结算记录。`your-runtime.yml` 是自己的交易所、数据库与 `time_range` 配置，`market_type`、`extra_markets` 和 `pairs` 会被模板覆盖。

## 参数

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `window` | 168 | 比值 z-score 的滚动窗口（1h 决策下约 7 天） |
| `entry` / `exit` / `stop` | 2 / 0.5 / 5 | 入场、止盈、止损阈值，需满足 `stop > entry > exit >= 0` |
| `maxHold` | 72 | 最大持仓 bar 数，0 关闭 |
| `gross` | 0.5 | 单对总名义占 NAV 的比例，两对合计不得超过 1 |
| `driftBand` | 0.01 | 权重漂移缓冲，小于该值时不调仓 |

## 注意事项

- 品种固定为 BTC 和 ETH，写在 [carry.go](carry.go) 中；换品种需要修改 Go 代码或注册新名字。
- 现货占用 100% 名义的现金，永续占用保证金，账户资金需要能同时覆盖；示例没有处理两腿之间的保证金划转或强平风险。
- 现货和永续的价格序列、手续费与精度不同，权重回测只用决策时刻之后的 1m 收盘价成交，不模拟盘口深度。
- 默认阈值没有经过优化，也没有扣除资金费波动的影响，使用前请在自己的样本外数据上检验。

## 测试

```sh
GOWORK=off go test ./examples/carry/...
```

测试不访问数据库或网络：用统一配置加载器解析 `carry.yml` 并做静态校验，用合成价格验证溢价时做空永续、做多现货，折价时不开仓，也不会出现现货空头。
