// Package carry registers a spot plus linear-perpetual basis strategy for the
// banbot factor engine. It is a RegisterPairs strategy whose legs live in two
// markets of one exchange: Y is the perpetual ("BTC/USDT:USDT"), X the spot
// ("BTC/USDT"). The run must declare `market_type: linear` with
// `extra_markets: [spot]` so both symbols share one run.
//
// When the perpetual trades rich against spot (basis z-score above the entry
// threshold) the strategy sells the perpetual and buys the same dollar amount
// of spot, collects funding on the short perpetual and exits when the basis
// reverts. It never shorts spot: Band.ShortOnly forbids the opposite spread and
// LongOnlySpot blocks any entry that would sell a spot leg.
package carry

import (
	"fmt"

	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/factor/statistics"
)

// Name is the registered definition name, also usable as `run_policy[].name`.
const Name = "CarryBasis"

func init() {
	if err := runner.RegisterPairs(Name, spec); err != nil {
		panic(err)
	}
}

// spec builds a basis-reversion carry book over BTC and ETH.
//
// Parameters (all optional, YAML `params`):
//
//	window            rolling window in decision bars for the perp/spot ratio z-score (default 168 = 7 days of 1h)
//	entry, exit, stop z-score thresholds on the ratio (short the rich basis at entry)
//	maxHold           force exit after this many decision bars (0 disables)
//	gross             gross notional of one open pair as a NAV fraction (default 0.5; two pairs)
//	driftBand         hold drifted weights while the L1 distance to the target is below this
func spec(p *runner.Params) (runner.PairsSpec, error) {
	out := runner.PairsSpec{
		Pairs: []runner.Pair{
			{Y: "BTC/USDT:USDT", X: "BTC/USDT"},
			{Y: "ETH/USDT:USDT", X: "ETH/USDT"},
		},
		Window: p.Int("window", 168),
		// The ratio model hedges one-for-one in dollars (hedge ratio 1), so each open pair
		// holds gross/2 short perpetual and gross/2 long spot.
		SpreadModel: "ratio",
		Band: statistics.BandConfig{
			Entry:     p.Float("entry", 2),
			Exit:      p.Float("exit", 0.5),
			Stop:      p.Float("stop", 5),
			MaxHold:   p.Int("maxHold", 72),
			ShortOnly: true,
		},
		LongOnlySpot: true,
		Gross:        p.Float("gross", 0.5),
		DriftBand:    p.Float("driftBand", 0.01),
	}
	if unused := p.Unused(); len(unused) > 0 {
		return out, fmt.Errorf("%s: unknown parameters %v", Name, unused)
	}
	return out, p.Err()
}
