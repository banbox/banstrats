package longshort

import (
	"reflect"
	"strings"
	"testing"

	"github.com/banbox/banbot/biz"
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
)

func TestBinanceLongShortStrategyRegistersAndCollectsSubs(t *testing.T) {
	makeFn, ok := strat.StratMake[StrategyKey()]
	if !ok {
		t.Fatalf("expected strategy %s to be registered", StrategyKey())
	}
	stgy := makeFn(&config.RunPolicyConfig{Name: StrategyKey()})
	if stgy == nil {
		t.Fatalf("expected strategy constructor for %s", StrategyKey())
	}
	if stgy.OnDataSubs == nil {
		t.Fatalf("expected OnDataSubs to be defined")
	}
	if stgy.OnData == nil {
		t.Fatalf("expected OnData to be defined")
	}
	if stgy.OnInfoBar != nil {
		t.Fatalf("expected strategy to use OnData/DataHub path only")
	}
	job := &strat.StratJob{
		Strat:  stgy,
		Symbol: &orm.ExSymbol{ID: 77, Exchange: "binance", Market: "future", Symbol: "BTCUSDT"},
	}
	subs := strat.CollectDataSubs(job)
	if len(subs) != 1 {
		t.Fatalf("expected 1 data sub, got %d", len(subs))
	}
	sub := subs[0]
	if sub.Source != SourceName {
		t.Fatalf("expected source %s, got %+v", SourceName, sub)
	}
	if sub.ExSymbol == nil || sub.ExSymbol.ID != job.Symbol.ID || sub.ExSymbol.Symbol != job.Symbol.Symbol {
		t.Fatalf("expected current symbol subscription, got %+v", sub.ExSymbol)
	}
	if sub.TimeFrame != DefaultTimeframe {
		t.Fatalf("expected timeframe %s, got %s", DefaultTimeframe, sub.TimeFrame)
	}
	if sub.WarmupNum != 2 {
		t.Fatalf("expected warmup 2, got %d", sub.WarmupNum)
	}
}

func TestBinanceLongShortStrategyOnDataAndDataHubProof(t *testing.T) {
	oldAccounts := config.Accounts
	oldInfoJobs := strat.AccInfoJobs
	config.Accounts = map[string]*config.AccountConfig{config.DefAcc: {}}
	strat.AccInfoJobs = map[string]map[string]map[string]*strat.StratJob{config.DefAcc: {}}
	t.Cleanup(func() {
		config.Accounts = oldAccounts
		strat.AccInfoJobs = oldInfoJobs
	})

	job := newExampleJob(t, "BTCUSDT")
	key := strat.DataSubKey(SourceName, job.Symbol.ID, DefaultTimeframe)
	strat.AccInfoJobs[config.DefAcc][key] = map[string]*strat.StratJob{"example": job}

	trader := &biz.Trader{}
	events := []*orm.DataSeries{
		newLongShortSeries(job.Symbol, 1704067200000, 0.61, 0.39, 1.56, true),
		newLongShortSeries(job.Symbol, 1704153600000, 0.58, 0.42, 1.38, false),
	}
	for _, evt := range events {
		if err := trader.FeedSeries(evt); err != nil {
			t.Fatalf("FeedSeries returned error for source=%s sid=%d tf=%s: %v", evt.Source, evt.Sid, evt.TimeFrame, err)
		}
	}
	state := EnsureStrategyState(job)
	if state.LastError != "" {
		t.Fatalf("expected no strategy error, got %s", state.LastError)
	}
	if state.SeenEvents != 2 {
		t.Fatalf("expected 2 seen events, got %d", state.SeenEvents)
	}
	if state.IgnoredEvents != 0 {
		t.Fatalf("expected 0 ignored events, got %d", state.IgnoredEvents)
	}
	if state.LastSource != SourceName || state.LastSid != job.Symbol.ID || state.LastTimeFrame != DefaultTimeframe {
		t.Fatalf("unexpected last routing identity: %+v", state)
	}
	if state.LastLongQty != 0.58 || state.LastShortQty != 0.42 || state.LastRatio != 1.38 {
		t.Fatalf("unexpected last event values: %+v", state)
	}
	if state.LatestLongQty != 0.58 || state.LatestShortQty != 0.42 || state.LatestRatio != 1.38 {
		t.Fatalf("unexpected latest hub values: %+v", state)
	}
	if state.WindowCount != 2 {
		t.Fatalf("expected window count 2, got %d", state.WindowCount)
	}
	if !reflect.DeepEqual(state.WindowLongQtys, []float64{0.61, 0.58}) {
		t.Fatalf("unexpected window long qtys: %+v", state.WindowLongQtys)
	}
	if !reflect.DeepEqual(state.WindowShortQtys, []float64{0.39, 0.42}) {
		t.Fatalf("unexpected window short qtys: %+v", state.WindowShortQtys)
	}
	if !reflect.DeepEqual(state.WindowWarmups, []bool{true, false}) {
		t.Fatalf("unexpected window warmup flags: %+v", state.WindowWarmups)
	}
	if state.LastEventWarmUp {
		t.Fatalf("expected last event to be live, got warmup")
	}
	if state.LastJobWarmUp || job.IsWarmUp {
		t.Fatalf("expected job warmup cleared after live event: state=%v job=%v", state.LastJobWarmUp, job.IsWarmUp)
	}
	latest := job.DataHub.Latest(SourceName, job.Symbol.ID, DefaultTimeframe)
	if latest == nil {
		t.Fatalf("expected latest hub event")
	}
	if got, err := latest.FloatValue("longQty"); err != nil || got != 0.58 {
		t.Fatalf("expected latest longQty 0.58, got %v err=%v", got, err)
	}
	if got, err := latest.FloatValue("shortQty"); err != nil || got != 0.42 {
		t.Fatalf("expected latest shortQty 0.42, got %v err=%v", got, err)
	}
	window := job.DataHub.Window(SourceName, job.Symbol.ID, DefaultTimeframe, 2)
	if len(window) != 2 || !window[0].IsWarmUp || window[1].IsWarmUp {
		t.Fatalf("unexpected hub window ordering/warmup: %+v", window)
	}
	if got, err := window[0].FloatValue("longShortRatio"); err != nil || got != 1.56 {
		t.Fatalf("expected first ratio 1.56, got %v err=%v", got, err)
	}
}

func TestBinanceLongShortStrategyIgnoresUnrelatedSource(t *testing.T) {
	job := newExampleJob(t, "BTCUSDT")
	evt := newLongShortSeries(job.Symbol, 1704067200000, 0.51, 0.49, 1.04, false)
	evt.Source = "other_source"
	job.Strat.OnData(job, evt)
	state := EnsureStrategyState(job)
	if state.IgnoredEvents != 1 {
		t.Fatalf("expected ignored event count 1, got %d", state.IgnoredEvents)
	}
	if state.SeenEvents != 0 {
		t.Fatalf("expected no seen events, got %d", state.SeenEvents)
	}
	if state.LastError != "" {
		t.Fatalf("expected unrelated source to be ignored without error, got %s", state.LastError)
	}
}

func TestBinanceLongShortStrategyReportsMissingDataHub(t *testing.T) {
	job := newExampleJob(t, "BTCUSDT")
	job.DataHub = nil
	job.Strat.OnData(job, newLongShortSeries(job.Symbol, 1704067200000, 0.5, 0.5, 1.0, false))
	state := EnsureStrategyState(job)
	if !strings.Contains(state.LastError, "missing DataHub") {
		t.Fatalf("expected missing DataHub error, got %s", state.LastError)
	}
	if !strings.Contains(state.LastError, "source="+SourceName) || !strings.Contains(state.LastError, "tf="+DefaultTimeframe) {
		t.Fatalf("expected contextual DataHub error, got %s", state.LastError)
	}
}

func TestBinanceLongShortStrategyReportsMalformedValues(t *testing.T) {
	job := newExampleJob(t, "BTCUSDT")
	evt := newLongShortSeries(job.Symbol, 1704067200000, 0.5, 0.5, 1.0, false)
	evt.Values = map[string]any{"shortQty": 0.5, "longShortRatio": 1.0}
	job.DataHub.Set(evt)
	job.Strat.OnData(job, evt)
	state := EnsureStrategyState(job)
	if !strings.Contains(state.LastError, "invalid event field longQty") {
		t.Fatalf("expected malformed longQty error, got %s", state.LastError)
	}
	if state.SeenEvents != 0 {
		t.Fatalf("expected malformed event to prevent strategy success, got %d", state.SeenEvents)
	}
}

func TestBinanceLongShortDataHubWindowBoundedAndLatestReadable(t *testing.T) {
	job := newExampleJob(t, "BTCUSDT")
	job.DataHub = strat.NewDataHub(2)
	events := []*orm.DataSeries{
		newLongShortSeries(job.Symbol, 1704067200000, 0.60, 0.40, 1.50, true),
		newLongShortSeries(job.Symbol, 1704153600000, 0.57, 0.43, 1.33, false),
		newLongShortSeries(job.Symbol, 1704240000000, 0.55, 0.45, 1.22, false),
	}
	for _, evt := range events {
		job.DataHub.Set(evt)
		job.IsWarmUp = evt.IsWarmUp
		job.Strat.OnData(job, evt)
	}
	state := EnsureStrategyState(job)
	if state.LastError != "" {
		t.Fatalf("expected no strategy error, got %s", state.LastError)
	}
	if state.WindowCount != 2 {
		t.Fatalf("expected bounded window count 2, got %d", state.WindowCount)
	}
	if !reflect.DeepEqual(state.WindowLongQtys, []float64{0.57, 0.55}) {
		t.Fatalf("unexpected bounded longQty window: %+v", state.WindowLongQtys)
	}
	if !reflect.DeepEqual(state.WindowWarmups, []bool{false, false}) {
		t.Fatalf("unexpected bounded warmups: %+v", state.WindowWarmups)
	}
	latest := job.DataHub.Latest(SourceName, job.Symbol.ID, DefaultTimeframe)
	if latest == nil || latest.TimeMS != 1704240000000 {
		t.Fatalf("unexpected latest after bounded window test: %+v", latest)
	}
	if got, err := latest.FloatValue("longQty"); err != nil || got != 0.55 {
		t.Fatalf("expected latest longQty 0.55, got %v err=%v", got, err)
	}
	window := job.DataHub.Window(SourceName, job.Symbol.ID, DefaultTimeframe, 0)
	if len(window) != 2 || window[0].TimeMS != 1704153600000 || window[1].TimeMS != 1704240000000 {
		t.Fatalf("unexpected bounded hub window: %+v", window)
	}
}

func TestBinanceLongShortDataHubIgnoresLegacyKlineEntriesWithSameSidAndTf(t *testing.T) {
	job := newExampleJob(t, "BTCUSDT")
	job.DataHub = strat.NewDataHub(4)
	longShortWarmup := newLongShortSeries(job.Symbol, 1704067200000, 0.61, 0.39, 1.56, true)
	legacyKline := &orm.DataSeries{
		Source:    "kline",
		Sid:       job.Symbol.ID,
		ExSymbol:  job.Symbol,
		TimeMS:    1704153600000,
		EndMS:     1704240000000,
		TimeFrame: DefaultTimeframe,
		Closed:    true,
		Values: map[string]any{
			"open":   1.0,
			"high":   2.0,
			"low":    0.5,
			"close":  1.5,
			"volume": 12.0,
		},
	}
	longShortLive := newLongShortSeries(job.Symbol, 1704240000000, 0.58, 0.42, 1.38, false)

	job.DataHub.Set(longShortWarmup)
	job.IsWarmUp = longShortWarmup.IsWarmUp
	job.Strat.OnData(job, longShortWarmup)
	job.DataHub.Set(legacyKline)
	job.DataHub.Set(longShortLive)
	job.IsWarmUp = longShortLive.IsWarmUp
	job.Strat.OnData(job, longShortLive)

	state := EnsureStrategyState(job)
	if state.LastError != "" {
		t.Fatalf("expected no strategy error after mixed-source hub history, got %s", state.LastError)
	}
	if state.SeenEvents != 2 || state.IgnoredEvents != 0 {
		t.Fatalf("expected only longshort events to count as seen, got %+v", state)
	}
	if !reflect.DeepEqual(state.WindowLongQtys, []float64{0.61, 0.58}) {
		t.Fatalf("expected strategy window to ignore legacy kline values, got %+v", state.WindowLongQtys)
	}
	if !reflect.DeepEqual(state.WindowWarmups, []bool{true, false}) {
		t.Fatalf("expected strategy warmup window to remain source-scoped, got %+v", state.WindowWarmups)
	}
	if got := job.DataHub.Window("kline", job.Symbol.ID, DefaultTimeframe, 0); len(got) != 1 || got[0].TimeMS != legacyKline.TimeMS {
		t.Fatalf("expected legacy kline history preserved separately, got %+v", got)
	}
	if got := job.DataHub.Window(SourceName, job.Symbol.ID, DefaultTimeframe, 0); len(got) != 2 || got[0].TimeMS != longShortWarmup.TimeMS || got[1].TimeMS != longShortLive.TimeMS {
		t.Fatalf("expected longshort history preserved separately, got %+v", got)
	}
}

func newExampleJob(t *testing.T, symbol string) *strat.StratJob {
	t.Helper()
	makeFn, ok := strat.StratMake[StrategyKey()]
	if !ok {
		t.Fatalf("expected strategy %s to be registered", StrategyKey())
	}
	stgy := makeFn(&config.RunPolicyConfig{Name: StrategyKey()})
	if stgy == nil {
		t.Fatalf("expected strategy instance")
	}
	job := &strat.StratJob{
		Strat:    stgy,
		DataHub:  strat.NewDataHub(),
		Symbol:   &orm.ExSymbol{ID: 77, Exchange: "binance", Market: "future", Symbol: symbol},
		Account:  config.DefAcc,
		TimeFrame: DefaultTimeframe,
	}
	EnsureStrategyState(job)
	return job
}

func newLongShortSeries(exs *orm.ExSymbol, timeMS int64, longQty, shortQty, ratio float64, warmup bool) *orm.DataSeries {
	return &orm.DataSeries{
		Source:    SourceName,
		Sid:       exs.ID,
		ExSymbol:  exs,
		TimeMS:    timeMS,
		EndMS:     timeMS + 86400000,
		TimeFrame: DefaultTimeframe,
		Closed:    true,
		IsWarmUp:  warmup,
		Values: map[string]any{
			"longQty":        longQty,
			"shortQty":       shortQty,
			"longShortRatio": ratio,
		},
	}
}
