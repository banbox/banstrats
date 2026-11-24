package tsfeatures

import (
	"github.com/banbox/banbot/btime"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
)

type dcaState struct {
	prevBarMS int64 // previous bar start, required by IsFirstBarOfPeriodUTC
}

// CalendarDCA: monthly dollar-cost averaging during the "Halloween" season (Nov - Apr).
// IsFirstBarOfPeriodUTC fires exactly once per period on any timeframe (5m ... 1d),
// unlike hard-coded "day == 1 && hour == 0" checks. The season is closed on the first
// bar of the first out-of-season month. Long-only, so it also works on spot.
func CalendarDCA(pol *config.RunPolicyConfig) *strat.TradeStrat {
	startMonth := pol.DefInt("startMonth", 11, core.PNorm(1, 12))
	endMonth := pol.DefInt("endMonth", 4, core.PNorm(1, 12)) // startMonth > endMonth wraps over the year end
	maxUnits := pol.Def("maxUnits", 6, core.PNorm(1, 12))    // cap, in multiples of the base stake

	return &strat.TradeStrat{
		WarmupNum: 1,
		OnStartUp: func(s *strat.StratJob) {
			s.More = &dcaState{}
		},
		OnData: strat.RouteData(strat.DataHandlers{
			Main: func(s *strat.StratJob, _ strat.DataEvent) {
				st := s.More.(*dcaState)
				prev := st.prevBarMS
				st.prevBarMS = s.Env.TimeStart // maintained on every bar, warmup included
				if s.IsWarmUp {
					return
				}
				ms := s.Env.TimeStart
				if !btime.IsFirstBarOfPeriodUTC(prev, ms, "month") {
					return
				}
				if !btime.InMonthRangeUTC(ms, startMonth, endMonth) {
					s.CloseOrders(&strat.ExitReq{Tag: "season_end", Dirt: core.OdDirtLong})
					return
				}
				if s.Position(core.OdDirtLong, "") >= maxUnits {
					return
				}
				s.OpenOrder(&strat.EnterReq{Tag: "monthly_dca", CostRate: 1})
			},
		}),
		EachMaxLong:  0, // unlimited same-direction orders: every installment is its own order
		EachMaxShort: -1,
	}
}
