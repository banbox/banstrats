package grid

import (
	"testing"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
)

func TestInvGridWaitsForPositiveUnit(t *testing.T) {
	stgy := InvGrid(&config.RunPolicyConfig{})
	job := &strat.StratJob{
		More:      &GridV1{Grid: NewGrid(1, 5, 6, false)},
		Symbol:    &orm.ExSymbol{ID: 1},
		TimeFrame: "1m",
	}

	data := job.SetData(&orm.DataSeries{Source: orm.SeriesSourceKline, Sid: 1, TimeFrame: "1m", TimeMS: 1})
	stgy.OnData(job, strat.DataEvent{DataFields: data, Role: strat.DataRoleMain, Symbol: job.Symbol})

	if len(job.Entrys) != 0 {
		t.Fatalf("grid opened before its unit was initialized: %+v", job.Entrys)
	}
}

func TestInvGridIgnoresNonMainDataWithMatchingSidAndTimeFrame(t *testing.T) {
	stgy := InvGrid(&config.RunPolicyConfig{})
	job := &strat.StratJob{
		More:      &GridV1{Grid: NewGrid(1, 5, 6, false), bigER: -1},
		Symbol:    &orm.ExSymbol{ID: 1},
		TimeFrame: "1m",
	}
	job.More.(*GridV1).Unit = 1

	data := job.SetData(&orm.DataSeries{Source: "macro", Sid: 1, TimeFrame: "1m", TimeMS: 1})
	stgy.OnData(job, strat.DataEvent{DataFields: data, Role: strat.DataRoleCustom, Symbol: job.Symbol})

	if len(job.Entrys) != 0 {
		t.Fatalf("custom data triggered primary grid logic: %+v", job.Entrys)
	}
}
