package ma

import (
	"testing"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg"
	"github.com/banbox/banexg/errs"
	ta "github.com/banbox/banta"
)

func TestWebsocketAndPostAPICallbacksQueueRuntimeOrders(t *testing.T) {
	oldStake, oldRate := config.StakeAmount, config.OpenVolRate
	config.StakeAmount, config.OpenVolRate = 100, 1
	t.Cleanup(func() { config.StakeAmount, config.OpenVolRate = oldStake, oldRate })
	env, err := ta.NewBarEnv("binance", "linear", "BTC/USDT:USDT", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.OnBar(1_704_067_200_000, 100, 101, 99, 100, 100, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	stgy := ws(&config.RunPolicyConfig{})
	job := &strat.StratJob{Strat: stgy, Env: env, TimeFrame: "1h", Account: config.DefAcc,
		Symbol:    &orm.ExSymbol{ID: 1, Exchange: "binance", Market: "linear", Symbol: "BTC/USDT:USDT"},
		CloseLong: true, CloseShort: true,
	}
	state := strat.NewState()
	var processed int
	if err := state.BindOrderProcessor(state, config.DefAcc, func(received *strat.StratJob) ([]*ormo.InOutOrder, []*ormo.InOutOrder, *errs.Error) {
		if received != job || received.PendingEntryCount() != 1 {
			t.Fatal("websocket order was not routed to its runtime")
		}
		processed++
		received.DropEntryRequests()
		return nil, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	job.BindRuntimeState(state, nil, nil)
	stgy.OnWsTrades(job, job.Symbol.Symbol, []*banexg.Trade{{Timestamp: env.TimeStart, Price: 100, Amount: 1}})
	stgy.OnWsDepth(job, &banexg.OrderBook{Symbol: job.Symbol.Symbol,
		Bids: &banexg.OdBookSide{Price: []float64{99}, Size: []float64{1}},
		Asks: &banexg.OdBookSide{Price: []float64{101}, Size: []float64{1}},
	})
	stgy.OnWsKline(job, job.Symbol.Symbol, &banexg.Kline{Time: env.TimeStart, Close: 100})
	if processed != 1 || job.PendingEntryCount() != 0 {
		t.Fatal("websocket order was not processed exactly once")
	}
	job.Strat = PostApi(&config.RunPolicyConfig{})
	if err := job.Strat.OnPostApi(&core.ApiClient{}, map[string]interface{}{"action": "openLong"},
		map[string]map[string]*strat.StratJob{config.DefAcc: {"BTC_1h": job}}); err != nil {
		t.Fatal(err)
	}
	if job.PendingEntryCount() != 1 {
		t.Fatal("POST API callback did not queue its order")
	}
}
