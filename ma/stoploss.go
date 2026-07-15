package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

// Just for demonstration, no trading, no registration required
func stoploss(pol *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum:     100,
		RunTimeFrames: []string{"1h"},
		OnPairInfos: func(s *strat.StratJob) []*strat.PairSub {
			return []*strat.PairSub{
				{Pair: "_cur_", TimeFrame: "1m", WarmupNum: 10},
			}
		},
		OnData: strat.RouteData(strat.DataHandlers{
			Main: func(s *strat.StratJob, _ strat.DataEvent) {
				e := s.Env
				c := e.Close.Get(0)
				atr := ta.ATR(e.High, e.Low, e.Close, 14).Get(0)

				// set stoploss when enter
				s.OpenOrder(&strat.EnterReq{
					Tag:      "open",
					StopLoss: c * 0.99,
					//StopLoss: c * 1.01, // for short
					//StopLossVal: c * 0.01 // Same effect
				})

				// use StopLossVal for stoploss range
				s.OpenOrder(&strat.EnterReq{
					Tag:         "open",
					StopLossVal: atr * 2,
				})

				// update stoploss on each bar (here is 1h)
				for _, od := range s.LongOrders {
					od.SetStopLoss(&ormo.ExitTrigger{
						Price: c * 0.99,
					})
				}
				// for _, od := range s.ShortOrders { // for short
			},
			Info: func(s *strat.StratJob, data strat.DataEvent) {
				c := data.Float64("close")
				for _, od := range s.LongOrders {
					od.SetStopLoss(&ormo.ExitTrigger{Price: c * 0.99})
				}
			},
		}),
	}
}
