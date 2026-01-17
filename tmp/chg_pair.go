package tmp

import (
	"github.com/banbox/banbot/btime"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg/log"
	ta "github.com/banbox/banta"
	"go.uber.org/zap"
)

// ChangePair 动态切换交易对
func ChangePair(pol *config.RunPolicyConfig) *strat.TradeStrat {
	changed := false
	return &strat.TradeStrat{
		WarmupNum: 20,
		OnBar: func(s *strat.StratJob) {
			e := s.Env

			ma5 := ta.SMA(e.Close, 5)
			ma20 := ta.SMA(e.Close, 20)
			maCrx := ma5.Cross(ma20)

			if maCrx == 1 {
				s.OpenOrder(&strat.EnterReq{Tag: "long"})
				if len(s.ShortOrders) > 0 {
					s.CloseOrders(&strat.ExitReq{Tag: "exitS", Dirt: core.OdDirtShort})
				}
			} else if maCrx == -1 {
				s.OpenOrder(&strat.EnterReq{Tag: "short", Short: true})
				if len(s.LongOrders) > 0 {
					s.CloseOrders(&strat.ExitReq{Tag: "exitL", Dirt: core.OdDirtLong})
				}
			}

			if e.BarNum > 1000 && !changed {
				changed = true
				log.Info("pair changed", zap.String("date", btime.ToDateStr(e.TimeStop, core.DefaultDateFmt)))
				s.Strat.UpdatePairs(strat.PairUpdateReq{
					Add:    []string{"BTC", "BCH"},
					Remove: []string{s.Symbol.Symbol},
				})
			}

		},
	}
}
