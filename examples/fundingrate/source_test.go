package fundingrate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/banbox/banbot/data"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg"
)

func TestFundingRateRegistersFuncDataSource(t *testing.T) {
	src := data.GetDataSource(SourceName)
	if src == nil {
		t.Fatalf("expected %s to be registered", SourceName)
	}
	if _, ok := src.(*data.FuncDataSource); !ok {
		t.Fatalf("expected registered source to use data.FuncDataSource, got %T", src)
	}
	if src.Info().Binding.Table != "funding_rate" {
		t.Fatalf("unexpected table: %s", src.Info().Binding.Table)
	}
	if err := RegisterDefaultSource(); err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}
}

func TestFundingRateFetchHistoryWith(t *testing.T) {
	calls := make([]struct {
		symbol string
		since  int64
		limit  int
		params map[string]interface{}
	}, 0)
	sub := validSub("BTC/USDT:USDT", DefaultTimeframe)
	rows, err := FetchHistoryWith(context.Background(), sub, 1704067200000, 1704153600000,
		func(ctx context.Context, symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.FundingRate, error) {
			calls = append(calls, struct {
				symbol string
				since  int64
				limit  int
				params map[string]interface{}
			}{symbol: symbol, since: since, limit: limit, params: params})
			return []*banexg.FundingRate{
				{Symbol: symbol, FundingRate: 0.0001, Timestamp: 1704067200000},
				{Symbol: symbol, FundingRate: -0.0002, Timestamp: 1704096000000},
			}, nil
		})
	if err != nil {
		t.Fatalf("FetchHistoryWith failed: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one fetch call, got %d", len(calls))
	}
	if calls[0].symbol != "BTC/USDT:USDT" || calls[0].since != 1704067200000 || calls[0].limit != DefaultLimit {
		t.Fatalf("unexpected call: %+v", calls[0])
	}
	if calls[0].params[banexg.ParamUntil] != int64(1704153599999) {
		t.Fatalf("expected until param, got %+v", calls[0].params)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].TimeMS != 1704067200000 || rows[0].EndMS != 1704096000000 || rows[0].Values["rate"] != 0.0001 {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].Values["rate"] != -0.0002 {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestFundingRateFetchHistoryWithChunksSortsAndDeduplicates(t *testing.T) {
	start := int64(1704067200000)
	interval := int64(8 * 60 * 60 * 1000)
	end := start + interval*(DefaultLimit+1)
	var calls int
	rows, err := FetchHistoryWith(context.Background(), validSub("BTC/USDT:USDT", DefaultTimeframe), start, end,
		func(ctx context.Context, symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.FundingRate, error) {
			calls++
			until := params[banexg.ParamUntil].(int64)
			return []*banexg.FundingRate{
				{Symbol: symbol, FundingRate: float64(calls), Timestamp: until + 1},
				{Symbol: symbol, FundingRate: float64(calls), Timestamp: since},
				{Symbol: symbol, FundingRate: float64(calls) + 0.5, Timestamp: since},
			}, nil
		})
	if err != nil {
		t.Fatalf("FetchHistoryWith failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 bounded calls, got %d", calls)
	}
	if len(rows) != 2 || rows[0].TimeMS != start || rows[1].TimeMS != start+interval*DefaultLimit {
		t.Fatalf("expected sorted unique in-range rows, got %+v", rows)
	}
	if got := rows[0].Values["rate"]; got != 1.5 {
		t.Fatalf("expected latest duplicate to win, got %v", got)
	}
}

func TestFundingRateFetchHistoryRejectsBadInput(t *testing.T) {
	_, err := FetchHistoryWith(context.Background(), nil, 1, 2, func(context.Context, string, int64, int, map[string]interface{}) ([]*banexg.FundingRate, error) {
		return nil, nil
	})
	assertErrContains(t, err, "data sub is required")

	_, err = FetchHistoryWith(context.Background(), validSub("BTCUSDT", "1h"), 1, 2, func(context.Context, string, int64, int, map[string]interface{}) ([]*banexg.FundingRate, error) {
		return nil, nil
	})
	assertErrContains(t, err, `unsupported timeframe "1h"`)

	wantErr := errors.New("boom")
	_, err = FetchHistoryWith(context.Background(), validSub("BTCUSDT", DefaultTimeframe), 1, 2, func(context.Context, string, int64, int, map[string]interface{}) ([]*banexg.FundingRate, error) {
		return nil, wantErr
	})
	assertErrContains(t, err, "phase=fetch")
	assertErrContains(t, err, "boom")
}

func validSub(symbol, timeframe string) *strat.DataSub {
	return &strat.DataSub{
		Source:    SourceName,
		ExSymbol:  &orm.ExSymbol{ID: 1, Symbol: symbol, Exchange: "binance", Market: "future"},
		TimeFrame: timeframe,
	}
}

func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), strings.ReplaceAll(want, `\"`, `"`)) {
		t.Fatalf("expected error %q to contain %q", err.Error(), want)
	}
}
