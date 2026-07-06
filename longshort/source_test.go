package longshort

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/banbox/banbot/data"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
)

func TestBinanceLongShortSourceInfo(t *testing.T) {
	src, err := NewSource(DefaultTimeframe, nil)
	if err != nil {
		t.Fatalf("NewSource returned error: %v", err)
	}
	info := src.Info()
	if info.Name != SourceName {
		t.Fatalf("expected source name %q, got %q", SourceName, info.Name)
	}
	if info.TimeFrame != DefaultTimeframe {
		t.Fatalf("expected timeframe %s, got %q", DefaultTimeframe, info.TimeFrame)
	}
	if src.endpoint != EndpointPath {
		t.Fatalf("expected endpoint %q, got %q", EndpointPath, src.endpoint)
	}
	if src.method != EndpointMethod {
		t.Fatalf("expected method %q, got %q", EndpointMethod, src.method)
	}
	wantFields := []string{"longQty", "shortQty", "longShortRatio"}
	if len(info.Binding.Fields) != len(wantFields) {
		t.Fatalf("expected %d fields, got %d", len(wantFields), len(info.Binding.Fields))
	}
	for i, name := range wantFields {
		if info.Binding.Fields[i].Name != name {
			t.Fatalf("expected field %d to be %q, got %q", i, name, info.Binding.Fields[i].Name)
		}
	}
}

func TestBinanceLongShortRegistersSource(t *testing.T) {
	src := data.GetDataSource(SourceName)
	if src == nil {
		t.Fatalf("expected %s to be registered during package init", SourceName)
	}
	if src.Info().TimeFrame != DefaultTimeframe {
		t.Fatalf("expected registered timeframe %s, got %s", DefaultTimeframe, src.Info().TimeFrame)
	}
	if err := RegisterDefaultSource(); err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}
}

func TestBinanceLongShortFetchHistory(t *testing.T) {
	t.Run("single page shapes request and normalizes rows", func(t *testing.T) {
		var gotReq FetchRequest
		src, err := NewSource(DefaultTimeframe, func(ctx context.Context, req FetchRequest) ([]byte, error) {
			gotReq = req
			return []byte(`[
				{"symbol":"BTCUSDT","longShortRatio":"1.4342","longAccount":"0.5891","shortAccount":"0.4109","timestamp":"1704067200000"},
				{"symbol":"BTCUSDT","longShortRatio":"1.2000","longAccount":"0.5454","shortAccount":"0.4546","timestamp":1704153600000}
			]`), nil
		})
		if err != nil {
			t.Fatalf("NewSource returned error: %v", err)
		}
		sub := validSub("BTCUSDT", DefaultTimeframe)
		rows, err := src.FetchHistory(context.Background(), sub, 1704067200000, 1704240000000)
		if err != nil {
			t.Fatalf("FetchHistory returned error: %v", err)
		}
		if gotReq.Endpoint != EndpointPath {
			t.Fatalf("expected endpoint %q, got %q", EndpointPath, gotReq.Endpoint)
		}
		if gotReq.Method != EndpointMethod {
			t.Fatalf("expected method %q, got %q", EndpointMethod, gotReq.Method)
		}
		assertParamEqual(t, gotReq.Params, "symbol", "BTCUSDT")
		assertParamEqual(t, gotReq.Params, "period", DefaultTimeframe)
		assertParamEqual(t, gotReq.Params, "startTime", int64(1704067200000))
		assertParamEqual(t, gotReq.Params, "endTime", int64(1704240000000))
		assertParamEqual(t, gotReq.Params, "limit", 2)
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(rows))
		}
		assertRow := func(idx int, row *orm.DataRecord, wantTime int64, wantLong, wantShort, wantRatio float64) {
			t.Helper()
			if row == nil {
				t.Fatalf("row %d is nil", idx)
			}
			if row.TimeMS != wantTime {
				t.Fatalf("row %d expected time %d, got %d", idx, wantTime, row.TimeMS)
			}
			if row.EndMS != wantTime+86400000 {
				t.Fatalf("row %d expected end %d, got %d", idx, wantTime+86400000, row.EndMS)
			}
			if !row.Closed {
				t.Fatalf("row %d expected Closed=true", idx)
			}
			if len(row.Values) != len(src.Info().Binding.Fields) {
				t.Fatalf("row %d expected %d values, got %d: %+v", idx, len(src.Info().Binding.Fields), len(row.Values), row.Values)
			}
			for _, field := range src.Info().Binding.Fields {
				if _, ok := row.Values[field.Name]; !ok {
					t.Fatalf("row %d missing declared field %q in values: %+v", idx, field.Name, row.Values)
				}
			}
			if got := row.Values["longQty"].(float64); got != wantLong {
				t.Fatalf("row %d expected longQty=%v, got %v", idx, wantLong, got)
			}
			if got := row.Values["shortQty"].(float64); got != wantShort {
				t.Fatalf("row %d expected shortQty=%v, got %v", idx, wantShort, got)
			}
			if got := row.Values["longShortRatio"].(float64); got != wantRatio {
				t.Fatalf("row %d expected longShortRatio=%v, got %v", idx, wantRatio, got)
			}
		}
		assertRow(0, rows[0], 1704067200000, 0.5891, 0.4109, 1.4342)
		assertRow(1, rows[1], 1704153600000, 0.5454, 0.4546, 1.2)
	})

	t.Run("multiple pages keep request ranges bounded", func(t *testing.T) {
		calls := make([]FetchRequest, 0)
		src, err := NewSource(DefaultTimeframe, func(ctx context.Context, req FetchRequest) ([]byte, error) {
			calls = append(calls, req)
			return []byte(`[
				{"symbol":"BTCUSDT","longShortRatio":"1.0","longAccount":"0.50","shortAccount":"0.50","timestamp":"1704067200000"}
			]`), nil
		})
		if err != nil {
			t.Fatalf("NewSource returned error: %v", err)
		}
		start := int64(1704067200000)
		end := start + int64(maxRecordsPerPage+1)*86400000
		rows, err := src.FetchHistory(context.Background(), validSub("BTCUSDT", DefaultTimeframe), start, end)
		if err != nil {
			t.Fatalf("FetchHistory returned error: %v", err)
		}
		if len(calls) != 2 {
			t.Fatalf("expected 2 paged requests, got %d", len(calls))
		}
		assertParamEqual(t, calls[0].Params, "limit", maxRecordsPerPage)
		assertParamEqual(t, calls[0].Params, "startTime", start)
		assertParamEqual(t, calls[0].Params, "endTime", start+int64(maxRecordsPerPage)*86400000)
		assertParamEqual(t, calls[1].Params, "limit", 1)
		assertParamEqual(t, calls[1].Params, "startTime", start+int64(maxRecordsPerPage)*86400000)
		assertParamEqual(t, calls[1].Params, "endTime", end)
		if len(rows) != 2 {
			t.Fatalf("expected one normalized row per page, got %d", len(rows))
		}
	})

	t.Run("empty payload returns no rows", func(t *testing.T) {
		src := mustSource(t, []byte(`[]`), nil)
		rows, err := src.FetchHistory(context.Background(), validSub("BTCUSDT", DefaultTimeframe), 1704067200000, 1704153600000)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected no rows, got %d", len(rows))
		}
	})

	t.Run("fetch failure bubbles with context", func(t *testing.T) {
		src, err := NewSource(DefaultTimeframe, func(ctx context.Context, req FetchRequest) ([]byte, error) {
			return nil, errors.New("boom")
		})
		if err != nil {
			t.Fatalf("NewSource returned error: %v", err)
		}
		_, err = src.FetchHistory(context.Background(), validSub("BTCUSDT", DefaultTimeframe), 1704067200000, 1704153600000)
		if err == nil {
			t.Fatalf("expected fetch error")
		}
		assertErrContains(t, err, "source=binance_longshort")
		assertErrContains(t, err, "symbol=BTCUSDT")
		assertErrContains(t, err, "timeframe=1d")
		assertErrContains(t, err, "endpoint=/futures/data/topLongShortPositionRatio")
		assertErrContains(t, err, "phase=fetch")
		assertErrContains(t, err, "request failed: boom")
	})

	t.Run("rejects malformed inputs and payloads", func(t *testing.T) {
		_, err := NewSource("3m", nil)
		if err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("expected unsupported timeframe error, got %v", err)
		}

		src := mustSource(t, []byte(`[]`), nil)
		_, err = src.FetchHistory(context.Background(), validSub("", DefaultTimeframe), 1704067200000, 1704153600000)
		if err == nil {
			t.Fatalf("expected missing symbol error")
		}
		assertErrContains(t, err, "phase=request")
		assertErrContains(t, err, "symbol is required")

		_, err = src.FetchHistory(context.Background(), validSub("BTCUSDT", "4h"), 1704067200000, 1704153600000)
		if err == nil {
			t.Fatalf("expected timeframe mismatch error")
		}
		assertErrContains(t, err, "unsupported timeframe \"4h\"")

		_, err = src.FetchHistory(context.Background(), &strat.DataSub{Source: SourceName, TimeFrame: DefaultTimeframe}, 1704067200000, 1704153600000)
		if err == nil {
			t.Fatalf("expected missing exsymbol error")
		}
		assertErrContains(t, err, "exsymbol is required")

		src = mustSource(t, []byte(`[{"symbol":"BTCUSDT","longShortRatio":"1.2","longAccount":"0.55","shortAccount":"0.45"}]`), nil)
		_, err = src.FetchHistory(context.Background(), validSub("BTCUSDT", DefaultTimeframe), 1704067200000, 1704153600000)
		if err == nil {
			t.Fatalf("expected missing timestamp error")
		}
		assertErrContains(t, err, "missing field timestamp")

		src = mustSource(t, []byte(`[{"symbol":"BTCUSDT","longShortRatio":"1.2","shortAccount":"0.45","timestamp":"1704067200000"}]`), nil)
		_, err = src.FetchHistory(context.Background(), validSub("BTCUSDT", DefaultTimeframe), 1704067200000, 1704153600000)
		if err == nil {
			t.Fatalf("expected missing longAccount error")
		}
		assertErrContains(t, err, "missing field longAccount")

		src = mustSource(t, []byte(`[{"symbol":"BTCUSDT","longShortRatio":"1.2","longAccount":"0.55","timestamp":"1704067200000"}]`), nil)
		_, err = src.FetchHistory(context.Background(), validSub("BTCUSDT", DefaultTimeframe), 1704067200000, 1704153600000)
		if err == nil {
			t.Fatalf("expected missing shortAccount error")
		}
		assertErrContains(t, err, "missing field shortAccount")

		src = mustSource(t, []byte(`[{"symbol":"BTCUSDT","longShortRatio":"x","longAccount":"0.55","shortAccount":"0.45","timestamp":"1704067200000"}]`), nil)
		_, err = src.FetchHistory(context.Background(), validSub("BTCUSDT", DefaultTimeframe), 1704067200000, 1704153600000)
		if err == nil {
			t.Fatalf("expected non numeric ratio error")
		}
		assertErrContains(t, err, "field longShortRatio is not numeric")
	})
}

func TestBinanceLongShortSubscribeLive(t *testing.T) {
	src, err := NewSource(DefaultTimeframe, nil)
	if err != nil {
		t.Fatalf("NewSource returned error: %v", err)
	}
	if err := src.SubscribeLive(context.Background(), nil, nil); err != nil {
		t.Fatalf("expected nil subs to be a deterministic no-op, got %v", err)
	}
	if err := src.SubscribeLive(context.Background(), []*strat.DataSub{validSub("BTCUSDT", DefaultTimeframe)}, nil); err != nil {
		t.Fatalf("expected valid live subscribe to be a no-op, got %v", err)
	}
	if err := src.SubscribeLive(context.Background(), []*strat.DataSub{validSub("BTCUSDT", "4h")}, nil); err == nil {
		t.Fatalf("expected unsupported timeframe to fail")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := src.SubscribeLive(ctx, []*strat.DataSub{validSub("BTCUSDT", DefaultTimeframe)}, nil); err == nil {
		t.Fatalf("expected canceled context to fail")
	}
}

func validSub(symbol, timeframe string) *strat.DataSub {
	return &strat.DataSub{
		Source:    SourceName,
		ExSymbol:  &orm.ExSymbol{ID: 1, Symbol: symbol, Exchange: "binance", Market: "future"},
		TimeFrame: timeframe,
	}
}

func mustSource(t *testing.T, payload []byte, fetchErr error) *Source {
	t.Helper()
	src, err := NewSource(DefaultTimeframe, func(ctx context.Context, req FetchRequest) ([]byte, error) {
		if fetchErr != nil {
			return nil, fetchErr
		}
		return payload, nil
	})
	if err != nil {
		t.Fatalf("NewSource returned error: %v", err)
	}
	return src
}

func assertParamEqual(t *testing.T, params map[string]interface{}, key string, want interface{}) {
	t.Helper()
	got, ok := params[key]
	if !ok {
		t.Fatalf("expected param %q to be present in %+v", key, params)
	}
	if got != want {
		t.Fatalf("expected param %s=%v, got %v", key, want, got)
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
