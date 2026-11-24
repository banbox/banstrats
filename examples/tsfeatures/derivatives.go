package tsfeatures

import (
	"math"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

// derivState keeps the latest derivatives readings of one symbol. NaN means "not seen yet" or NULL.
type derivState struct {
	funding  float64 // latest settled funding rate (0.0001 = +0.01%)
	oiChange float64 // open interest change over the lookback window
	premium  float64 // close of the perpetual premium (basis) kline
}

// DerivFilter: Donchian breakouts on a linear perpetual, filtered by the BUILT-IN derivatives
// sources. Nothing is registered here: funding_rate, open_interest and premium_kline are
// subscribed by name and arrive as DataRoleCustom events. Download them first with
// `bot series down -tables funding_rate,open_interest,premium_kline ...`.
func DerivFilter(pol *config.RunPolicyConfig) *strat.TradeStrat {
	period := pol.DefInt("period", 20, core.PNorm(10, 60))
	atrLen := pol.DefInt("atrLen", 14, core.PNorm(7, 30))
	slMul := pol.Def("slMul", 2, core.PNorm(1, 4))
	maxFunding := pol.Def("maxFunding", 0.0003, core.PNorm(0.0001, 0.001)) // avoid crowded side
	maxPremium := pol.Def("maxPremium", 0.002, core.PNorm(0.0005, 0.01))
	oiLookback := pol.DefInt("oiLookback", 24, core.PNorm(6, 72)) // in open_interest bars (1h)
	minOIChg := pol.Def("minOIChg", 0.0, core.PNorm(-0.05, 0.1))

	return &strat.TradeStrat{
		WarmupNum:    period * 3,
		EachMaxLong:  1,
		EachMaxShort: 1,
		OnStartUp: func(s *strat.StratJob) {
			s.More = &derivState{funding: math.NaN(), oiChange: math.NaN(), premium: math.NaN()}
		},
		OnDataSubs: func(s *strat.StratJob) []*strat.DataSub {
			// ExSymbol nil = current symbol; each source brings its own default timeframe set.
			return []*strat.DataSub{
				{Source: orm.SeriesSourceFundingRate, TimeFrame: "8h", WarmupNum: 3},
				{Source: orm.SeriesSourceOpenInterest, TimeFrame: "1h", WarmupNum: oiLookback + 1,
					SeriesFields: []string{orm.FieldOpenInterest}},
				{Source: orm.SeriesSourcePremiumKline, TimeFrame: "1h", WarmupNum: 3},
			}
		},
		OnData: strat.RouteData(strat.DataHandlers{
			Custom: func(s *strat.StratJob, d strat.DataEvent) {
				st := s.More.(*derivState)
				switch d.Source {
				case orm.SeriesSourceFundingRate:
					st.funding = d.Float64("rate")
				case orm.SeriesSourcePremiumKline:
					st.premium = d.Float64("close")
				case orm.SeriesSourceOpenInterest:
					// Missing metrics are explicit NULLs: Series() yields NaN, which blocks entries below.
					if oi := d.Series(orm.FieldOpenInterest); oi != nil {
						st.oiChange = oi.Get(0)/oi.Get(oiLookback) - 1
					}
				}
			},
			Main: func(s *strat.StratJob, _ strat.DataEvent) {
				e := s.Env
				hi := ta.Highest(e.High, period).Back(1).Get(0)
				lo := ta.Lowest(e.Low, period).Back(1).Get(0)
				atr := ta.ATR(e.High, e.Low, e.Close, atrLen).Get(0)
				st := s.More.(*derivState)
				if math.IsNaN(hi) || math.IsNaN(lo) || math.IsNaN(atr) || atr <= 0 ||
					s.GetOrderNum(0) > 0 || len(s.Entrys) > 0 {
					return
				}
				// Every reading must be present and the open interest must confirm the breakout.
				if math.IsNaN(st.funding+st.premium+st.oiChange) || st.oiChange < minOIChg {
					return
				}
				c := e.Close.Get(0)
				if c > hi && st.funding <= maxFunding && st.premium <= maxPremium {
					s.OpenOrder(&strat.EnterReq{Tag: "brk_long", StopLossVal: atr * slMul})
				} else if c < lo && st.funding >= -maxFunding && st.premium >= -maxPremium {
					s.OpenOrder(&strat.EnterReq{Tag: "brk_short", Short: true, StopLossVal: atr * slMul})
				}
			},
		}),
	}
}
