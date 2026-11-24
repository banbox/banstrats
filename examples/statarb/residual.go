package statarb

import "github.com/banbox/banbot/factor/runner"

// residualSpec holds the parameters shared by both residual s-score variants
// (Avellaneda and Lee 2010): per-asset OU s-scores of the return residual after
// removing systematic factors, then a dollar-neutral and factor-beta-neutral book.
//
// Shared parameters (YAML `params`):
//
//	window, ouWindow        return-regression window and OU window in bars (ouWindow 0 = window)
//	entryS, exitS, stopS    s-score thresholds; exitS applies to both sides, stopS 0 disables
//	maxHold                 max holding bars per asset (0 disables)
//	minHalfLife/maxHalfLife OU half-life gate in bars (0 disables a bound)
//	inverseSigma            1: scale active desires by 1/sigma_eq
//	targetGross, maxWeight  book gross in (0, 1] and per-asset |weight| cap (0 = no cap)
//	driftBand               hold weights while the active set is unchanged and drift is small
func residualSpec(p *runner.Params, window int) runner.ResidualStatArbSpec {
	spec := runner.ResidualStatArbSpec{
		Window:            p.Int("window", window),
		OUWindow:          p.Int("ouWindow", 0),
		EntryS:            p.Float("entryS", 1.25),
		ExitLongS:         p.Float("exitS", 0.5),
		StopS:             p.Float("stopS", 3),
		MaxHoldBars:       p.Int("maxHold", 48),
		MinHalfLife:       p.Float("minHalfLife", 0),
		MaxHalfLife:       p.Float("maxHalfLife", 30),
		TargetGross:       p.Float("targetGross", 1),
		MaxWeightPerAsset: p.Float("maxWeight", 0.15),
		DriftBand:         p.Float("driftBand", 0.02),
	}
	spec.ExitShortS = spec.ExitLongS
	if p.Bool("inverseSigma", true) {
		spec.WeightMode = "inverse_sigma"
	}
	return spec
}

// residualPCASpec extracts `numFactors` principal components of the pool's
// standardized returns and trades the idiosyncratic residual of every asset in
// the point-in-time investable universe (root `pairs`, optionally narrowed by a
// rolling liquid_top universe).
func residualPCASpec(p *runner.Params) (runner.ResidualStatArbSpec, error) {
	spec := residualSpec(p, 60)
	spec.FactorModel = "pca"
	spec.NumFactors = p.Int("numFactors", 3)
	return spec, rejectUnused(NameResidualPCA, p)
}

// residualBenchSpec regresses returns on BTC and ETH instead of principal
// components. Both benchmarks must be part of the run's symbol pool; they are
// also traded as ordinary assets unless excluded by the universe.
func residualBenchSpec(p *runner.Params) (runner.ResidualStatArbSpec, error) {
	spec := residualSpec(p, 60)
	spec.FactorModel = "benchmarks"
	spec.Benchmarks = []string{"BTC/USDT:USDT", "ETH/USDT:USDT"}
	return spec, rejectUnused(NameResidualBench, p)
}
