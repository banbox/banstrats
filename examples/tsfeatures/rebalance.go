package tsfeatures

import (
	"cmp"
	"math"
	"slices"

	"github.com/banbox/banbot/btime"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

// rebState is the per-symbol state read by OnBatchJobs.
type rebState struct {
	prevBarMS int64
	score     float64 // risk-adjusted momentum, NaN until ready
	newWeek   bool    // this bar is the first bar of a UTC week
}

// BatchRebalance: weekly long/short momentum basket. Every job scores itself in OnData; OnBatchJobs
// (all symbols of the same bar) ranks them and hands target weights to RebalanceBatchWeights, which
// closes/reduces first and then opens the largest deltas first. Jobs arrive in a neutral
// strat.BarPriority order, so ties never favour alphabetically early symbols.
func BatchRebalance(pol *config.RunPolicyConfig) *strat.TradeStrat {
	lookback := pol.DefInt("lookback", 30, core.PNorm(10, 90))
	basket := pol.DefInt("basket", 2, core.PNorm(1, 5)) // symbols per side
	gross := pol.Def("gross", 1.0, core.PNorm(0.4, 2))  // total |weight| target
	drift := pol.Def("drift", 0.03, core.PNorm(0.01, 0.1))
	stopRate := pol.Def("stopRate", 0.15, core.PNorm(0.05, 0.4)) // per-symbol protective stop

	return &strat.TradeStrat{
		WarmupNum:    lookback * 2,
		BatchInOut:   true,
		EachMaxLong:  0, // 0 = unlimited: rebalancing may scale into an existing position
		EachMaxShort: 0,
		OnStartUp: func(s *strat.StratJob) {
			s.More = &rebState{score: math.NaN()}
		},
		OnData: strat.RouteData(strat.DataHandlers{
			Main: func(s *strat.StratJob, _ strat.DataEvent) {
				e := s.Env
				st := s.More.(*rebState)
				st.newWeek = btime.IsFirstBarOfPeriodUTC(st.prevBarMS, e.TimeStart, "week")
				st.prevBarMS = e.TimeStart
				vol := ta.StdDev(ta.ROC(e.Close, 1), lookback).Get(0)
				roc := ta.ROC(e.Close, lookback).Get(0)
				st.score = roc / vol
				if math.IsNaN(vol) || vol == 0 {
					st.score = math.NaN()
				}
			},
		}),
		OnBatchJobs: func(jobs []*strat.StratJob) {
			if len(jobs) == 0 || jobs[0].IsWarmUp {
				return
			}
			ranked := make([]*strat.StratJob, 0, len(jobs))
			rebalance := false
			for _, j := range jobs {
				st := j.More.(*rebState)
				rebalance = rebalance || st.newWeek
				if !math.IsNaN(st.score) {
					ranked = append(ranked, j)
				}
			}
			if !rebalance || len(ranked) < 2*basket {
				return
			}
			barMS := jobs[0].Env.TimeStart
			slices.SortStableFunc(ranked, func(a, b *strat.StratJob) int {
				sa, sb := a.More.(*rebState).score, b.More.(*rebState).score
				if c := cmp.Compare(sb, sa); c != 0 {
					return c // strongest first
				}
				return cmp.Compare(strat.BarPriority(barMS, a.Symbol.Symbol, 0), strat.BarPriority(barMS, b.Symbol.Symbol, 0))
			})
			weights := make(map[string]float64, 2*basket)
			w := gross / float64(2*basket)
			for i := 0; i < basket; i++ {
				weights[ranked[i].Symbol.Symbol] = w                // long the strongest
				weights[ranked[len(ranked)-1-i].Symbol.Symbol] = -w // short the weakest
			}
			// Symbols missing from weights are flattened. The result carries counts and per-order actions.
			strat.RebalanceBatchWeights(jobs, weights, strat.BatchRebalanceOpts{
				Tag: "mom_rebalance", DriftThreshold: drift, MaxGrossExposure: gross, StopLossRate: stopRate,
			})
		},
	}
}
