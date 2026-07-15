package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/strat"
)

func editPairs(p *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		OnSymbols: func(items []string) []string {
			hasBTC := false
			for _, it := range items {
				if it == "BTC/USDT:USDT" {
					hasBTC = true
				}
			}
			if !hasBTC {
				return append([]string{"BTC/USDT:USDT"}, items...)
			}
			return items
		},
		OnData: func(_ *strat.StratJob, _ strat.DataEvent) {},
	}
}
