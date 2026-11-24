package statarb

import (
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/factor/statistics"
)

// rollingPairsSpec re-selects pairs from the run's candidate pool (root
// `pairs`) every `reselect` bars using only data visible at that bar, then
// trades the selected pairs with the same band machinery as RegisterPairs.
// No symbol literal appears in Go code.
//
// Parameters (all optional, YAML `params`):
//
//	formation, reselect     formation window and trading-period length in bars
//	pairCount, maxPerSymbol pairs per period, and appearances of one symbol among them
//	distance                1: Gatev minimum-distance selection, 0: Engle-Granger cointegration
//	maxAdf, adfLags         cointegration gate: residual ADF t-statistic upper bound (negative) and lags
//	minCorr                 formation return-correlation floor
//	minHalfLife/maxHalfLife FitOU half-life window in bars on the formation window (0 max = formation)
//	window, entry, exit, stop, maxHold   online band (window 0 = formation)
//	driftBand, inverseVol, minBeta       as in the fixed-pair example
//	onlineHalfLife          1: also gate entries on the online window half-life
//	handoffImmediate        1: flatten unselected pairs on reselection, 0: let them exit gracefully
func rollingPairsSpec(p *runner.Params) (runner.RollingPairsSpec, error) {
	spec := runner.RollingPairsSpec{
		FormationBars:      p.Int("formation", 240),
		ReselectBars:       p.Int("reselect", 120),
		PairCount:          p.Int("pairCount", 4),
		MaxPerSymbol:       p.Int("maxPerSymbol", 2),
		MaxADF:             p.Float("maxAdf", -2.86),
		ADFLags:            p.Int("adfLags", 1),
		MinCorr:            p.Float("minCorr", 0.6),
		MinHalfLife:        p.Float("minHalfLife", 2),
		MaxHalfLife:        p.Float("maxHalfLife", 0),
		Window:             p.Int("window", 120),
		MinBeta:            p.Float("minBeta", 0.3),
		DriftBand:          p.Float("driftBand", 0.02),
		OnlineHalfLifeGate: p.Bool("onlineHalfLife", true),
		Band: statistics.BandConfig{
			Entry:   p.Float("entry", 2),
			Exit:    p.Float("exit", 0.5),
			Stop:    p.Float("stop", 4),
			MaxHold: p.Int("maxHold", 60),
		},
	}
	if p.Bool("distance", false) {
		spec.SelectionMethod = "distance"
	}
	if p.Bool("inverseVol", true) {
		spec.WeightMode = "inverse_vol"
	}
	if p.Bool("handoffImmediate", false) {
		spec.HandoffMode = "immediate"
	}
	return spec, rejectUnused(NameRollingPairs, p)
}
