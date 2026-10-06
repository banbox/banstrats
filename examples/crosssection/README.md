# 截面与多因子示例

截面策略在同一时点比较多个资产的因子得分，选择得分较高的资产做多、较低的资产做空。本目录提供动量、反转、低波、趋势和多因子组合等示例，主要使用加密资产的价格与成交量数据。

本目录已适配 Banbot v0.6.0-beta.6，使用 Go 1.26.0+。生命周期配置与参数解析扩展见 [新增示例指南](lifecycle/README.md)。

你可以用两种方式定义因子，两者都使用 `engine: factor`，并支持因子研究、回测和交易：

| 实现方式 | 从哪里开始 | 适合的场景 |
| --- | --- | --- |
| Go 因子图 | [strategies.go](strategies.go) 和 [runtime.yml](runtime.yml) | 使用已注册的示例策略，或编写需要自定义处理的因子 |
| YAML 表达式 | [expressions.yml](expressions.yml) | 直接修改公式、窗口和组合权重，尝试新因子，无须新增 Go 策略或重新编译程序 |
| 组合生命周期 | [lifecycle/README.md](lifecycle/README.md) | 配置调仓、持仓期限、数量分批退出、cohort、约束与多期限研究 |
| Go 参数 resolver | [policyresearch/main.go](policyresearch/main.go) | 用已发布研究产物决定每资产规则；可执行的无数据库合成示例 |

如果只是调整公式，建议从 YAML 表达式开始；如果要直接运行下面的经典示例，可以使用已注册的策略名称。

## 快速开始

在项目根目录构建程序：

```sh
go build -o bot .
```

以下命令使用 `./bot`；Windows 下可构建为 `bot.exe`，再使用 `./bot.exe`。

准备自己的基础配置，例如 `your-runtime.yml`，其中包含数据库、交易市场、静态交易对池和回测时间范围 `time_range`。本目录的两个 YAML 都是策略覆盖文件，需要与基础配置一起使用。

运行已注册的 Go 多因子策略：

```sh
./bot backtest --config your-runtime.yml --config examples/crosssection/runtime.yml
```

运行 YAML 中定义的风险调整动量与反转组合：

```sh
./bot backtest --config your-runtime.yml --config examples/crosssection/expressions.yml
```

建议先复制对应模板，再修改自己的策略配置。两个模板默认每天计算一次因子，使用 1m 闭合 K 线的收盘价作为成交价流，因此数据库需要覆盖交易对池中全部资产的日线和 1m 数据。

模板默认采用权重回测（`execution.mode: weights`），`k: 3` 表示选择得分最高的 3 个资产做多、最低的 3 个资产做空。`portfolio.long_notional: 0.5` 和 `short_notional: 0.5` 分别分配多头与空头名义敞口；选满时，每个多头权重为 `1/6`，每个空头权重为 `-1/6`。交易对池需要有足够的有效资产供两侧选择。

## 方式一：使用 Go 因子图

### 选择策略与修改参数

[runtime.yml](runtime.yml) 默认使用 `CSMultiFactor`。修改 `run_policy` 条目中的 `name` 可以选择其他已注册策略，并在 `params` 中设置该策略支持的参数。

例如，使用 30 根 K 线的动量，跳过最近 1 根，每侧选择 3 个资产。将模板中的 `name` 和 `params` 替换为：

```yaml
name: CSMomentum
params:
  k: 3
  momentum_window: 30
  skip: 1
```

切换策略时，只保留该策略支持的因子参数和通用组合参数 `k`。例如，`CSMomentum` 不接受 `reversal_window` 或 `volatility_window`，保留这些参数会报错。

窗口参数按**输入 K 线根数**计数：日线下的 `20` 是 20 根日线，小时线下则是 20 根小时线。修改 `run_timeframes` 时，需要相应调整窗口。Go 示例参数均为整数，最大值为 10000；`skip` 可为 0，波动率窗口至少为 2，其他窗口及 `k` 至少为 1。

### 新增自己的 Go 因子

在 [strategies.go](strategies.go) 中，用 `factor.Field` 读取字段，用 `factor.Return`、`factor.EMA`、`factor.StdDev` 等算子构建公式，再用 `factor.ZScore` 做截面标准化。`factor.New().Add(...)` 添加输出因子，`Compile()` 编译为执行计划。

扩展本目录的策略时，在 `names` 中加入名称，在 `definitions` 中定义默认参数和因子组合权重，再在 `build` 中添加公式。包初始化时会通过 `runner.RegisterDefinition` 注册；项目主程序已经导入本包，重新构建后即可在 YAML 中使用新名称。现有实现可作为完整示例。

优先使用内置算子。需要自定义计算或额外的数据有效性保护时，再使用 Go 扩展；本目录的流动性中性动量就是这种场景。

## 方式二：使用 YAML 表达式

### 编写公式与组合

[expressions.yml](expressions.yml) 中的 `CSRiskAdjustedMomentum` 直接由 `run_policy[].expressions` 定义，不需要注册同名 Go 策略。它将动量除以波动率，再与短期反转组合：

```yaml
expressions:
  schema_version: 1
  timeframe: 1d
  bindings:
    kline: {source: kline, timeframe: 1d}
  params:
    momentum_window: 20
    skip: 1
    reversal_window: 3
    volatility_window: 20
  lets:
    price: 'positive(kline.close)'
    momentum: 'ts.return(ts.lag(factor.price, param.skip), param.momentum_window)'
    volatility: 'ts.std(ts.return(factor.price, 1), param.volatility_window, 0)'
  outputs:
    risk_adjusted: 'cs.zscore(factor.momentum / max(factor.volatility, 1e-6))'
    reversal: 'cs.zscore(-ts.return(factor.price, param.reversal_window))'
  combine:
    method: fixed
    columns: [risk_adjusted, reversal]
    weights: {risk_adjusted: 0.8, reversal: 0.2}
```

这段配置放在策略条目下，完整运行配置见模板。各字段的用途如下：

| 字段 | 用途 | 引用方式或示例 |
| --- | --- | --- |
| `timeframe` | 因子决策周期，应与策略的 `run_timeframes` 一致 | `1d` |
| `bindings` | 为数据源及其周期起别名 | `kline.close`；特殊字段名可用 `field("kline", "field-name")` |
| `params` | 定义公式参数 | `param.momentum_window` |
| `lets` | 定义可复用的中间公式 | `factor.price`、`factor.momentum` |
| `outputs` | 定义输出因子列 | `risk_adjusted`、`reversal` |
| `combine` | 选择输出列并设置合成权重 | 80% 风险调整动量 + 20% 反转 |

公式参数放在 `expressions.params` 中；选股数量 `k` 放在策略条目的 `params` 中，两者分别控制因子计算和组合选择。`combine.columns` 与 `combine.weights` 中的名称应对应 `outputs` 中的列名。

公式支持括号、四则运算、负号、科学记数，以及时序函数（`ts.*`）和截面函数（`cs.*`）。例如，要把模板改为单一趋势因子，可保留 `lets.price`，将 `outputs` 和 `combine` 替换为：

```yaml
outputs:
  trend: 'cs.zscore(factor.price / ts.ema(factor.price, 20) - 1)'
combine:
  method: fixed
  columns: [trend]
  weights: {trend: 1}
```

`positive(...)` 用于要求价格、成交量为正值；`max(..., 1e-6)` 为有效的波动率数值设置分母下限。缺失、NULL、类型错误或预热不足仍会保留为无效值，不会自动填零。

`expressions`、`prices`、`portfolio` 等字段直接放在 `run_policy` 的策略条目下。公式中的 `factor.price` 引用 `lets.price`。

### 使用已注册的表达式示例

除了直接在 YAML 中写公式，本目录还在 [expressions.go](expressions.go) 中注册了六个表达式版本：`CSMomentumExpr`、`CSReversalExpr`、`CSLowVolExpr`、`CSTrendExpr`、`CSVolumeMomentumExpr` 和 `CSMultiFactorExpr`。

使用它们时，将 `runtime.yml` 中的 `name` 改为对应的 `Expr` 名称即可，参数仍放在策略条目的 `params` 中。它们与对应 Go 示例共用默认参数和组合权重。`CSLiquidityNeutralMomentum` 仅提供 Go 版本，用于保留回归输入的原始无效原因。

### 检查公式

将策略中的 **`expressions` 内部内容**单独保存为 `formula.yml`（从 `schema_version` 开始，去掉外层 `expressions` 和 `run_policy`），即可检查公式是否能编译，并查看输入、输出及预热长度：

```sh
./bot validate --spec formula.yml
./bot explain --spec formula.yml
```

这两个命令不需要连接数据库或交易账户。它们检查公式与组合配置；实际因子数值还需要结合行情数据验证。

## 示例因子介绍

下面的窗口均为默认值。各因子先在同一时点的参考资产池中做 Z-score 标准化，再按固定权重合成为得分；表中的多因子权重作用于标准化后的因子。

| 策略名称 | 因子计算 | 选择倾向 | 默认参数 |
| --- | --- | --- | --- |
| `CSMomentum` | 跳过最近 1 根后的 20 根收益率 | 买强卖弱 | `momentum_window: 20`, `skip: 1` |
| `CSReversal` | 过去 3 根收益率取负 | 买短期输家、卖短期赢家 | `reversal_window: 3` |
| `CSLowVol` | 过去 20 根单期收益率的总体标准差取负 | 买低波、卖高波 | `volatility_window: 20` |
| `CSTrend` | 收盘价 / EMA(20) − 1 | 买趋势强者、卖弱者 | `trend_window: 20` |
| `CSVolumeMomentum` | 0.7 × 动量 + 0.3 × 当期对数成交额 | 偏好强势、高流动性资产 | `momentum_window: 20`, `skip: 1` |
| `CSLiquidityNeutralMomentum` | 将动量对当期对数成交额做截面线性回归，用残差排名 | 选择剔除流动性线性影响后仍较强的资产 | `momentum_window: 20`, `skip: 1` |
| `CSMultiFactor` | 0.5 × 动量 + 0.2 × 反转 + 0.3 × 低波 | 综合趋势延续、短期反转与低波偏好 | `momentum_window: 20`, `skip: 1`, `reversal_window: 3`, `volatility_window: 20` |

动量中 `skip: 1`、`momentum_window: 20` 对应 `close[t-1] / close[t-21] - 1`。跳过最近一根可以将较短期的价格变化与动量信号分开。

成交额使用 `close × volume` 的代理值，其对数通过 `log(close) + log(volume)` 计算。因此，`CSVolumeMomentum` 的成交量部分衡量当期流动性水平；它不是成交量增长率，也不是交易所提供的精确 `quote_volume`。流动性中性动量只消除当期对数成交额的线性影响，仍可能有行业或市场暴露。

YAML 模板中的 `CSRiskAdjustedMomentum` 是额外的组合示例：80% 风险调整动量 + 20% 反转。风险调整动量将跳过最近一根的 20 根收益率除以最近 20 根单期收益率的标准差，再做截面标准化，偏好相对波动率而言涨幅较强的资产。

## 用于研究与交易

两种实现方式使用同一套因子引擎。回测使用 `bot backtest`，因子研究使用 `bot research`，交易使用 `bot trade`。

两个模板都设置了 `research.labels: []`，用于运行固定权重策略，不等待未来收益标签。研究 IC、RankIC 等指标时，需要先在自己的配置中设置真实的收益标签与预测期限，评估时只使用已经成熟的标签，再运行：

```sh
./bot research --config your-runtime.yml --config your-strategy.yml
```

连接交易账户前，基础配置还需要真实合约单位、精度和账户执行绑定。两个模板的 `explicit-zero` 表示资金费率按零处理，使用永续合约交易时应配置真实资金费率来源；手续费、滑点和名义敞口也需要按自己的市场设置。多策略共用账户时，应明确分配各策略的 `capital_weight`。

本目录示例覆盖价格与成交量因子。股票价值、盈利质量、财务因子或资金费率因子需要对应的数据源。
