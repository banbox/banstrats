package tsfeatures

import (
	"math"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

// ocoGroup ties the long and short breakout entries together: when one fills,
// the framework cancels the other still-pending entry of the same group.
const ocoGroup = "donchian"

// Bracket: arms a stop-limit breakout entry on both sides of the Donchian channel.
// Each entry carries a protective stop, a two-rung take-profit ladder and a trailing
// stop for the remaining position.
func Bracket(pol *config.RunPolicyConfig) *strat.TradeStrat {
	period := pol.DefInt("period", 20, core.PNorm(10, 60))
	atrLen := pol.DefInt("atrLen", 14, core.PNorm(7, 30))
	slMul := pol.Def("slMul", 2, core.PNorm(1, 4))
	tp1Mul := pol.Def("tp1Mul", 1.5, core.PNorm(0.5, 3))
	tp2Mul := pol.Def("tp2Mul", 3, core.PNorm(1.5, 6))
	pol.Require(tp1Mul < tp2Mul, "require tp1Mul < tp2Mul")
	callback := pol.Def("callback", 1.5, core.PNorm(0.5, 5)) // trailing callback in percent (1.5 = 1.5%)
	expireBars := pol.DefInt("expireBars", 2, core.PNorm(1, 6))

	return &strat.TradeStrat{
		WarmupNum:    period * 3,
		EachMaxLong:  1,
		EachMaxShort: 1,
		OnData: strat.RouteData(strat.DataHandlers{
			Main: func(s *strat.StratJob, _ strat.DataEvent) {
				e := s.Env
				hi := ta.Highest(e.High, period).Get(0)
				lo := ta.Lowest(e.Low, period).Get(0)
				atr := ta.ATR(e.High, e.Low, e.Close, atrLen).Get(0)
				if math.IsNaN(hi) || math.IsNaN(lo) || math.IsNaN(atr) || atr <= 0 {
					return
				}
				// Re-arm only when flat and nothing is pending (unfilled entries expire after StopBars).
				if s.GetOrderNum(0) > 0 || len(s.Entrys) > 0 {
					return
				}
				// Stop-limit: once price reaches Stop, a limit order rests at Limit (a small slippage cap).
				// Prices are only known in advance, so tp/trailing levels are expressed from the Stop.
				longStop, shortStop := hi, lo
				s.OpenOrder(&strat.EnterReq{
					Tag: "brk_long", Stop: longStop, Limit: longStop + 0.1*atr,
					OCOGroup: ocoGroup, StopBars: expireBars,
					StopLossVal: atr * slMul,
					TakeProfits: []strat.TakeProfitTarget{
						{Val: atr * tp1Mul, ExitRate: 0.4, MoveSLToBreakeven: true, Tag: "tp1"},
						{Val: atr * tp2Mul, ExitRate: 0.3, Tag: "tp2"},
					},
					ActivationPrice: longStop + atr*tp1Mul, CallbackPct: callback, // trails the remaining 30%
				})
				s.OpenOrder(&strat.EnterReq{
					Tag: "brk_short", Short: true, Stop: shortStop, Limit: shortStop - 0.1*atr,
					OCOGroup: ocoGroup, StopBars: expireBars,
					StopLossVal: atr * slMul,
					TakeProfits: []strat.TakeProfitTarget{
						{Val: atr * tp1Mul, ExitRate: 0.4, MoveSLToBreakeven: true, Tag: "tp1"},
						{Val: atr * tp2Mul, ExitRate: 0.3, Tag: "tp2"},
					},
					ActivationPrice: shortStop - atr*tp1Mul, CallbackPct: callback,
				})
			},
		}),
	}
}
