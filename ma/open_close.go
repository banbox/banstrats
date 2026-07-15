package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/strat"
)

// Just for demonstration, no trading, no registration required
func openClose(pol *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum:     100,
		RunTimeFrames: []string{"1m"},
		OnData: func(s *strat.StratJob, _ strat.DataEvent) {
			if len(s.LongOrders) == 0 {
				s.OpenOrder(&strat.EnterReq{
					Tag: "open",
				})
			} else {
				s.CloseOrders(&strat.ExitReq{
					Tag: "exit",
				})
			}
		},
	}
}
