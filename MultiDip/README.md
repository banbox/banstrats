# MultiDip

- `register.go`: register strat group (group/key: `MultiDip`)
- `types.go`: structs and shared types
- `params.go`: centralize parameters
- `indicators.go`: indicators and helpers
- `infos.go`: multi-timeframe info aggregation (`OnInfoBar`)
- `strategy.go`: main strategy (`NewStrategy4`, Name=`MultiDip`)

Brief: Multi-factor dip mean-reversion strategy combining BB lower/width, VWAP lower band, EWO, CTI, RMI, CCI, StochRSI, with PMAX/EMA trend filters. 