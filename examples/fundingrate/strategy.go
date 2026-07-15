package fundingrate

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/strat"
)

const ExampleStrategyName = "funding_rate_demo"

func init() {
	strat.AddStratGroup("fundingrate", map[string]strat.FuncMakeStrat{
		ExampleStrategyName: func(pol *config.RunPolicyConfig) *strat.TradeStrat {
			return NewExampleStrategy()
		},
	})
}

func NewExampleStrategy() *strat.TradeStrat {
	return &strat.TradeStrat{
		Name: ExampleStrategyName,
		OnDataSubs: func(job *strat.StratJob) []*strat.DataSub {
			return []*strat.DataSub{{
				Source:    SourceName,
				ExSymbol:  job.Symbol,
				TimeFrame: DefaultTimeframe,
				WarmupNum: 30,
			}}
		},
		OnData: func(job *strat.StratJob, fields strat.DataEvent) {
			if fields.DataFields == nil || fields.Source != SourceName {
				return
			}
			rate := fields.Float64("rate")
			if rate == 0 {
				return
			}
			_ = rate
		},
	}
}
