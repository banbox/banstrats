package fundingrate

import (
	"testing"

	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
)

func TestFundingRateExampleStrategySubscribesSource(t *testing.T) {
	st := NewExampleStrategy()
	job := &strat.StratJob{
		Symbol: &orm.ExSymbol{ID: 2, Symbol: "ETH/USDT:USDT"},
	}
	subs := st.OnDataSubs(job)
	if len(subs) != 1 {
		t.Fatalf("expected one sub, got %d", len(subs))
	}
	sub := subs[0]
	if sub.Source != SourceName || sub.ExSymbol != job.Symbol || sub.TimeFrame != DefaultTimeframe || sub.WarmupNum != 30 {
		t.Fatalf("unexpected sub: %+v", sub)
	}
}
