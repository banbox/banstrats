package longshort

import (
	"fmt"
	"strings"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
)

const ExampleStrategyName = "binance_ls_example"

type StrategyState struct {
	SeenEvents       int
	IgnoredEvents    int
	LastError        string
	LastSource       string
	LastTimeFrame    string
	LastSid          int32
	LastLongQty      float64
	LastShortQty     float64
	LastRatio        float64
	LatestLongQty    float64
	LatestShortQty   float64
	LatestRatio      float64
	WindowCount      int
	WindowLongQtys   []float64
	WindowShortQtys  []float64
	WindowWarmups    []bool
	LastEventWarmUp  bool
	LastJobWarmUp    bool
}

func init() {
	strat.AddStratGroup("longshort", map[string]strat.FuncMakeStrat{
		ExampleStrategyName: ExampleStrategy,
	})
}

func ExampleStrategy(pol *config.RunPolicyConfig) *strat.TradeStrat {
	return &strat.TradeStrat{
		WarmupNum: 2,
		OnDataSubs: func(s *strat.StratJob) []*strat.DataSub {
			return []*strat.DataSub{{
				Source:    SourceName,
				ExSymbol:  s.Symbol,
				TimeFrame: DefaultTimeframe,
				WarmupNum: 2,
			}}
		},
		OnData: func(s *strat.StratJob, evt *orm.DataSeries) {
			state := EnsureStrategyState(s)
			if evt == nil {
				state.LastError = "source event is nil"
				return
			}
			if orm.NormalizeSeriesSource(evt.Source) != SourceName {
				state.IgnoredEvents++
				return
			}
			if evt.TimeFrame != DefaultTimeframe {
				state.LastError = fmt.Sprintf("unexpected timeframe for source=%s sid=%d tf=%s", evt.Source, evt.Sid, evt.TimeFrame)
				return
			}
			if s.DataHub == nil {
				state.LastError = fmt.Sprintf("missing DataHub for source=%s sid=%d tf=%s", evt.Source, evt.Sid, evt.TimeFrame)
				return
			}
			longQty, err := evt.FloatValue("longQty")
			if err != nil {
				state.LastError = fmt.Sprintf("invalid event field longQty for source=%s sid=%d tf=%s: %v", evt.Source, evt.Sid, evt.TimeFrame, err)
				return
			}
			shortQty, err := evt.FloatValue("shortQty")
			if err != nil {
				state.LastError = fmt.Sprintf("invalid event field shortQty for source=%s sid=%d tf=%s: %v", evt.Source, evt.Sid, evt.TimeFrame, err)
				return
			}
			ratio, err := evt.FloatValue("longShortRatio")
			if err != nil {
				state.LastError = fmt.Sprintf("invalid event field longShortRatio for source=%s sid=%d tf=%s: %v", evt.Source, evt.Sid, evt.TimeFrame, err)
				return
			}
			latest := s.DataHub.Latest(evt.Source, evt.Sid, evt.TimeFrame)
			if latest == nil {
				state.LastError = fmt.Sprintf("missing DataHub latest for source=%s sid=%d tf=%s", evt.Source, evt.Sid, evt.TimeFrame)
				return
			}
			latestLongQty, err := latest.FloatValue("longQty")
			if err != nil {
				state.LastError = fmt.Sprintf("invalid latest field longQty for source=%s sid=%d tf=%s: %v", evt.Source, evt.Sid, evt.TimeFrame, err)
				return
			}
			latestShortQty, err := latest.FloatValue("shortQty")
			if err != nil {
				state.LastError = fmt.Sprintf("invalid latest field shortQty for source=%s sid=%d tf=%s: %v", evt.Source, evt.Sid, evt.TimeFrame, err)
				return
			}
			latestRatio, err := latest.FloatValue("longShortRatio")
			if err != nil {
				state.LastError = fmt.Sprintf("invalid latest field longShortRatio for source=%s sid=%d tf=%s: %v", evt.Source, evt.Sid, evt.TimeFrame, err)
				return
			}
			window := s.DataHub.Window(evt.Source, evt.Sid, evt.TimeFrame, 0)
			state.SeenEvents++
			state.LastError = ""
			state.LastSource = evt.Source
			state.LastTimeFrame = evt.TimeFrame
			state.LastSid = evt.Sid
			state.LastLongQty = longQty
			state.LastShortQty = shortQty
			state.LastRatio = ratio
			state.LatestLongQty = latestLongQty
			state.LatestShortQty = latestShortQty
			state.LatestRatio = latestRatio
			state.WindowCount = len(window)
			state.WindowLongQtys = collectWindowFloat(window, "longQty")
			state.WindowShortQtys = collectWindowFloat(window, "shortQty")
			state.WindowWarmups = collectWindowWarmups(window)
			state.LastEventWarmUp = evt.IsWarmUp
			state.LastJobWarmUp = s.IsWarmUp
		},
	}
}

func EnsureStrategyState(job *strat.StratJob) *StrategyState {
	if job == nil {
		return &StrategyState{LastError: "strategy job is nil"}
	}
	if state, ok := job.More.(*StrategyState); ok && state != nil {
		return state
	}
	state := &StrategyState{}
	job.More = state
	return state
}

func collectWindowFloat(window []*orm.DataSeries, field string) []float64 {
	vals := make([]float64, 0, len(window))
	for _, item := range window {
		if item == nil {
			continue
		}
		val, err := item.FloatValue(field)
		if err != nil {
			continue
		}
		vals = append(vals, val)
	}
	return vals
}

func collectWindowWarmups(window []*orm.DataSeries) []bool {
	vals := make([]bool, 0, len(window))
	for _, item := range window {
		if item == nil {
			continue
		}
		vals = append(vals, item.IsWarmUp)
	}
	return vals
}

func StrategyKey() string {
	return strings.Join([]string{"longshort", ExampleStrategyName}, ":")
}
