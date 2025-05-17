package ma

import (
	"fmt"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/log"
)

func ws(p *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WsSubs: map[string]string{
			core.WsSubDepth: "",
			core.WsSubTrade: "",
			core.WsSubKLine: "",
		},
		OnBar: func(s *strat.StratJob) {
			e := s.Env
			log.Info(fmt.Sprintf("OnBar %v: %v", e.TimeStop, e.Close.Get(0)))
		},
		OnWsKline: func(s *strat.StratJob, pair string, k *banexg.Kline) {
			log.Info(fmt.Sprintf("OnWsKline %v: %v", k.Time, k.Close))
		},
		OnWsTrades: func(s *strat.StratJob, pair string, trades []*banexg.Trade) {
			last := trades[len(trades)-1]
			log.Info(fmt.Sprintf("OnWsTrades %v %v, %v", last.Timestamp, last.Price, last.Amount))
		},
		OnWsDepth: func(s *strat.StratJob, dep *banexg.OrderBook) {
			bp1, bm1 := dep.Bids.Price[0], dep.Bids.Size[0]
			ap1, am1 := dep.Asks.Price[0], dep.Asks.Size[0]
			log.Info(fmt.Sprintf("OnWsDepth %v %v, %v,, %v, %v", dep.TimeStamp, bp1, bm1, ap1, am1))
		},
	}
}
