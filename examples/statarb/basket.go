package statarb

import (
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/factor/statistics"
)

// basketSpec trades two 3-leg baskets. Weights come from a rolling Johansen
// VECM fit (first cointegrating vector) or, with ols=1, from a multivariate
// OLS of leg 0 on the other legs.
//
// Parameters (all optional, YAML `params`):
//
//	window, differenceLags   estimation window in bars and VECM difference lags (window >= 3*(lags+1)+3)
//	confidence               Johansen confidence level, one of 0.90 / 0.95 / 0.99
//	requireCoint             1: new entries need a Johansen rank >= 1 at `confidence`
//	ols                      1: multivariate OLS weights instead of Johansen
//	rawPrice                 1: fit on raw prices and convert to dollar-notional weights
//	entry, exit, stop, maxHold   z-score band of the basket spread
//	minHalfLife/maxHalfLife  online FitOU half-life gate in bars
//	dynamicWeights           1: refit weights while holding instead of freezing them at entry
//	driftBand, inverseVol    as in the pairs example
//	grossPerBasket, maxGross basket and portfolio gross (0 = maxGross / baskets)
func basketSpec(p *runner.Params) (runner.BasketsSpec, error) {
	spec := runner.BasketsSpec{
		Baskets: []runner.BasketDef{
			{Name: "majors", Symbols: []string{"BTC/USDT:USDT", "ETH/USDT:USDT", "BNB/USDT:USDT"}},
			{Name: "alt-l1", Symbols: []string{"SOL/USDT:USDT", "AVAX/USDT:USDT", "NEAR/USDT:USDT"}},
		},
		Window:               p.Int("window", 240),
		DifferenceLags:       p.Int("differenceLags", 1),
		Confidence:           p.Float("confidence", 0.95),
		RequireCointegration: p.Bool("requireCoint", true),
		UseRawPrice:          p.Bool("rawPrice", false),
		MinHalfLife:          p.Float("minHalfLife", 2),
		MaxHalfLife:          p.Float("maxHalfLife", 72),
		DynamicWeights:       p.Bool("dynamicWeights", false),
		DriftBand:            p.Float("driftBand", 0.03),
		GrossPerBasket:       p.Float("grossPerBasket", 0),
		MaxGross:             p.Float("maxGross", 1),
		Band: statistics.BandConfig{
			Entry:   p.Float("entry", 2),
			Exit:    p.Float("exit", 0.5),
			Stop:    p.Float("stop", 4),
			MaxHold: p.Int("maxHold", 96),
		},
	}
	if p.Bool("ols", false) {
		spec.WeightMethod = "multivariate_ols"
	} else {
		spec.WeightMethod = "johansen"
	}
	if p.Bool("inverseVol", true) {
		spec.WeightMode = "inverse_vol"
	}
	return spec, rejectUnused(NameBasket, p)
}
