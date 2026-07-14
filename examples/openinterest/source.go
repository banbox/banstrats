package openinterest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/banbox/banbot/data"
	"github.com/banbox/banbot/exg"
	"github.com/banbox/banbot/orm"
	banbinance "github.com/banbox/banexg/binance"
	utils2 "github.com/banbox/banexg/utils"
)

const (
	EndpointPath      = "/futures/data/openInterestHist"
	EndpointMethod    = banbinance.MethodFapiDataGetOpenInterestHist
	maxRecordsPerPage = 500
)

type FetchRequest struct {
	Method   string
	Endpoint string
	Params   map[string]interface{}
}

type FetchFunc func(ctx context.Context, req FetchRequest) ([]byte, error)

type historyRow struct {
	Symbol            string          `json:"symbol"`
	OpenInterest      json.RawMessage `json:"sumOpenInterest"`
	OpenInterestValue json.RawMessage `json:"sumOpenInterestValue"`
	CirculatingSupply json.RawMessage `json:"CMCCirculatingSupply"`
	Timestamp         json.RawMessage `json:"timestamp"`
}

func FetchHistory(ctx context.Context, exs *orm.ExSymbol, startMS, endMS int64) ([]Row, error) {
	return FetchHistoryWith(ctx, exs, startMS, endMS, defaultFetchFunc)
}

func FetchHistoryWith(ctx context.Context, exs *orm.ExSymbol, startMS, endMS int64, fetch FetchFunc) ([]Row, error) {
	if fetch == nil {
		return nil, wrapErr(exs, "fetch", "fetch function is required")
	}
	if exs == nil {
		return nil, wrapErr(nil, "request", "exsymbol is required")
	}
	symbol, err := binanceSymbol(exs.Symbol)
	if err != nil {
		return nil, wrapErr(exs, "request", "%v", err)
	}
	if startMS <= 0 {
		return nil, wrapErr(exs, "request", "startTime is required")
	}
	if endMS <= startMS {
		return nil, wrapErr(exs, "request", "endTime must be greater than startTime")
	}
	tfMS := int64(utils2.TFToSecs(DefaultTimeframe)) * 1000
	rowsByTime := make(map[int64]Row)
	for cursor := startMS; cursor < endMS; {
		pageEnd := cursor + tfMS*maxRecordsPerPage
		if pageEnd > endMS {
			pageEnd = endMS
		}
		limit := int(math.Ceil(float64(pageEnd-cursor) / float64(tfMS)))
		req := FetchRequest{
			Method:   EndpointMethod,
			Endpoint: EndpointPath,
			Params: map[string]interface{}{
				"symbol":    symbol,
				"period":    DefaultTimeframe,
				"startTime": cursor,
				"endTime":   pageEnd - 1,
				"limit":     limit,
			},
		}
		payload, err := fetch(ctx, req)
		if err != nil {
			return nil, wrapErr(exs, "fetch", "request failed: %v", err)
		}
		page, err := normalizePayload(exs, symbol, payload)
		if err != nil {
			return nil, err
		}
		for _, row := range page {
			if row.TimeMS >= cursor && row.TimeMS < pageEnd {
				rowsByTime[row.TimeMS] = row
			}
		}
		cursor = pageEnd
	}
	rows := make([]Row, 0, len(rowsByTime))
	for _, row := range rowsByTime {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TimeMS < rows[j].TimeMS })
	return rows, nil
}

func FetchAndSave(ctx context.Context, exs *orm.ExSymbol, startMS, endMS int64) ([]Row, error) {
	rows, err := FetchHistory(ctx, exs, startMS, endMS)
	if err != nil {
		return nil, err
	}
	if err := Save(ctx, exs, rows); err != nil {
		return nil, wrapErr(exs, "store", "%v", err)
	}
	return rows, nil
}

func defaultFetchFunc(ctx context.Context, req FetchRequest) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if exg.Default == nil {
		return nil, fmt.Errorf("default exchange is not configured")
	}
	rsp, err := exg.Default.Call(req.Method, cloneParams(req.Params))
	if err != nil {
		return nil, err
	}
	if rsp == nil {
		return nil, fmt.Errorf("exchange returned nil response")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []byte(rsp.Content), nil
}

func normalizePayload(exs *orm.ExSymbol, apiSymbol string, payload []byte) ([]Row, error) {
	var raw []historyRow
	if err := utils2.Unmarshal(payload, &raw, utils2.JsonNumDefault); err != nil {
		return nil, wrapErr(exs, "parse", "invalid JSON payload: %v", err)
	}
	rows := make([]Row, 0, len(raw))
	for idx, item := range raw {
		if item.Symbol == "" {
			return nil, wrapErr(exs, "parse", "row[%d] missing field symbol", idx)
		}
		if item.Symbol != apiSymbol && item.Symbol != exs.Symbol {
			return nil, wrapErr(exs, "parse", "row[%d] symbol %q does not match %q", idx, item.Symbol, exs.Symbol)
		}
		timeMS, err := data.ParseJSONInt(item.Timestamp, "timestamp")
		if err != nil {
			return nil, wrapErr(exs, "parse", "row[%d] %v", idx, err)
		}
		openInterest, err := data.ParseJSONFloat(item.OpenInterest, "sumOpenInterest")
		if err != nil {
			return nil, wrapErr(exs, "parse", "row[%d] %v", idx, err)
		}
		openInterestValue, err := data.ParseJSONFloat(item.OpenInterestValue, "sumOpenInterestValue")
		if err != nil {
			return nil, wrapErr(exs, "parse", "row[%d] %v", idx, err)
		}
		circulatingSupply, err := data.ParseJSONFloat(item.CirculatingSupply, "CMCCirculatingSupply")
		if err != nil {
			return nil, wrapErr(exs, "parse", "row[%d] %v", idx, err)
		}
		rows = append(rows, Row{
			TimeMS:            timeMS,
			OpenInterest:      openInterest,
			OpenInterestValue: openInterestValue,
			CirculatingSupply: circulatingSupply,
		})
	}
	return rows, nil
}

func cloneParams(params map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(params))
	for key, value := range params {
		cp[key] = value
	}
	return cp
}

func binanceSymbol(symbol string) (string, error) {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return "", fmt.Errorf("symbol is required")
	}
	if exg.Default != nil {
		if market, err := exg.Default.GetMarket(symbol); err == nil && market != nil && market.ID != "" {
			return market.ID, nil
		}
	}
	baseQuote := strings.SplitN(symbol, ":", 2)[0]
	baseQuote = strings.ReplaceAll(baseQuote, "/", "")
	if strings.ContainsAny(baseQuote, ":/ ") || baseQuote == "" {
		return "", fmt.Errorf("cannot map symbol %q to Binance market id", symbol)
	}
	return baseQuote, nil
}

func wrapErr(exs *orm.ExSymbol, phase, format string, args ...any) error {
	symbol := ""
	if exs != nil {
		symbol = exs.Symbol
	}
	return fmt.Errorf("source=%s symbol=%s timeframe=%s endpoint=%s phase=%s: %s",
		SourceName, symbol, DefaultTimeframe, EndpointPath, phase, fmt.Sprintf(format, args...))
}
