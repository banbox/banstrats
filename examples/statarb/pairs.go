package statarb

import (
	"math"

	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/factor/statistics"
)

// legVolFeature is the per-leg realised-volatility column exposed to EntryFilter.
const legVolFeature = "vol"

// pairsSpec declares two fixed log-OLS pairs (ETH/BTC and SOL/ETH).
//
// Parameters (all optional, YAML `params`):
//
//	window, entry, exit, stop, maxHold   z-score band on the rolling log-OLS residual
//	gross             gross notional of one open pair as a NAV fraction (default 0.5; 2 pairs)
//	minBeta           reject entries whose hedge ratio (elasticity) is <= this value
//	inverseVol        1: split total gross across open pairs by 1/residual-std, 0: fixed gross
//	driftBand         hold drifted weights while the L1 distance to the target is below this
//	dynamicExitZ      1: exit z uses the refitted slope instead of the entry-frozen hedge ratio
//	minHalfLife/maxHalfLife   online FitOU half-life gate in bars (0 disables a bound)
//	minCorr           1-bar return correlation gate over `window`
//	maxHurst          spread Hurst gate (0 disables; 0.5 = mean-reverting regimes only)
//	maxLegVol         EntryFilter: skip entries when either leg's 24-bar return std exceeds this
//	kalman            1: Kalman random-walk hedge instead of rolling log-OLS (kalmanDelta, kalmanVe; 0 = defaults)
//	dynamicHedge      1: keep refreshing an open pair's hedge ratio instead of freezing it at entry
func pairsSpec(p *runner.Params) (runner.PairsSpec, error) {
	spec := runner.PairsSpec{
		Pairs: []runner.Pair{
			{Y: "ETH/USDT:USDT", X: "BTC/USDT:USDT"},
			{Y: "SOL/USDT:USDT", X: "ETH/USDT:USDT"},
		},
		Window: p.Int("window", 120),
		Band: statistics.BandConfig{
			Entry:   p.Float("entry", 2),
			Exit:    p.Float("exit", 0.5),
			Stop:    p.Float("stop", 4),
			MaxHold: p.Int("maxHold", 0),
		},
		Gross:          p.Float("gross", 0.5),
		MinBeta:        p.Float("minBeta", 0.3),
		DriftBand:      p.Float("driftBand", 0.02),
		DynamicExitZ:   p.Bool("dynamicExitZ", false),
		MinHalfLife:    p.Float("minHalfLife", 0),
		MaxHalfLife:    p.Float("maxHalfLife", 48),
		MinRollingCorr: p.Float("minCorr", 0.6),
		MaxHurst:       p.Float("maxHurst", 0.5),
	}
	if p.Bool("inverseVol", true) {
		spec.WeightMode = "inverse_vol"
	}
	kalman := p.Bool("kalman", false)
	delta, ve := p.Float("kalmanDelta", 0), p.Float("kalmanVe", 0)
	spec.DynamicHedge = p.Bool("dynamicHedge", false)
	if kalman {
		spec.SpreadModel, spec.KalmanDelta, spec.KalmanVe = "kalman", delta, ve
	}
	maxLegVol := p.Float("maxLegVol", 0.03)
	if maxLegVol > 0 {
		spec.FeatureExprs = map[string]string{legVolFeature: "ts.std(ts.return(kline.close, 1), 24, 0)"}
		spec.EntryFilter = volatilityGate(maxLegVol)
	}
	return spec, rejectUnused(NamePairs, p)
}

// volatilityGate blocks new entries while either leg is in a volatility spike
// and while the feature is still warming up. It only reads point-in-time leg
// features, and the framework never applies it to exits or stops.
func volatilityGate(limit float64) func(runner.PairFilterContext) bool {
	return func(c runner.PairFilterContext) bool {
		y, okY := c.FeatureY(legVolFeature)
		x, okX := c.FeatureX(legVolFeature)
		return okY && okX && !math.IsNaN(y) && !math.IsNaN(x) && y <= limit && x <= limit
	}
}
