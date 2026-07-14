package ma

import (
	"testing"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

func TestDemoInfoPersistsHigherTimeframeDirection(t *testing.T) {
	stgy := DemoInfo(&config.RunPolicyConfig{})
	job := &strat.StratJob{}
	stgy.OnStartUp(job)

	feedTrend(t, stgy, job, 100, 1)
	if got := job.More.(*Demo2Sta).bigDirt; got != 1 {
		t.Fatalf("bullish higher-timeframe direction = %d, want 1", got)
	}

	job = &strat.StratJob{}
	stgy.OnStartUp(job)
	feedTrend(t, stgy, job, 200, -1)
	if got := job.More.(*Demo2Sta).bigDirt; got != -1 {
		t.Fatalf("bearish higher-timeframe direction = %d, want -1", got)
	}
}

func TestDemoInfoOpensAndClosesWhenTimeframesAgree(t *testing.T) {
	oldStake, oldOpenVolRate := config.StakeAmount, config.OpenVolRate
	config.StakeAmount = 100
	config.OpenVolRate = 1
	t.Cleanup(func() {
		config.StakeAmount = oldStake
		config.OpenVolRate = oldOpenVolRate
	})

	stgy := DemoInfo(&config.RunPolicyConfig{Params: map[string]float64{"smlLen": 3, "bigLen": 10}})
	small, err := ta.NewBarEnv("binance", "linear", "BTC/USDT:USDT", "15m")
	if err != nil {
		t.Fatalf("create primary bar environment: %v", err)
	}
	big, err := ta.NewBarEnv("binance", "linear", "BTC/USDT:USDT", "1h")
	if err != nil {
		t.Fatalf("create info bar environment: %v", err)
	}
	job := &strat.StratJob{
		Strat: stgy, Env: small, TimeFrame: "15m", Account: config.DefAcc,
		Symbol:    &orm.ExSymbol{ID: 1, Exchange: "binance", Market: "linear", Symbol: "BTC/USDT:USDT"},
		CloseLong: true, CloseShort: true,
	}
	stgy.OnStartUp(job)

	const startMS = int64(1_704_067_200_000)
	for i := 0; i < 30; i++ {
		price := 100 + float64(i)
		big.OnBar(startMS+int64(i)*3_600_000, price, price, price, price, 1, 0, 0, 0)
		stgy.OnInfoBar(job, big, job.Symbol.Symbol, "1h")
	}
	for i := 0; i < 30; i++ {
		feedDemoInfoBar(stgy, job, startMS+int64(i)*900_000, 100-float64(i))
	}
	for i := 0; i < 45; i++ {
		feedDemoInfoBar(stgy, job, startMS+int64(i+30)*900_000, 70+float64(i))
	}
	if len(job.Entrys) != 1 || job.Entrys[0].Tag != "open" {
		t.Fatalf("expected one long entry after aligned crossover, got %+v", job.Entrys)
	}

	job.LongOrders = []*ormo.InOutOrder{{IOrder: &ormo.IOrder{Short: false}}}
	for i := 0; i < 45; i++ {
		feedDemoInfoBar(stgy, job, startMS+int64(i+75)*900_000, 115-float64(i))
	}
	if len(job.Exits) != 1 || job.Exits[0].Tag != "exit" {
		t.Fatalf("expected a close on downward crossover, got %+v", job.Exits)
	}
}

func feedDemoInfoBar(stgy *strat.TradeStrat, job *strat.StratJob, timeMS int64, price float64) {
	job.Env.OnBar(timeMS, price, price, price, price, 1_000, 0, 0, 0)
	stgy.OnBar(job)
}

func feedTrend(t *testing.T, stgy *strat.TradeStrat, job *strat.StratJob, start, step float64) {
	t.Helper()
	env, err := ta.NewBarEnv("binance", "linear", "BTC/USDT:USDT", "1h")
	if err != nil {
		t.Fatalf("create bar environment: %v", err)
	}
	const startMS = int64(1_704_067_200_000)
	for i := 0; i < 60; i++ {
		price := start + float64(i)*step
		env.OnBar(startMS+int64(i)*3_600_000, price, price, price, price, 1, 0, 0, 0)
		stgy.OnInfoBar(job, env, "BTC/USDT:USDT", "1h")
	}
}
