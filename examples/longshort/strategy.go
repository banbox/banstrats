package longshort

import (
	"fmt"
	"math"
	"strings"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banbot/utils"
)

const ExampleStrategyName = "binance_ls_example"

type StrategyState struct {
	SeenEvents          int
	IgnoredEvents       int
	LastError           string
	LastSource          string
	LastTimeFrame       string
	LastSid             int32
	LastLongAccount     float64
	LastShortAccount    float64
	LastRatio           float64
	LatestLongAccount   float64
	LatestShortAccount  float64
	LatestRatio         float64
	WindowCount         int
	WindowLongAccounts  []float64
	WindowShortAccounts []float64
	WindowWarmups       []bool
	LastEventWarmUp     bool
	LastJobWarmUp       bool
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
		OnData: func(s *strat.StratJob, fields strat.DataEvent) {
			state := EnsureStrategyState(s)
			if fields.DataFields == nil {
				state.LastError = "data fields are nil"
				return
			}
			if orm.NormalizeSeriesSource(fields.Source) != SourceName {
				state.IgnoredEvents++
				return
			}
			if fields.TimeFrame != DefaultTimeframe {
				state.LastError = fmt.Sprintf("unexpected timeframe for source=%s sid=%d tf=%s", fields.Source, fields.Sid, fields.TimeFrame)
				return
			}
			longAccount, err := dataFieldFloat(fields.DataFields, FieldLongAccount)
			if err != nil {
				state.LastError = fmt.Sprintf("invalid data field %s for source=%s sid=%d tf=%s: %v", FieldLongAccount, fields.Source, fields.Sid, fields.TimeFrame, err)
				return
			}
			shortAccount, err := dataFieldFloat(fields.DataFields, FieldShortAccount)
			if err != nil {
				state.LastError = fmt.Sprintf("invalid data field %s for source=%s sid=%d tf=%s: %v", FieldShortAccount, fields.Source, fields.Sid, fields.TimeFrame, err)
				return
			}
			ratio, err := dataFieldFloat(fields.DataFields, FieldRatio)
			if err != nil {
				state.LastError = fmt.Sprintf("invalid data field longShortRatio for source=%s sid=%d tf=%s: %v", fields.Source, fields.Sid, fields.TimeFrame, err)
				return
			}
			state.SeenEvents++
			state.LastError = ""
			state.LastSource = fields.Source
			state.LastTimeFrame = fields.TimeFrame
			state.LastSid = fields.Sid
			state.LastLongAccount = longAccount
			state.LastShortAccount = shortAccount
			state.LastRatio = ratio
			state.LatestLongAccount = longAccount
			state.LatestShortAccount = shortAccount
			state.LatestRatio = ratio
			state.WindowLongAccounts = collectSeriesFloat(fields.Series(FieldLongAccount))
			state.WindowShortAccounts = collectSeriesFloat(fields.Series(FieldShortAccount))
			state.WindowCount = len(state.WindowLongAccounts)
			state.WindowWarmups = append(state.WindowWarmups, fields.IsWarmUp)
			if size := state.WindowCount; len(state.WindowWarmups) > size {
				state.WindowWarmups = state.WindowWarmups[len(state.WindowWarmups)-size:]
			}
			state.LastEventWarmUp = fields.IsWarmUp
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

func dataFieldFloat(fields *strat.DataFields, field string) (float64, error) {
	if fields == nil || fields.Raw(field) == nil {
		return 0, fmt.Errorf("field is missing")
	}
	value, err := utils.ToFloat64(fields.Raw(field))
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("field is not numeric")
	}
	return value, nil
}

func collectSeriesFloat(series interface {
	Len() int
	Get(int) float64
}) []float64 {
	if series == nil {
		return nil
	}
	values := make([]float64, 0, series.Len())
	for idx := series.Len() - 1; idx >= 0; idx-- {
		values = append(values, series.Get(idx))
	}
	return values
}

func StrategyKey() string {
	return strings.Join([]string{"longshort", ExampleStrategyName}, ":")
}
