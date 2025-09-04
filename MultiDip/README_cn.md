# MultiDip

- `register.go`: 策略分组注册（组名/键：`MultiDip`）
- `types.go`: 结构体与共享类型
- `params.go`: 参数集中管理
- `indicators.go`: 指标与工具函数
- `infos.go`: 多周期信息汇聚（`OnInfoBar`）
- `strategy.go`: 主策略（`NewStrategy4`，Name=`MultiDip`）

简述：多因子下跌回撤均值回归策略，结合 BB 下轨/宽度、VWAP 下沿、EWO、CTI、RMI、CCI、StochRSI 与 PMAX/EMA 趋势过滤。 