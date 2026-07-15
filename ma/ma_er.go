package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

func DemoER(pol *config.RunPolicyConfig) *strat.TradeStrat {
	smlLen := int(pol.Def("smlLen", 9, core.PNorm(3, 10)))
	bigLen := int(pol.Def("bigLen", 23, core.PNorm(10, 40)))
	erUpp := pol.Def("erUpp", 0.13, core.PNorm(0.1, 0.7))
	return &strat.TradeStrat{
		WarmupNum: 100,
		OnData: func(s *strat.StratJob, _ strat.DataEvent) {
			e := s.Env
			ma5 := ta.SMA(e.Close, smlLen)
			ma20 := ta.SMA(e.Close, bigLen)
			maCrx := ta.Cross(ma5, ma20)

			er := ta.ER(e.Close, 50).Get(0)

			if maCrx == 1 && er < erUpp {
				s.OpenOrder(&strat.EnterReq{Tag: "open"})
			} else if maCrx == -1 {
				s.CloseOrders(&strat.ExitReq{Tag: "exit"})
			}
		},
	}
}
