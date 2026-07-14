package openinterest

import (
	"context"
	"testing"

	"github.com/banbox/banbot/orm"
)

func TestFetchHistoryWithMapsAllFieldsAndUsesHalfOpenRange(t *testing.T) {
	exs := &orm.ExSymbol{ID: 1, Exchange: "binance", Market: "linear", Symbol: "BTC/USDT:USDT"}
	start := int64(1704067200000)
	end := start + 2*60*60*1000
	var got FetchRequest
	rows, err := FetchHistoryWith(context.Background(), exs, start, end, func(ctx context.Context, req FetchRequest) ([]byte, error) {
		got = req
		return []byte(`[
			{"symbol":"BTCUSDT","sumOpenInterest":"100.5","sumOpenInterestValue":"6400000.25","CMCCirculatingSupply":"19000000","timestamp":"1704070800000"},
			{"symbol":"BTCUSDT","sumOpenInterest":"101","sumOpenInterestValue":"6500000","CMCCirculatingSupply":"19000001","timestamp":"1704067200000"},
			{"symbol":"BTCUSDT","sumOpenInterest":"999","sumOpenInterestValue":"999","CMCCirculatingSupply":"999","timestamp":"1704074400000"}
		]`), nil
	})
	if err != nil {
		t.Fatalf("FetchHistoryWith failed: %v", err)
	}
	if got.Method != EndpointMethod || got.Endpoint != EndpointPath {
		t.Fatalf("unexpected request: %+v", got)
	}
	if got.Params["symbol"] != "BTCUSDT" || got.Params["endTime"] != end-1 || got.Params["limit"] != 2 {
		t.Fatalf("unexpected request params: %+v", got.Params)
	}
	if len(rows) != 2 || rows[0].TimeMS != start || rows[1].TimeMS != start+60*60*1000 {
		t.Fatalf("expected sorted in-range rows, got %+v", rows)
	}
	if rows[0].OpenInterest != 101 || rows[0].OpenInterestValue != 6500000 || rows[0].CirculatingSupply != 19000001 {
		t.Fatalf("unexpected mapped row: %+v", rows[0])
	}
}

func TestFetchHistoryWithRejectsMissingFields(t *testing.T) {
	exs := &orm.ExSymbol{ID: 1, Exchange: "binance", Market: "linear", Symbol: "BTCUSDT"}
	_, err := FetchHistoryWith(context.Background(), exs, 1, 2, func(ctx context.Context, req FetchRequest) ([]byte, error) {
		return []byte(`[{
			"symbol":"BTCUSDT","sumOpenInterest":"100","CMCCirculatingSupply":"19000000","timestamp":"1"
		}]`), nil
	})
	if err == nil {
		t.Fatal("expected missing sumOpenInterestValue to fail")
	}
}
