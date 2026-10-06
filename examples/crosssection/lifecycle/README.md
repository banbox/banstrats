# 组合生命周期与研究示例

这些 YAML 是**策略覆盖文件**，与自己的市场配置 `your-runtime.yml` 一起加载；基础配置提供数据库、市场、交易对池、时间范围及资金。每个文件完整替换 `run_policy`，因此一次选择一个即可。配置采用 2h 因子决策和 1m 执行价格流，需要真实数据覆盖这两个周期。

在 banstrats 根目录构建并运行：

```sh
go build -o bot .
./bot backtest --config your-runtime.yml --config examples/crosssection/lifecycle/min-linear-quantity.yml
./bot research --config your-runtime.yml --config examples/crosssection/lifecycle/research-horizons.yml
```

Windows 可构建 `bot.exe` 后使用 `./bot.exe`。这些配置使用 `execution.mode: weights`，手续费/滑点是示例假设，`explicit-zero` 明确假设零 funding。它们没有账户绑定，也不代表真实订单、真实成交或经过验证的收益结果。永续市场应在自己的配置中替换成本及资金费率假设。

| 文件 | 含义 | 最先修改的字段 |
| --- | --- | --- |
| [direct.yml](direct.yml) | 每 2h 选股，独立多头 3 个/空头 2 个；持满 16h，同侧排名前 4 的旧仓保留，其余落选旧仓每侧每轮最多退出 2 个；最长 48h | `selection`、`holding` |
| [min-linear-quantity.yml](min-linear-quantity.yml) | 首次真实成交后至少 16h，落选后用 8 次调仓将退出锚点数量归零 | `min_bars`、`exit_steps`、`on_reselect` |
| [cohort.yml](cohort.yml) | 每 2h 建一个寿命 16h 的组合批次，逐步部署，每轮约使用总额度的 1/8 | `period_bars`、`startup`、`sizing` |
| [geometric.yml](geometric.yml) | 最短 16h 后落选，数量按锚点 × 0.5 的递减轨迹退出；低于锚点的 1% 后归零 | `ratio`、`final_threshold` |
| [target-step.yml](target-step.yml) | 每轮向新的权重目标移动差额的 25%，包括新增及退出 | `alpha` |
| [selection-constraints.yml](selection-constraints.yml) | 两侧各取有效得分池的 20%，按绝对得分分配，限制单资产、净额及普通换手 | `quantile`、`reserve_ratio`、各 cap |
| [research-horizons.yml](research-horizons.yml) | 同一个冻结截面独立观察 2h/8h/16h 可执行收益标签 | `research.labels`、`decision.max_pending` |

`every_bars`、`min_bars`、`max_bars`、`period_bars` 都以 **2h 决策网格**计数。调仓间隔、最短持仓、最长持仓和批次周期分别配置；没有全局 `swapPerBars`/`holdBars` 别名。最短/最长持仓年龄来自真实首次 fill；最长期限会在首个到期监控网格生成零目标，实际退出仍由执行器收敛。

“持满 16h 后落选再分 8 次退出”与“每 2h 建一个 16h 批次”具有不同资金部署和到期行为。数量退出固定标准资产单位，并按真实精度减少，价格/NAV 变化不会把已减仓数量买回；`basis: weight` 则允许按 NAV 重估。`on_reselect: finish` 表示重新入选时仍完成退出，`resume` 暂停后续退出并在再次落选时继续，`restore` 恢复理想目标。最大年龄、风险强平等硬退出优先于普通过渡与换手限制。

`min-linear-quantity.yml` 中 `BTC/USDT:USDT` 只是资产覆盖的写法示例，应改为自己 SIDMap 的数据 symbol；不在池中的覆盖不会凭空增加资产。显式 `by_asset` 优先于 Go resolver，resolver 优先于默认规则。分位选择无需旧 `k`；多空池互斥，数量不足时按 `missing_scores` 处理。约束可能留现金或无法满足，不能把目标敞口当保证成交。

多 horizon 标签分别等待执行价格流成熟，末尾没有成熟的长 horizon 留在未完成统计中；未来标签不参与当期选股。8h/16h 与 2h 采样重叠，不能把样本数视作独立样本数。这里使用固定组合权重；若改为历史 IC 权重，应明确 `combo.label`，多标签省略时选择最短期限。历史 IC 组合当前用于研究/回放，live 没有实时成熟历史 provider。

## Go 扩展：按已发布研究产物解析规则

[parameter_context.go](../parameter_context.go) 提供可复制修改的 `NewParameterContext`，将资产、分组、全局参数建议接入 `runner.Config.PolicyContext`。它每轮先调用 `research.ResolveParameters` 检查训练 manifest、内容 hash、训练截止和发布时间，然后填写 `HoldingRules`/`TransitionRules`。已退出选股池但仍有仓位的 SID 也参与规则解析。示例接收 `min_bars`、`max_bars`、`exit_steps`；改成自己的 schema 或使用完整 `runner.RegisterPortfolioPolicy("my-policy-v1", factory)` 可以自由定义调度、分配及有界 JSON 状态。

```go
resolver, err := crosssection.NewParameterContext(trainingManifestID, artifacts)
if err != nil { return err }
cfg.PolicyContext = resolver
// cfg.Manifest.Portfolio 使用 lifecycle-v1 + linear-exit 配置。
result, err := runner.Run(ctx, cfg, sink, output)
```

完整、可执行的 [policyresearch/main.go](../policyresearch/main.go) 用合成训练观测调用 `SelectParameters`，冻结产物后解析两种期限候选，再用真实 lifecycle policy API 生成一个目标；不会连接数据库或发送订单：

```sh
go run ./examples/crosssection/policyresearch
go test ./examples/crosssection/...
go vet ./examples/crosssection/...
```

输出 `Synthetic: true`、产物 ID、规则与目标，是接口行为演示，不能据此推荐某个真实资产的最佳持仓周期。真实研究应使用独立训练/样本外窗口和成本后观测。组合参数扫描可用 `runner.ScanPortfolioTrials`，必须给出显式上限并使用隔离的 `weights` 模拟；测试窗口不参与选择训练参数。

此 resolver 填写的是完整 `HoldingRule`；候选中省略 `min_bars`/`max_bars` 时按 0 处理，覆盖全局默认。需要默认最长期限时应在候选产物中显式声明，或按自己的规则调整 resolver。`exit_steps` 必须为正整数；没有可见产物、hash 损坏或非法规则会报错。

`group_quota`/`group_caps` 需要当期可见的 `PortfolioContext.Groups`，`inverse-volatility`/`vol-target` 需要同周期 `Volatility`，`beta_cap` 需要 `Beta`。普通 CLI 不会自动从 Frame 填写这些映射。接入方式是在自己的 Go runner 中组合 `cfg.PolicyContext` 回调，先从冻结且 PIT 可见的数据填映射，再调用 resolver；想通过 CLI 暴露自定义逻辑，可注册版本化 policy 或 portfolio builder 后在 YAML 选择注册名。原始字段仍通过 `DataSeries.Values map[string]any` 传递，不能把 NULL 当零暴露。

本地联调 banbot 工作树时，在临时目录创建 Go workspace，`go work use` 加入 banstrats、banbot 及 banbot workspace 里的 banexg/banta，再把 `GOWORK` 指向该临时 `go.work` 后运行上述命令。不要修改 banbot 的共享 `go.work` 或版本依赖文件。

例如 Windows 下四个仓库位于 `D:\ban`，在 banstrats 根目录执行：

```powershell
$factorWork = Join-Path ([System.IO.Path]::GetTempPath()) ('factor-example-' + [guid]::NewGuid() + '.work')
$previousGoWork = $env:GOWORK
[System.IO.File]::WriteAllText($factorWork, "go 1.26.0`n", [System.Text.UTF8Encoding]::new($false))
$env:GOWORK = $factorWork
try {
    go work edit -toolchain go1.26.8 -use 'D:\ban\banbot' -use 'D:\ban\banstrats' -use 'D:\ban\banexg' -use 'D:\ban\banta'
    go test ./examples/crosssection/...
    go run ./examples/crosssection/policyresearch
} finally {
    $env:GOWORK = $previousGoWork
    Remove-Item -LiteralPath $factorWork -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath ($factorWork + '.sum') -ErrorAction SilentlyContinue
}
```

工具链版本随本地 banbot `go.work` 调整。Windows 的 workspace 路径用本机反斜杠路径交给 `go work edit`，避免手写正斜杠路径导致模块归属检测不一致。
