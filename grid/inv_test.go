package grid

import (
	"math"
	"testing"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
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

// Synthetic bars exercise the fixed 15m information subscription without
// requiring exchange history or an external database.
func TestInvGridSyntheticInformationWarmup(t *testing.T) {
	stgy := InvGrid(&config.RunPolicyConfig{})
	env, err := ta.NewBarEnv("binance", "linear", "BTC/USDT:USDT", "5m")
	if err != nil {
		t.Fatal(err)
	}
	job := &strat.StratJob{
		Strat: stgy, Env: env, TimeFrame: "5m", IsWarmUp: true,
		Symbol: &orm.ExSymbol{ID: 1, Exchange: "binance", Market: "linear", Symbol: "BTC/USDT:USDT"},
	}
	stgy.OnStartUp(job)
	subs := strat.CollectDataSubs(job)
	if len(subs) != 1 || subs[0].TimeFrame != "15m" {
		t.Fatalf("grid information subscription = %+v", subs)
	}
	for i := 0; i < 160; i++ {
		price := 100 + math.Sin(float64(i)*math.Pi/5)
		at := int64(1_704_067_200_000) + int64(i)*900_000
		fields := job.SetData(&orm.DataSeries{
			Source: orm.SeriesSourceKline, Sid: 1, TimeFrame: "15m", TimeMS: at, EndMS: at + 900_000,
			Closed: true, IsWarmUp: true,
			Values: map[string]any{"open": price, "high": price + 1, "low": price - 1, "close": price, "volume": 100.0},
		})
		stgy.OnData(job, strat.DataEvent{DataFields: fields, Role: strat.DataRoleInfo, Symbol: job.Symbol})
	}
	state := job.More.(*GridV1)
	if state.Unit <= 0 || math.IsNaN(state.Unit) || math.IsInf(state.Unit, 0) {
		t.Fatalf("information warmup did not initialize grid unit: %v", state.Unit)
	}
	if math.IsNaN(state.bigER) || state.bigER >= 0.3 {
		t.Fatalf("periodic information bars did not initialize grid efficiency: %v", state.bigER)
	}
	if len(job.Entrys) != 0 {
		t.Fatal("information warmup created an order")
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
