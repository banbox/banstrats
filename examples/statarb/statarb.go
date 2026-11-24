// Package statarb registers statistical-arbitrage examples built on the banbot
// factor engine: a fixed cointegrated pair set, rolling point-in-time pair
// formation, a Johansen/OLS multi-leg basket and cross-sectional residual
// s-score strategies (PCA or benchmark factors).
//
// Windows count decision bars: the defaults assume 1h decisions with 1m close
// prices as the execution price stream (see the YAML overlays in this folder).
// Symbols appear exactly as in the run's SIDMap, for example "ETH/USDT:USDT".
// Pair legs and benchmarks are Go literals; the rolling and residual strategies
// take their candidate pool from the root `pairs` list instead.
package statarb

import (
	"fmt"
	"slices"

	"github.com/banbox/banbot/factor/runner"
)

// Registered definition names, also usable as `run_policy[].name`.
const (
	NamePairs         = "SAPairsEthBtc"
	NameRollingPairs  = "SARollingPairs"
	NameBasket        = "SABasketJohansen"
	NameResidualPCA   = "SAResidualPCA"
	NameResidualBench = "SAResidualBench"
)

var names = []string{NamePairs, NameRollingPairs, NameBasket, NameResidualPCA, NameResidualBench}

// Names returns the registered definition names in documentation order.
func Names() []string { return slices.Clone(names) }

func init() {
	must(runner.RegisterPairs(NamePairs, pairsSpec))
	must(runner.RegisterRollingPairs(NameRollingPairs, rollingPairsSpec))
	must(runner.RegisterBasketsParams(NameBasket, basketSpec))
	must(runner.RegisterResidualStatArbParams(NameResidualPCA, residualPCASpec))
	must(runner.RegisterResidualStatArbParams(NameResidualBench, residualBenchSpec))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// rejectUnused turns a misspelled YAML parameter into an error. Params.Unused
// is not called by the framework; without it `entrey: 2` silently keeps the
// default. Call after every parameter has been read.
func rejectUnused(name string, p *runner.Params) error {
	if unused := p.Unused(); len(unused) > 0 {
		return fmt.Errorf("%s: unknown parameters %v", name, unused)
	}
	return p.Err()
}
