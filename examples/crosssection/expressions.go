// Package crosssection keeps expression variants alongside the original examples.
package crosssection

import (
	"fmt"
	"slices"
	"strings"

	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/expr"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
)

var expressionNames = []string{"CSMomentumExpr", "CSReversalExpr", "CSLowVolExpr", "CSTrendExpr", "CSVolumeMomentumExpr", "CSMultiFactorExpr"}

// ExpressionNames returns the additive expression variants; Names retains originals.
func ExpressionNames() []string { return slices.Clone(expressionNames) }

func init() {
	for _, name := range expressionNames {
		if err := runner.RegisterDefinition(name, func(c runner.Config) (*factor.Plan, research.ComboSpec, error) {
			return exprBuild(name, c)
		}); err != nil {
			panic(err)
		}
	}
}

func exprBuild(name string, c runner.Config) (*factor.Plan, research.ComboSpec, error) {
	name = strings.TrimSuffix(name, "Expr")
	if !slices.Contains(expressionNames, name+"Expr") {
		return nil, research.ComboSpec{}, fmt.Errorf("crosssection: unknown expression definition %q", name)
	}
	params, combo, err := settings(name, c)
	if err != nil {
		return nil, research.ComboSpec{}, err
	}
	spec := expr.Spec{
		SchemaVersion: 1,
		TimeFrame:     c.Factor.TimeFrame,
		Bindings:      map[string]expr.Binding{"kline": {Source: c.Factor.Source, TimeFrame: c.Factor.TimeFrame}},
		Params:        params,
		Lets:          map[string]string{"price": fmt.Sprintf("positive(field(%q, %q))", "kline", c.Factor.Field)},
		Outputs:       map[string]string{},
	}
	add := func(column, expression string) {
		spec.Outputs[column] = "cs.zscore(" + expression + ")"
	}
	momentum := "ts.return(ts.lag(factor.price, param.skip), param.momentum_window)"
	reversal := "-ts.return(factor.price, param.reversal_window)"
	lowvol := "-ts.std(ts.return(factor.price, 1), param.volatility_window, 0)"
	liquidity := "log(factor.price) + log(positive(kline.volume))"
	switch name {
	case "CSMomentum":
		add("momentum", momentum)
	case "CSReversal":
		add("reversal", reversal)
	case "CSLowVol":
		add("low_volatility", lowvol)
	case "CSTrend":
		add("trend", "factor.price / ts.ema(factor.price, param.trend_window) - 1")
	case "CSVolumeMomentum":
		add("momentum", momentum)
		add("liquidity", liquidity)
	case "CSMultiFactor":
		add("momentum", momentum)
		add("reversal", reversal)
		add("low_volatility", lowvol)
	}
	spec.Combine = combo
	plan, err := expr.Compile(spec)
	return plan, combo, err
}
