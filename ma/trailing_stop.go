package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

func TrailStop(p *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum:  100,
		TimeFrames: "1h",
		OnBar: func(s *strat.StratJob) {
			e := s.Env
			atr := ta.ATR(e.High, e.Low, e.Close, 14).Get(0)
			atrPct := atr * 100 / e.Close.Get(0)
			k, d, _ := ta.KDJ(e.High, e.Low, e.Close, 9, 3, 3)
			kdCross := k.Cross(d)
			if len(s.LongOrders) == 0 && kdCross == 1 {
				s.OpenOrder(&strat.EnterReq{
					Tag:         "long",
					CallbackPct: 5 * atrPct,
				})
			}
		},
	}
}
