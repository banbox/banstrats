# 统计套利示例

本目录用 banbot 因子引擎的统计套利注册接口实现五个示例：固定配对、滚动重选配对、多腿 Johansen 篮子，以及截面 PCA / 基准残差 s-score。它们都使用 `engine: factor`，由 `bot backtest` 在权重回测（`execution.mode: weights`）下运行，也可以用 `bot research` 查看输入面板。

本目录使用 Go 1.26.0+ 和已发布的 Banbot v0.7.8。示例中的阈值与窗口只是演示用的合理起点，没有经过参数优化，也不代表任何收益；使用前请先在自己的数据上做样本外检验。

| 策略名称 | 注册接口 | 做什么 | 覆盖文件 |
| --- | --- | --- | --- |
| `SAPairsEthBtc` | `runner.RegisterPairs` | 固定两对：ETH 对 BTC、SOL 对 ETH，滚动对数 OLS 残差 z-score 回归 | [pairs.yml](pairs.yml) |
| `SARollingPairs` | `runner.RegisterRollingPairs` | 从 YAML 的 `pairs` 候选池中，按形成期 + 交易期无前视地周期重选配对 | [rolling-pairs.yml](rolling-pairs.yml) |
| `SABasketJohansen` | `runner.RegisterBasketsParams` | 两个三腿篮子，滚动 Johansen 协整向量（或多元 OLS）构造价差 | [basket.yml](basket.yml) |
| `SAResidualPCA` | `runner.RegisterResidualStatArbParams` | 截面 PCA 因子残差的 OU s-score，美元中性且因子 Beta 中性，配滚动流动性宇宙 | [residual-pca.yml](residual-pca.yml) |
| `SAResidualBench` | `runner.RegisterResidualStatArbParams` | 同上，但系统因子换成 BTC、ETH 两个基准收益 | [residual-benchmark.yml](residual-benchmark.yml) |

期现基差套利见 [../carry](../carry/README.md)。

## 快速开始

在项目根目录构建程序：

```sh
go build -o bot .
```

准备自己的基础配置 `your-runtime.yml`（数据库、交易所、`time_range` 等）。本目录的 YAML 都是策略覆盖文件，需要与基础配置一起使用。每个覆盖文件里的根级 `pairs` 是该示例需要的交易对池，会替换基础配置里的池；换成自己的标的时请同步修改。

```sh
# 固定配对
./bot backtest --config your-runtime.yml --config examples/statarb/pairs.yml
# 滚动重选配对
./bot backtest --config your-runtime.yml --config examples/statarb/rolling-pairs.yml
# Johansen 篮子
./bot backtest --config your-runtime.yml --config examples/statarb/basket.yml
# PCA / 基准残差 s-score
./bot backtest --config your-runtime.yml --config examples/statarb/residual-pca.yml
./bot backtest --config your-runtime.yml --config examples/statarb/residual-benchmark.yml
```

研究命令使用同样的配置：

```sh
./bot research --config your-runtime.yml --config examples/statarb/pairs.yml
```

对配对和篮子，`research` 只输出对数价格面板与标签诊断；绩效以 `backtest` 为准。`backtest` 的结果里 `Result.Pairs` 给出每对（或每个篮子）的入出场记录、胜率、持仓时间、最近对冲比和换手，`PairsGrossPnL`、`PairsNAVReturn`、`PairsResidual` 用来对账（残差约等于成本和执行近似）。

## 数据与配置要求

- 决策周期默认 `1h`，需要池内全部标的的 1h K 线；成交价流为 `kline 1m close`，需要 1m K 线。只用 1h 收盘价作价格流时，下一个报价晚于默认 `expiry_ms`，目标全部过期、零成交，所以模板用 1m 价格流并把 `expiry_ms` 设为 120000。
- 窗口都按**决策 K 线根数**计数。改 `run_timeframes` 时要同步调整 `window`、`formation`、`reselect` 等参数。
- 模板使用 `funding_policy: explicit-zero`，即资金费按零处理。永续合约的统计套利对资金费敏感，实测前请改成真实资金费来源。
- `portfolio.mode` 保持 `full`。平仓的对会从目标中删除，`patch` 模式不会平掉未列出的仓位。
- 参数写在 `run_policy[].params`，拼错的参数名会报错（示例在构建时调用了 `Params.Unused()`），而不是悄悄使用默认值。布尔参数写 `0` 或 `1`。改任何参数都会产生新的策略身份。
- 固定配对和篮子的品种写在 Go 代码里（[pairs.go](pairs.go)、[basket.go](basket.go)），`Params` 没有字符串读取方法，所以不能从 YAML 修改。要换品种，改 Go 代码重新编译，或注册另一个名字。
- 某个标的在回测窗口内没有数据（退市、未上市）时，含该腿的配对整次跳过，其余配对照常运行；运行中缺数据的腿会让所在配对平仓并暂停。

## SAPairsEthBtc：固定配对

[pairs.go](pairs.go) 中的 `pairsSpec` 声明两对 `{Y: ETH, X: BTC}` 与 `{Y: SOL, X: ETH}`。每个决策 bar：用最近 `window` 个有效观测拟合 `ln(Y) = α + β·ln(X)`，残差除以残差标准差得到 z；`z <= -entry` 做多价差（买 Y 卖 X），`z >= entry` 做空价差；`|z| <= exit` 止盈；`|z| >= stop` 止损并要求 z 回到 `[-exit, exit]` 内才可再入场。

用到的选项：

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `window` | 120 | 滚动回归窗口（决策 bar 数） |
| `entry` / `exit` / `stop` | 2 / 0.5 / 4 | z-score 入场、止盈、止损阈值，需满足 `stop > entry > exit >= 0`（`stop: 0` 关闭止损） |
| `maxHold` | 0 | 最大持仓 bar 数，0 关闭 |
| `gross` | 0.5 | 单对总名义占 NAV 的比例；两对合计不得超过 1 |
| `minBeta` | 0.3 | 入场时对冲比（对数模型中是弹性）必须大于该值。默认还会拒绝 `β <= 0`，避免两腿同向 |
| `inverseVol` | 1 | 1：把总敞口按 `1/残差标准差` 在持仓对之间分配（单对上限默认 `min(总敞口, 2×gross)`）；0：每对固定 `gross` |
| `driftBand` | 0.02 | 持仓期权重自然漂移的 L1 距离小于该值时不调仓，减少无谓换手和费用 |
| `dynamicExitZ` | 0 | 0：持仓期退出 z 用入场时冻结的对冲比计算，与实际持仓盈亏一致；1：改用每轮重拟合的斜率 |
| `minHalfLife` / `maxHalfLife` | 0 / 48 | 在线 OU 半衰期（bar 数）闸门，窗口内不回归的价差不开新仓 |
| `minCorr` | 0.6 | 窗口内 1-bar 对数收益相关系数下限 |
| `maxHurst` | 0.5 | 价差窗口 Hurst 上限，0 关闭；0.5 只在均值回归状态入场 |
| `maxLegVol` | 0.03 | 入场过滤钩子：任一腿最近 24 根收益标准差超过该值时不开新仓，0 关闭 |
| `kalman` | 0 | 1：改用 Kalman 随机游走估计斜率与截距（`kalmanDelta`、`kalmanVe`，0 为框架默认），可配 `dynamicHedge: 1` 让持仓对冲比随估计更新 |

`maxLegVol` 展示了 `FeatureExprs` + `EntryFilter`：表达式 `ts.std(ts.return(kline.close, 1), 24, 0)` 给每条腿生成特征列，`EntryFilter` 只在开仓时读取当期特征（无前视），平仓和止损永远不会被它拦截。

## SARollingPairs：滚动点时配对

[rolling.go](rolling.go) 不写任何品种。每隔 `reselect` 根 bar，只用已收盘数据在候选池里重选：在最近 `formation` 根上，要求覆盖率、对数收益相关、Engle-Granger 残差 ADF 统计量（`<= maxAdf`）和 OU 半衰期都达标，按 ADF 统计量从小到大贪心选出至多 `pairCount` 对，同一品种最多出现 `maxPerSymbol` 次。入选后按与固定配对相同的状态机在线交易。排序并列时按品种符号和 SID 确定性打破，同输入重复运行结果一致。

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `formation` / `reselect` | 240 / 120 | 形成期窗口与交易期长度（bar 数），`formation >= 20` |
| `pairCount` / `maxPerSymbol` | 4 / 2 | 每期最多入选对数、单品种最多出现次数；单对 `gross` 缺省为 `1/pairCount` |
| `distance` | 0 | 1：改用 Gatev 最小距离法选对；0：协整检验 |
| `maxAdf` / `adfLags` | -2.86 / 1 | ADF 上限（须为负）与滞后阶数 |
| `minCorr` | 0.6 | 形成期对数收益相关系数下限 |
| `minHalfLife` / `maxHalfLife` | 2 / 0 | 形成期半衰期范围；上限 0 取 `formation` |
| `window` | 120 | 在线价差窗口；新入选的对用此前可见的历史预热，当轮即可交易 |
| `entry` / `exit` / `stop` / `maxHold` | 2 / 0.5 / 4 / 60 | 在线状态机阈值 |
| `driftBand` / `inverseVol` / `minBeta` | 0.02 / 1 / 0.3 | 同固定配对 |
| `onlineHalfLife` | 1 | 1：开仓前还要求在线窗口的半衰期落在 `[minHalfLife, maxHalfLife]` |
| `handoffImmediate` | 0 | 0：换期时未连任但仍持仓的对继续持有到正常退出，只退不进并占用名额；1：换期当根立即平仓 |

首次满 `formation` 根之前没有目标（诊断 `pairs-not-ready`）。0 对入选时诊断为 `rolling-pairs-empty-selection`。

## SABasketJohansen：多腿协整篮子

[basket.go](basket.go) 声明两个三腿篮子（`majors`：BTC、ETH、BNB；`alt-l1`：SOL、AVAX、NEAR）。每个决策 bar 对窗口内的对数价格做 Johansen 检验，取首个协整向量并固定符号使首个分量为正，用它构造篮子价差，再用与配对相同的 z-score 状态机交易。持仓期默认冻结入场时的单位权重和价差向量，使退出 z 与实际持仓盈亏一致。

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `window` | 240 | 估计窗口；Johansen 需要 `window >= 腿数 × (differenceLags + 1) + 3` |
| `differenceLags` | 1 | VECM 差分滞后阶数 |
| `confidence` | 0.95 | Johansen 检验置信水平，只能取 0.90、0.95、0.99 |
| `requireCoint` | 1 | 1：只有当期检验得到协整秩至少为 1 才允许开新仓 |
| `ols` | 0 | 1：改用第 0 腿对其余腿的多元 OLS 权重，此时 `requireCoint` 不起作用 |
| `rawPrice` | 0 | 1：用原始价格拟合并按 `β_i × 价格_i` 换算美元名义权重 |
| `entry` / `exit` / `stop` / `maxHold` | 2 / 0.5 / 4 / 96 | z-score 状态机 |
| `minHalfLife` / `maxHalfLife` | 2 / 72 | 入场半衰期闸门 |
| `dynamicWeights` | 0 | 1：持仓期继续刷新权重 |
| `driftBand` / `inverseVol` | 0.03 / 1 | 漂移缓冲与按波动率倒数分配篮子间敞口 |
| `grossPerBasket` / `maxGross` | 0 / 1 | 单篮子敞口与组合总敞口上限；0 表示 `maxGross / 篮子数` |

`Result.Pairs` 按篮子给出收益和换手。篮子中任一腿中途缺数据时，该篮子当轮平仓并暂停。

## SAResidualPCA 与 SAResidualBench：残差 s-score

按 Avellaneda 与 Lee（2010）的做法：在滚动 `window` 根单期收益上提取系统因子（`SAResidualPCA` 用标准化收益的前 `numFactors` 个主成分，`SAResidualBench` 用 BTC、ETH 收益做多元 OLS），逐资产得到残差，累计后用 OU 过程估计均衡位置和尺度，得到中心化 s-score。`s <= -entryS` 做多，`s >= entryS` 做空，回到 `exitS` 以内止盈，`|s| >= stopS` 或持仓满 `maxHold` 根平仓。目标权重经投影后严格满足美元中性（权重和为 0）与因子 Beta 中性。

| 参数 | 默认 | 含义 |
| --- | --- | --- |
| `window` | 60 | 收益回归窗口，需要 `window + 1` 根价格；至少为 `max(6, 因子数 + 3)` |
| `ouWindow` | 0 | 累计残差的 OU 窗口，0 取 `window`，范围 `[4, window]` |
| `numFactors` | 3 | 仅 PCA：主成分个数 |
| `entryS` / `exitS` / `stopS` | 1.25 / 0.5 / 3 | 入场、双边止盈、止损 s-score；`exitS < entryS`，`stopS` 为 0 或大于 `entryS` |
| `maxHold` | 48 | 单资产最大持仓 bar 数，0 关闭 |
| `minHalfLife` / `maxHalfLife` | 0 / 30 | OU 半衰期（bar 数）入场范围，0 不限该侧 |
| `inverseSigma` | 1 | 1：主动意愿按 `1/σ_eq` 缩放；0：等权 |
| `targetGross` | 1 | 组合总名义占 NAV 的比例，范围 `(0, 1]` |
| `maxWeight` | 0.15 | 单资产绝对权重上限，0 不限（[residual-benchmark.yml](residual-benchmark.yml) 写 0.2） |
| `driftBand` | 0.02 | 活跃仓位集合不变时的权重漂移缓冲 |

`SAResidualPCA` 的模板（[residual-pca.yml](residual-pca.yml)）还演示了滚动流动性宇宙：

```yaml
universe:
  mode: rolling_liquid_top
  top_n: 20
  lookback_days: 30
  min_listing_days: 14
  reconstitute_days: 30
  always_include: ['BTC/USDT:USDT', 'ETH/USDT:USDT']
```

根级 `pairs` 是候选超集（22 个合约）。每 30 天只用当时已可见的 K 线成交额重选前 20 个进入可投资宇宙，上市不足 14 天的不入选；退出宇宙的仓位在下一个决策平掉。`SAResidualBench` 的两个基准（BTC、ETH）必须在 `pairs` 里，否则构造组合时报错。

## 常见问题与注意事项

- **没有成交**：先检查 1m K 线和价格流配置；其次检查 `time_range` 是否至少覆盖 `window`（滚动配对是 `formation`）加交易期。预热期没有目标，不是错误。
- **阈值与成本**：`entry: 2` 只是常用的起点。每次往返要付手续费、滑点，永续还有资金费；请核对 `(entry - exit) × 残差波动` 是否明显大于往返成本，否则提高 `entry` 或放弃该对。
- **样本偏差**：固定配对示例里的两对是演示用的常见组合，不是在某个数据区间上挑出来的协整对。用全样本挑“协整对”再回测会引入选择偏差；滚动配对策略每期只用当时可见数据选对，更接近可交易流程。数值上通过检验不等于显著或可盈利。
- **验证范围**：本目录的测试只覆盖配置解析、参数校验和合成数据下的行为，没有用真实行情或数据库回测过；示例面向权重回测，实盘需要真实合约单位、精度、账户绑定和资金费来源。
- **窗口与缺口**：收益窗口还没填满的资产不产生信号；某根 bar 缺数据时，该资产在收益窗口里记为缺失（不补零），窗口含缺失期间不产生信号。滚动流动性宇宙下新进入的资产同样要等窗口填满。

## 测试

```sh
GOWORK=off go test ./examples/statarb/...
```

测试不访问数据库或网络：用 banbot 的统一配置加载器解析每个 YAML，重建工厂配置并做静态校验，再用合成数据构造每个策略的组合构造器，检查参数校验、拼错参数、交易方向、美元中性等行为。
