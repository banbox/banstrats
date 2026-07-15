package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

type Demo2Sta struct {
	bigDirt int
}

func DemoInfo(pol *config.RunPolicyConfig) *strat.TradeStrat {
	smlLen := int(pol.Def("smlLen", 5, core.PNorm(3, 10)))
	bigLen := int(pol.Def("bigLen", 20, core.PNorm(10, 40)))
	return &strat.TradeStrat{
		WarmupNum: 100,
		OnPairInfos: func(s *strat.StratJob) []*strat.PairSub {
			return []*strat.PairSub{
				{Pair: "_cur_", TimeFrame: "1h", WarmupNum: 30},
			}
		},
		OnStartUp: func(s *strat.StratJob) {
			s.More = &Demo2Sta{}
		},
		OnData: strat.RouteData(strat.DataHandlers{
			Main: func(s *strat.StratJob, _ strat.DataEvent) {
				e := s.Env
				m, _ := s.More.(*Demo2Sta)
				ma5 := ta.SMA(e.Close, smlLen)
				ma20 := ta.SMA(e.Close, bigLen)
				maCrx := ta.Cross(ma5, ma20)
				if maCrx == 1 && m.bigDirt > 0 {
					s.OpenOrder(&strat.EnterReq{Tag: "open"})
				} else if maCrx == -1 {
					s.CloseOrders(&strat.ExitReq{Tag: "exit"})
				}
			},
			Info: func(s *strat.StratJob, data strat.DataEvent) {
				close := data.Series("close")
				if close == nil {
					return
				}
				m, _ := s.More.(*Demo2Sta)
				ma5 := ta.SMA(close, smlLen)
				ma20 := ta.SMA(close, bigLen)
				m.bigDirt = ta.Cross(ma5, ma20)
			},
		}),
	}
}
