// Package crosssection registers classic cross-sectional factor examples.
// Windows count source bars: use daily data for the daily defaults described here.
package crosssection

import (
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
)

var names = []string{"CSMomentum", "CSReversal", "CSLowVol", "CSTrend", "CSVolumeMomentum", "CSLiquidityNeutralMomentum", "CSMultiFactor"}

// The Go and expression graphs share defaults and combination weights.
var definitions = map[string]struct {
	params  map[string]float64
	weights map[string]float64
}{
	"CSMomentum": {
		params:  map[string]float64{"momentum_window": 20, "skip": 1},
		weights: map[string]float64{"momentum": 1},
	},
	"CSReversal": {
		params:  map[string]float64{"reversal_window": 3},
		weights: map[string]float64{"reversal": 1},
	},
	"CSLowVol": {
		params:  map[string]float64{"volatility_window": 20},
		weights: map[string]float64{"low_volatility": 1},
	},
	"CSTrend": {
		params:  map[string]float64{"trend_window": 20},
		weights: map[string]float64{"trend": 1},
	},
	"CSVolumeMomentum": {
		params:  map[string]float64{"momentum_window": 20, "skip": 1},
		weights: map[string]float64{"momentum": .7, "liquidity": .3},
	},
	"CSLiquidityNeutralMomentum": {
		params:  map[string]float64{"momentum_window": 20, "skip": 1},
		weights: map[string]float64{"neutral_momentum": 1},
	},
	"CSMultiFactor": {
		params:  map[string]float64{"momentum_window": 20, "skip": 1, "reversal_window": 3, "volatility_window": 20},
		weights: map[string]float64{"momentum": .5, "reversal": .2, "low_volatility": .3},
	},
}

// Names returns the registered definition names in documentation order.
func Names() []string { return slices.Clone(names) }

func init() {
	for _, name := range names {
		if err := runner.RegisterDefinition(name, func(c runner.Config) (*factor.Plan, research.ComboSpec, error) {
			return build(name, c)
		}); err != nil {
			panic(err)
		}
	}
}

func settings(name string, c runner.Config) (map[string]float64, research.ComboSpec, error) {
	definition, ok := definitions[name]
	if !ok {
		return nil, research.ComboSpec{}, fmt.Errorf("crosssection: unknown definition %q", name)
	}
	params := maps.Clone(definition.params)
	for key, value := range c.Manifest.Parameters {
		if _, exists := params[key]; !exists && key != "k" {
			return nil, research.ComboSpec{}, fmt.Errorf("%s: unknown parameter %q", name, key)
		}
		minimum := 1.0
		if key == "skip" {
			minimum = 0
		}
		if key == "volatility_window" {
			minimum = 2
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < minimum || value > 10000 || math.Trunc(value) != value {
			return nil, research.ComboSpec{}, fmt.Errorf("%s: %s must be an integer in [%g,10000]", name, key, minimum)
		}
		// Entry owns portfolio selection K; it is validated here but must not
		// change the factor graph or its cache identity.
		if key != "k" {
			params[key] = value
		}
	}
	if c.Factor.Source == "" || c.Factor.Field == "" || c.Factor.TimeFrame == "" {
		return nil, research.ComboSpec{}, fmt.Errorf("%s: factor source, price field and timeframe are required", name)
	}
	combo := research.ComboSpec{Method: research.Fixed, Columns: slices.Sorted(maps.Keys(definition.weights)), Weights: maps.Clone(definition.weights)}
	return params, combo, nil
}

func build(name string, c runner.Config) (*factor.Plan, research.ComboSpec, error) {
	params, combo, err := settings(name, c)
	if err != nil {
		return nil, research.ComboSpec{}, err
	}
	price := factor.Positive(factor.Field(c.Factor.Source, c.Factor.Field, c.Factor.TimeFrame))
	momentum := func() *factor.Node {
		input := price
		if params["skip"] > 0 {
			input = factor.Lag(input, int(params["skip"]))
		}
		return factor.Return(input, int(params["momentum_window"]))
	}
	reversal := func() *factor.Node {
		return factor.Neg(factor.Return(price, int(params["reversal_window"])))
	}
	lowvol := func() *factor.Node {
		return factor.Neg(factor.StdDev(factor.Return(price, 1), int(params["volatility_window"]), 0))
	}
	liquidity := func() *factor.Node {
		volume := factor.Positive(factor.Field(c.Factor.Source, "volume", c.Factor.TimeFrame))
		// log(price*volume) is evaluated as log(price)+log(volume) to avoid overflow.
		return factor.Add(factor.Log(price), factor.Log(volume))
	}
	b := factor.New()
	add := func(column string, node *factor.Node) {
		b.Add(column, factor.ZScore(node))
	}
	switch name {
	case "CSMomentum":
		add("momentum", momentum())
	case "CSReversal":
		add("reversal", reversal())
	case "CSLowVol":
		add("low_volatility", lowvol())
	case "CSTrend":
		trend := factor.Sub(factor.Div(price, factor.EMA(price, int(params["trend_window"]))), factor.Constant(1, c.Factor.TimeFrame))
		add("trend", trend)
	case "CSVolumeMomentum":
		add("momentum", momentum())
		add("liquidity", liquidity())
	case "CSLiquidityNeutralMomentum":
		liquid := liquidity()
		residual := factor.Residual(momentum(), liquid)
		// The builtin regression uses complete pairs and marks an invalid X as
		// Missing. Preserve the original liquidity validity in the public column.
		guarded := factor.Custom("banstrats/liquidity-residual-validity-v1", []*factor.Node{liquid, residual}, func(values []factor.Numeric) factor.Numeric {
			if values[0].Validity != factor.Valid {
				return values[0]
			}
			return values[1]
		})
		add("neutral_momentum", guarded)
	case "CSMultiFactor":
		add("momentum", momentum())
		add("reversal", reversal())
		add("low_volatility", lowvol())
	}
	plan, err := b.Compile()
	return plan, combo, err
}
