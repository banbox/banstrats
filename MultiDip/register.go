package newstrategy4

import (
	"github.com/banbox/banbot/strat"
)

func init() {
	strat.AddStratGroup("MultiDip", map[string]strat.FuncMakeStrat{
		"MultiDip": NewStrategy4,
	})
} 