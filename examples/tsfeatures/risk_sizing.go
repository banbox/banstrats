package tsfeatures

import (
	"math"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

// riskState is the per-symbol (per StratJob) state. The TradeStrat itself is shared by all symbols.
type riskState struct {
	paused bool // new entries are blocked while the account drawdown is high
}

// RiskSizing: EMA cross entries sized by RiskRate (a fixed share of account equity is
// risked between entry and the ATR stop), guarded by the account equity/drawdown APIs.
func RiskSizing(pol *config.RunPolicyConfig) *strat.TradeStrat {
	fast := pol.DefInt("fast", 10, core.PNorm(5, 30))
	slow := pol.DefInt("slow", 30, core.PNorm(20, 100))
	pol.Require(fast < slow, "require fast < slow")
	atrLen := pol.DefInt("atrLen", 14, core.PNorm(7, 30))
	slMul := pol.Def("slMul", 2, core.PNorm(1, 4))
	riskRate := pol.Def("riskRate", 0.01, core.PNorm(0.002, 0.02)) // 1% of equity per trade
	ddPause := pol.Def("ddPause", 0.10, core.PNorm(0.05, 0.3))     // pause entries at 10% drawdown
	ddResume := pol.Def("ddResume", 0.05, core.PNorm(0.01, 0.2))   // resume below 5% drawdown
	minFree := pol.Def("minFree", 0.2, core.PNorm(0.05, 0.5))      // keep 20% of equity unused
	pol.Require(ddResume < ddPause, "require ddResume < ddPause")

	return &strat.TradeStrat{
		WarmupNum:    slow * 3,
		EachMaxLong:  1,
		EachMaxShort: 1,
		OnStartUp: func(s *strat.StratJob) {
			s.More = &riskState{}
		},
		OnData: strat.RouteData(strat.DataHandlers{
			Main: func(s *strat.StratJob, _ strat.DataEvent) {
				e := s.Env
				// Indicators run on every bar, before any early return.
				cross := ta.EMA(e.Close, fast).Cross(ta.EMA(e.Close, slow))
				atr := ta.ATR(e.High, e.Low, e.Close, atrLen).Get(0)
				st := s.More.(*riskState)

				// Drawdown guard with hysteresis. AccountDrawdown is relative to the
				// account's high-water mark and is shared by all symbols.
				dd := s.AccountDrawdown()
				if dd >= ddPause && !st.paused {
					st.paused = true
					s.CloseOrders(&strat.ExitReq{Tag: "dd_guard"})
				} else if dd <= ddResume {
					st.paused = false
				}
				if cross == 0 || math.IsNaN(atr) || atr <= 0 {
					return
				}
				if cross == 1 {
					s.CloseOrders(&strat.ExitReq{Tag: "cross_exit_short", Dirt: core.OdDirtShort})
				} else if cross == -1 {
					s.CloseOrders(&strat.ExitReq{Tag: "cross_exit_long", Dirt: core.OdDirtLong})
				}
				if st.paused || s.AvailableBalance() < s.AccountEquity()*minFree {
					return
				}
				// RiskRate: LegalCost = equity * riskRate / (stop distance / entry price).
				// It needs a stop (StopLossVal here) and is capped by leverage / max_stake_amt.
				if cross == 1 {
					s.OpenOrder(&strat.EnterReq{Tag: "ema_long", StopLossVal: atr * slMul, RiskRate: riskRate})
				} else if cross == -1 {
					s.OpenOrder(&strat.EnterReq{Tag: "ema_short", Short: true, StopLossVal: atr * slMul, RiskRate: riskRate})
				}
			},
		}),
	}
}
