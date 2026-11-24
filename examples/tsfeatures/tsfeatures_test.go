package tsfeatures

import (
	"math"
	"testing"
	"time"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

var allNames = []string{RiskSizingName, BracketName, OpeningRangeName, CalendarDCAName, DerivFilterName, BatchRebalanceName}

func makeStrat(t *testing.T, name string, params ...map[string]float64) *strat.TradeStrat {
	t.Helper()
	pol := &config.RunPolicyConfig{}
	if len(params) > 0 {
		pol.Params = params[0]
	}
	factory, ok := strat.GetStrategyFactory("tsfeatures:" + name)
	if !ok {
		t.Fatalf("strategy tsfeatures:%s is not registered", name)
	}
	st := factory(pol)
	if st == nil {
		t.Fatalf("factory of %s returned nil", name)
	}
	return st
}

// newJob builds a job without a runtime: OpenOrder/CloseOrders only queue requests on it.
func newJob(t *testing.T, st *strat.TradeStrat, symbol, tf string) *strat.StratJob {
	t.Helper()
	env, err := ta.NewBarEnv("binance", "linear", symbol, tf)
	if err != nil {
		t.Fatal(err)
	}
	job := &strat.StratJob{Strat: st, Env: env, TimeFrame: tf, Account: config.DefAcc,
		DataHub: strat.NewDataHub(), CloseLong: true, CloseShort: true,
		Symbol: &orm.ExSymbol{ID: 1, Exchange: "binance", Market: "linear", Symbol: symbol}}
	if st.OnStartUp != nil {
		st.OnStartUp(job)
	}
	return job
}

// feed appends one closed bar and runs the main callback, like the framework does.
func feed(t *testing.T, job *strat.StratJob, startMS, tfMS int64, o, h, l, c float64) {
	t.Helper()
	job.DropEntryRequests()
	job.Env.OnBar2(startMS, startMS+tfMS, o, h, l, c, 100, c*100, 50, 10)
	job.Strat.OnData(job, strat.DataEvent{DataFields: &strat.DataFields{}, Role: strat.DataRoleMain, Symbol: job.Symbol})
}

func withStake(t *testing.T) {
	oldStake, oldRate := config.StakeAmount, config.OpenVolRate
	config.StakeAmount, config.OpenVolRate = 100, 1000
	t.Cleanup(func() { config.StakeAmount, config.OpenVolRate = oldStake, oldRate })
}

func utc(y int, m time.Month, d, h, min int) int64 {
	return time.Date(y, m, d, h, min, 0, 0, time.UTC).UnixMilli()
}

func TestAllStrategiesRegisteredWithCallbacks(t *testing.T) {
	for _, name := range allNames {
		st := makeStrat(t, name)
		if st.OnData == nil || st.WarmupNum <= 0 {
			t.Errorf("%s: OnData and WarmupNum must be set", name)
		}
		if st.OnBar != nil || st.OnInfoBar != nil {
			t.Errorf("%s: legacy callbacks cannot be combined with OnData", name)
		}
		if name != BracketName && st.OnStartUp == nil {
			t.Errorf("%s: per-symbol state must be created in OnStartUp", name)
		}
	}
	if st := makeStrat(t, BatchRebalanceName); !st.BatchInOut || st.OnBatchJobs == nil {
		t.Error("batch_rebalance needs BatchInOut and OnBatchJobs")
	}
	if st := makeStrat(t, DerivFilterName); st.OnDataSubs == nil {
		t.Error("deriv_filter needs OnDataSubs")
	}
}

func TestDerivFilterSubscribesBuiltinSources(t *testing.T) {
	st := makeStrat(t, DerivFilterName)
	subs := st.OnDataSubs(newJob(t, st, "BTC/USDT:USDT", "1h"))
	want := map[string]bool{orm.SeriesSourceFundingRate: false, orm.SeriesSourceOpenInterest: false, orm.SeriesSourcePremiumKline: false}
	for _, sub := range subs {
		if _, ok := want[sub.Source]; !ok || sub.ExSymbol != nil || sub.WarmupNum <= 0 {
			t.Fatalf("unexpected sub %+v", sub)
		}
		want[sub.Source] = true
	}
	for src, seen := range want {
		if !seen {
			t.Errorf("missing built-in source %s", src)
		}
	}
	// No reading yet: breakouts must stay blocked even on a clear breakout.
	job := newJob(t, st, "BTC/USDT:USDT", "1h")
	ms := utc(2024, 1, 1, 0, 0)
	for i := 0; i < 80; i++ {
		feed(t, job, ms+int64(i)*3_600_000, 3_600_000, 100, 101, 99, 100+float64(i)*0.5)
	}
	if job.PendingEntryCount() != 0 {
		t.Fatal("entry queued without derivatives readings")
	}
	if st := job.More.(*derivState); !math.IsNaN(st.funding) {
		t.Fatal("funding should start as NaN")
	}
}

func TestCalendarDCAFiresOncePerMonthInSeason(t *testing.T) {
	withStake(t)
	st := makeStrat(t, CalendarDCAName)
	job := newJob(t, st, "BTC/USDT:USDT", "1d")
	day := int64(86_400_000)
	start := utc(2024, time.November, 28, 0, 0)
	var entries []string
	for i := 0; i < 8; i++ { // Nov 28 .. Dec 5
		feed(t, job, start+int64(i)*day, day, 100, 101, 99, 100)
		for _, req := range job.Entrys {
			entries = append(entries, time.UnixMilli(start+int64(i)*day).UTC().Format("01-02")+":"+req.Tag)
		}
	}
	if len(entries) != 1 || entries[0] != "12-01:monthly_dca" {
		t.Fatalf("expected a single DCA on 12-01, got %v", entries)
	}
	// Out of season (May): no new installment.
	job = newJob(t, st, "BTC/USDT:USDT", "1d")
	start = utc(2024, time.April, 29, 0, 0)
	for i := 0; i < 5; i++ {
		feed(t, job, start+int64(i)*day, day, 100, 101, 99, 100)
		if job.PendingEntryCount() != 0 {
			t.Fatalf("unexpected entry on day %d out of season", i)
		}
	}
}

func TestOpeningRangeBreakoutAfterRangeIsComplete(t *testing.T) {
	withStake(t)
	st := makeStrat(t, OpeningRangeName)
	job := newJob(t, st, "BTC/USDT:USDT", "5m")
	tf := int64(300_000)
	open := utc(2024, time.March, 5, 13, 30)
	// 13:30-14:00 forms the range 99..101; a close above it inside the range must not trade.
	for i := 0; i < 6; i++ {
		feed(t, job, open+int64(i)*tf, tf, 100, 101, 99, 100)
		if job.PendingEntryCount() != 0 {
			t.Fatal("entry inside the opening range (lookahead)")
		}
	}
	feed(t, job, open+6*tf, tf, 100, 103, 100, 102.5)
	if len(job.Entrys) != 1 || job.Entrys[0].Tag != "orb_long" {
		t.Fatalf("expected orb_long, got %+v", job.Entrys)
	}
	if req := job.Entrys[0]; req.StopLoss != 100 || req.RiskRate <= 0 || req.LegalCost <= 0 || req.TakeProfit != 106.5 {
		t.Fatalf("unexpected bracket %+v", req)
	}
}

func TestBracketArmsOCOStopLimitEntries(t *testing.T) {
	withStake(t)
	st := makeStrat(t, BracketName)
	job := newJob(t, st, "BTC/USDT:USDT", "1h")
	ms := utc(2024, 1, 1, 0, 0)
	for i := 0; i < 80; i++ {
		feed(t, job, ms+int64(i)*3_600_000, 3_600_000, 100, 101+float64(i%3), 99, 100)
	}
	if len(job.Entrys) != 2 {
		t.Fatalf("expected a long and a short entry, got %d", len(job.Entrys))
	}
	for _, req := range job.Entrys {
		if req.OCOGroup == "" || req.OCOGroup != job.Entrys[0].OCOGroup || req.Stop <= 0 || req.Limit <= 0 {
			t.Fatalf("bad stop-limit/OCO fields: %+v", req)
		}
		sum := 0.0
		for _, tp := range req.TakeProfits {
			sum += tp.ExitRate
		}
		if len(req.TakeProfits) != 2 || sum > 1 || req.CallbackPct < 0.05 || req.CallbackPct > 50 {
			t.Fatalf("bad ladder/trailing fields: %+v", req)
		}
	}
	if !job.Entrys[0].Short && job.Entrys[1].Short == job.Entrys[0].Short {
		t.Fatal("OCO pair must contain both directions")
	}
}

func TestRiskSizingQueuesRiskRateEntryOnCross(t *testing.T) {
	withStake(t)
	st := makeStrat(t, RiskSizingName)
	job := newJob(t, st, "BTC/USDT:USDT", "1h")
	ms := utc(2024, 1, 1, 0, 0)
	var got *strat.EnterReq
	for i := 0; i < 200 && got == nil; i++ {
		px := 200 - float64(i) // falling market, then a sharp rebound
		if i >= 100 {
			px = 100 + float64(i-100)*3
		}
		feed(t, job, ms+int64(i)*3_600_000, 3_600_000, px, px+1, px-1, px)
		for _, req := range job.Entrys {
			if req.Tag == "ema_long" {
				got = req
			}
		}
	}
	if got == nil {
		t.Fatal("no ema_long entry on the golden cross")
	}
	// OpenOrder resolved StopLossVal into a stop price and derived LegalCost from the risk budget.
	if got.RiskRate != 0.01 || got.StopLoss <= 0 || got.LegalCost <= 0 {
		t.Fatalf("RiskRate sizing was not resolved: %+v", got)
	}
}

func TestBatchRebalanceLongsStrongestShortsWeakest(t *testing.T) {
	withStake(t)
	st := makeStrat(t, BatchRebalanceName, map[string]float64{"basket": 1}) // one symbol per side
	symbols := []string{"AAA/USDT:USDT", "BBB/USDT:USDT", "CCC/USDT:USDT", "DDD/USDT:USDT"}
	slope := []float64{0.8, 0.2, -0.2, -0.8}
	jobs := make([]*strat.StratJob, len(symbols))
	for i, sym := range symbols {
		jobs[i] = newJob(t, st, sym, "1d")
	}
	day := int64(86_400_000)
	start := utc(2024, time.January, 1, 0, 0) // Monday
	for d := 0; d < 100; d++ {
		for i, job := range jobs {
			job.DropEntryRequests()
			px := 100 + slope[i]*float64(d) + math.Sin(float64(d))
			feed(t, job, start+int64(d)*day, day, px, px+1, px-1, px)
		}
		if d >= 63 && jobs[0].More.(*rebState).newWeek {
			st.OnBatchJobs(jobs)
			break
		}
	}
	dirs := map[string]bool{}
	for _, job := range jobs {
		for _, req := range job.Entrys {
			dirs[job.Symbol.Symbol] = req.Short
		}
	}
	if len(dirs) != 2 {
		t.Fatalf("expected one long and one short, got %v", dirs)
	}
	if short, ok := dirs[symbols[0]]; !ok || short {
		t.Errorf("strongest symbol should be long: %v", dirs)
	}
	if short, ok := dirs[symbols[3]]; !ok || !short {
		t.Errorf("weakest symbol should be short: %v", dirs)
	}
}

func TestRiskSizingDrawdownGuardBlocksEntries(t *testing.T) {
	withStake(t)
	st := makeStrat(t, RiskSizingName)
	job := newJob(t, st, "BTC/USDT:USDT", "1h")
	state := strat.NewState()
	// Equity 85 against a peak of 100: 15% drawdown, above the default 10% pause level.
	if err := state.BindAccountMetrics(state, config.DefAcc, func(string) strat.AccountMetrics {
		return strat.AccountMetrics{Equity: 85, Available: 85, Peak: 100, Valid: true}
	}); err != nil {
		t.Fatal(err)
	}
	job.BindRuntimeState(state, nil, nil)
	if dd := job.AccountDrawdown(); math.Abs(dd-0.15) > 1e-9 {
		t.Fatalf("AccountDrawdown = %v, want 0.15", dd)
	}
	ms := utc(2024, 1, 1, 0, 0)
	for i := 0; i < 200; i++ {
		px := 200 - float64(i)
		if i >= 100 {
			px = 100 + float64(i-100)*3
		}
		feed(t, job, ms+int64(i)*3_600_000, 3_600_000, px, px+1, px-1, px)
		for _, req := range job.Entrys {
			if req.Tag == "ema_long" || req.Tag == "ema_short" {
				t.Fatalf("entry %s queued during the drawdown pause", req.Tag)
			}
		}
	}
	if !job.More.(*riskState).paused {
		t.Fatal("guard should be paused")
	}
}
