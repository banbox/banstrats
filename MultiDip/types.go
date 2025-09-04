package newstrategy4

// New4More carries aggregated multi-timeframe infos
// set at OnStartUp and updated in OnInfoBar
// Note: keep fields exported only if cross-file access is needed
// to avoid import cycles and for clarity.
type New4More struct {
	SupLevel1h     float64
	Cmf1h          float64
	Rsi14_1h       float64
	Roc1h          float64
	R480_1h        float64
	CtI40_1h       float64
	Ema200IsRising bool
} 