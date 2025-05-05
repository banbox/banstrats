package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

// Just for demonstration, no trading, no registration required
func takeprofit(pol *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum:     100,
		RunTimeFrames: []string{"1h"},
		OnPairInfos: func(s *strat.StratJob) []*strat.PairSub {
			return []*strat.PairSub{
				{"_cur_", "1m", 10},
			}
		},
		OnBar: func(s *strat.StratJob) {
			e := s.Env
			c := e.Close.Get(0)
			atr := ta.ATR(e.High, e.Low, e.Close, 14).Get(0)

			// set takeprofit when enter
			s.OpenOrder(&strat.EnterReq{
				Tag:        "open",
				TakeProfit: c * 1.01,
				//TakeProfit: c * 0.99, // for short
				//TakeProfitVal: c * 0.01 // Same effect
			})

			// use TakeProfitVal for takeprofit range
			s.OpenOrder(&strat.EnterReq{
				Tag:           "open",
				TakeProfitVal: atr * 2,
			})

			// update takeprofit on each bar (here is 1h)
			for _, od := range s.LongOrders {
				od.SetTakeProfit(&ormo.ExitTrigger{
					Price: c * 0.99,
				})
			}
			// for _, od := range s.ShortOrders { // for short
		},
		OnInfoBar: func(s *strat.StratJob, e *ta.BarEnv, pair, tf string) {
			c := e.Close.Get(0)
			if tf == "1m" {
				// Update takeprofit every minute
				for _, od := range s.LongOrders {
					od.SetTakeProfit(&ormo.ExitTrigger{
						Price: c * 0.99,
					})
				}
			}
		},
	}
}
