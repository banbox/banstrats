// Package tsfeatures holds small time-series (engine: time_series) strategies that
// showcase banbot v0.7.4 - v0.7.8 features: risk-based sizing and account
// guards, bracket orders, UTC session / calendar helpers, built-in derivatives
// data sources and neutral batch rebalancing.
// These are teaching examples, not tuned trading systems.
package tsfeatures

import (
	"github.com/banbox/banbot/strat"
)

// Strategy names, referenced from run_policy[].name as "tsfeatures:<name>".
const (
	RiskSizingName     = "risk_sizing"
	BracketName        = "bracket"
	OpeningRangeName   = "orb"
	CalendarDCAName    = "calendar_dca"
	DerivFilterName    = "deriv_filter"
	BatchRebalanceName = "batch_rebalance"
)

func init() {
	strat.AddStratGroup("tsfeatures", map[string]strat.FuncMakeStrat{
		RiskSizingName:     RiskSizing,
		BracketName:        Bracket,
		OpeningRangeName:   OpeningRange,
		CalendarDCAName:    CalendarDCA,
		DerivFilterName:    DerivFilter,
		BatchRebalanceName: BatchRebalance,
	})
}
