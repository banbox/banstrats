package tsfeatures

import (
	"github.com/banbox/banbot/btime"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
)

type orbState struct {
	tradedDay int64 // SessionDay of the last entry: at most one trade per session
}

// OpeningRange: US-session opening range breakout on a 5m/15m timeframe.
// s.OpeningRange tracks the first N minutes after 13:30 UTC without lookahead: the range is
// only Ready once its last bar has closed, and bars inside the range cannot trigger entries.
func OpeningRange(pol *config.RunPolicyConfig) *strat.TradeStrat {
	rangeMin := pol.DefInt("rangeMin", 30, core.PNorm(15, 60))
	rr := pol.Def("rr", 2, core.PNorm(1, 4)) // take-profit as a multiple of the range width
	riskRate := pol.Def("riskRate", 0.005, core.PNorm(0.002, 0.02))

	return &strat.TradeStrat{
		WarmupNum:    300, // enough history to replay the previous session
		EachMaxLong:  1,
		EachMaxShort: 1,
		OnStartUp: func(s *strat.StratJob) {
			s.More = &orbState{}
		},
		OnData: strat.RouteData(strat.DataHandlers{
			Main: func(s *strat.StratJob, _ strat.DataEvent) {
				e := s.Env
				// Update the per-job tracker on every bar. Applicable=false means the timeframe does not fit
				// (>= 1d, or coarser than rangeMin), so the strategy stays silent.
				orb := s.OpeningRange(btime.NewYorkOpenMinutes, rangeMin)
				if btime.IsNearSessionEnd(e.TimeStart, e.TimeStop, btime.USCashCloseMinutes) {
					s.CloseOrders(&strat.ExitReq{Tag: "session_end"}) // flat before the US cash close
					return
				}
				st := s.More.(*orbState)
				if !orb.Ready || !orb.InPostRange || st.tradedDay == orb.SessionDay ||
					!btime.SessionUSCash.Contains(e.TimeStart) || s.GetOrderNum(0) > 0 || len(s.Entrys) > 0 {
					return
				}
				c := e.Close.Get(0)
				if c > orb.High {
					// Stop at the range midpoint; RiskRate sizes the position from that distance.
					st.tradedDay = orb.SessionDay
					s.OpenOrder(&strat.EnterReq{Tag: "orb_long", StopLoss: orb.Mid, TakeProfitVal: orb.Width * rr, RiskRate: riskRate})
				} else if c < orb.Low {
					st.tradedDay = orb.SessionDay
					s.OpenOrder(&strat.EnterReq{Tag: "orb_short", Short: true, StopLoss: orb.Mid, TakeProfitVal: orb.Width * rr, RiskRate: riskRate})
				}
			},
		}),
	}
}
