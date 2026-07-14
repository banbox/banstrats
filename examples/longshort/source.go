package longshort

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
	"github.com/banbox/banbot/strat"
	banbinance "github.com/banbox/banexg/binance"
	utils2 "github.com/banbox/banexg/utils"
)

const (
	SourceName        = "binance_longshort"
	EndpointPath      = "/futures/data/topLongShortPositionRatio"
	EndpointMethod    = banbinance.MethodFapiDataGetTopLongShortPositionRatio
	DefaultTimeframe  = "1d"
	maxRecordsPerPage = 500
	FieldLongAccount  = "longAccount"
	FieldShortAccount = "shortAccount"
	FieldRatio        = "longShortRatio"
)

var supportedPeriods = map[string]struct{}{
	"5m":  {},
	"15m": {},
	"30m": {},
	"1h":  {},
	"2h":  {},
	"4h":  {},
	"6h":  {},
	"12h": {},
	"1d":  {},
}

type FetchRequest struct {
	Method   string
	Endpoint string
	Params   map[string]interface{}
}

type FetchFunc func(ctx context.Context, req FetchRequest) ([]byte, error)

type Source struct {
	info      *orm.SeriesInfo
	endpoint  string
	method    string
	tfMS      int64
	fetchFunc FetchFunc
}

type ratioRow struct {
	Symbol         string          `json:"symbol"`
	LongShortRatio json.RawMessage `json:"longShortRatio"`
	LongAccount    json.RawMessage `json:"longAccount"`
	ShortAccount   json.RawMessage `json:"shortAccount"`
	Timestamp      json.RawMessage `json:"timestamp"`
}

func init() {
	if err := RegisterDefaultSource(); err != nil {
		panic(fmt.Sprintf("register %s: %v", SourceName, err))
	}
}

func RegisterDefaultSource() error {
	src, err := NewSource(DefaultTimeframe, nil)
	if err != nil {
		return err
	}
	return data.RegisterFuncDataSource(src.Info(), src.FetchHistory, src.SubscribeLive)
}

func NewSource(timeframe string, fetchFunc FetchFunc) (*Source, error) {
	if _, ok := supportedPeriods[timeframe]; !ok {
		return nil, fmt.Errorf("%s timeframe %q is unsupported", SourceName, timeframe)
	}
	tfMS := int64(utils2.TFToSecs(timeframe)) * 1000
	if tfMS <= 0 {
		return nil, fmt.Errorf("%s timeframe %q is invalid", SourceName, timeframe)
	}
	if fetchFunc == nil {
		fetchFunc = defaultFetchFunc
	}
	src := &Source{
		endpoint:  EndpointPath,
		method:    EndpointMethod,
		tfMS:      tfMS,
		fetchFunc: fetchFunc,
		info: &orm.SeriesInfo{
			Name:      SourceName,
			TimeFrame: timeframe,
			Binding: orm.SeriesBinding{
				Table:      "binance_longshort",
				TimeColumn: "ts",
				EndColumn:  "end_ms",
				SIDColumn:  "sid",
				Fields: []orm.SeriesField{
					{Name: FieldLongAccount, Type: "float", Role: "value"},
					{Name: FieldShortAccount, Type: "float", Role: "value"},
					{Name: FieldRatio, Type: "float", Role: "value"},
				},
			},
		},
	}
	if err := orm.ValidateSeriesInfo(src.info); err != nil {
		return nil, err
	}
	return src, nil
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

func cloneParams(params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}
	cp := make(map[string]interface{}, len(params))
	for k, v := range params {
		cp[k] = v
	}
	return cp
}

func (s *Source) Info() *orm.SeriesInfo {
	return s.info
}

func (s *Source) FetchHistory(ctx context.Context, sub *strat.DataSub, startMS, endMS int64) ([]*orm.DataRecord, error) {
	if s.fetchFunc == nil {
		return nil, s.wrapErr(sub, "fetch", "fetch function is not configured")
	}
	if _, err := s.validateSub(sub); err != nil {
		return nil, err
	}
	if startMS <= 0 {
		return nil, s.wrapErr(sub, "request", "startTime is required")
	}
	if endMS <= startMS {
		return nil, s.wrapErr(sub, "request", "endTime must be greater than startTime")
	}
	rowsByTime := make(map[int64]*orm.DataRecord)
	for cursor := startMS; cursor < endMS; {
		pageEnd := cursor + s.tfMS*maxRecordsPerPage
		if pageEnd > endMS {
			pageEnd = endMS
		}
		req, err := s.buildRequest(sub, cursor, pageEnd)
		if err != nil {
			return nil, err
		}
		payload, err := s.fetchFunc(ctx, req)
		if err != nil {
			return nil, s.wrapErr(sub, "fetch", "request failed: %v", err)
		}
		pageRows, err := s.normalizePayload(sub, payload)
		if err != nil {
			return nil, err
		}
		for _, row := range pageRows {
			if row != nil && row.TimeMS >= startMS && row.TimeMS < endMS {
				rowsByTime[row.TimeMS] = row
			}
		}
		cursor = pageEnd
	}
	rows := make([]*orm.DataRecord, 0, len(rowsByTime))
	for _, row := range rowsByTime {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TimeMS < rows[j].TimeMS })
	return rows, nil
}

// SubscribeLive is intentionally a validation-only no-op until Binance exposes a
// supported long/short stream that can feed the shared DataSink contract.
func (s *Source) SubscribeLive(ctx context.Context, subs []*strat.DataSub, sink data.DataSink) error {
	if err := ctx.Err(); err != nil {
		return s.wrapErr(nil, "subscribe", "context canceled: %v", err)
	}
	for idx, sub := range subs {
		if _, err := s.validateSub(sub); err != nil {
			return s.wrapErr(sub, "subscribe", "sub[%d] %v", idx, err)
		}
	}
	return nil
}

func (s *Source) validateSub(sub *strat.DataSub) (*strat.DataSub, error) {
	normalized, err := data.NormalizeDataSub(s.info, sub)
	if err != nil {
		return nil, s.wrapErr(sub, "request", "%v", err)
	}
	return normalized, nil
}

func (s *Source) buildRequest(sub *strat.DataSub, startMS, endMS int64) (FetchRequest, error) {
	normalized, err := s.validateSub(sub)
	if err != nil {
		return FetchRequest{}, err
	}
	if startMS <= 0 {
		return FetchRequest{}, s.wrapErr(normalized, "request", "startTime is required")
	}
	if endMS <= startMS {
		return FetchRequest{}, s.wrapErr(normalized, "request", "endTime must be greater than startTime")
	}
	symbol, err := binanceSymbol(normalized.ExSymbol.Symbol)
	if err != nil {
		return FetchRequest{}, s.wrapErr(normalized, "request", "%v", err)
	}
	limit := int(math.Ceil(float64(endMS-startMS) / float64(s.tfMS)))
	if limit < 1 {
		limit = 1
	}
	if limit > maxRecordsPerPage {
		limit = maxRecordsPerPage
	}
	return FetchRequest{
		Method:   s.method,
		Endpoint: s.endpoint,
		Params: map[string]interface{}{
			"symbol":    symbol,
			"period":    s.info.TimeFrame,
			"startTime": startMS,
			"endTime":   endMS - 1,
			"limit":     limit,
		},
	}, nil
}

func (s *Source) normalizePayload(sub *strat.DataSub, payload []byte) ([]*orm.DataRecord, error) {
	var raw []ratioRow
	if err := utils2.Unmarshal(payload, &raw, utils2.JsonNumDefault); err != nil {
		return nil, s.wrapErr(sub, "parse", "invalid JSON payload: %v", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	rows := make([]*orm.DataRecord, 0, len(raw))
	for idx, item := range raw {
		row, err := s.normalizeRow(sub, idx, item)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *Source) normalizeRow(sub *strat.DataSub, idx int, item ratioRow) (*orm.DataRecord, error) {
	if strings.TrimSpace(item.Symbol) == "" {
		return nil, s.wrapErr(sub, "parse", "row[%d] missing field symbol", idx)
	}
	if sub != nil && sub.ExSymbol != nil && strings.TrimSpace(sub.ExSymbol.Symbol) != "" {
		expected, err := binanceSymbol(sub.ExSymbol.Symbol)
		if err != nil {
			return nil, s.wrapErr(sub, "parse", "row[%d] %v", idx, err)
		}
		if item.Symbol != sub.ExSymbol.Symbol && item.Symbol != expected {
			return nil, s.wrapErr(sub, "parse", "row[%d] symbol %q does not match sub symbol %q", idx, item.Symbol, sub.ExSymbol.Symbol)
		}
	}
	timeMS, err := data.ParseJSONInt(item.Timestamp, "timestamp")
	if err != nil {
		return nil, s.wrapErr(sub, "parse", "row[%d] %v", idx, err)
	}
	longAccount, err := data.ParseJSONFloat(item.LongAccount, "longAccount")
	if err != nil {
		return nil, s.wrapErr(sub, "parse", "row[%d] %v", idx, err)
	}
	shortAccount, err := data.ParseJSONFloat(item.ShortAccount, "shortAccount")
	if err != nil {
		return nil, s.wrapErr(sub, "parse", "row[%d] %v", idx, err)
	}
	ratio, err := data.ParseJSONFloat(item.LongShortRatio, "longShortRatio")
	if err != nil {
		return nil, s.wrapErr(sub, "parse", "row[%d] %v", idx, err)
	}
	values := map[string]any{
		FieldLongAccount:  longAccount,
		FieldShortAccount: shortAccount,
		FieldRatio:        ratio,
	}
	return &orm.DataRecord{
		TimeMS: timeMS,
		EndMS:  timeMS + s.tfMS,
		Closed: true,
		Values: values,
	}, nil
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
	if strings.Contains(baseQuote, "/") {
		baseQuote = strings.ReplaceAll(baseQuote, "/", "")
	}
	if strings.ContainsAny(baseQuote, ":/ ") || baseQuote == "" {
		return "", fmt.Errorf("cannot map symbol %q to Binance market id", symbol)
	}
	return baseQuote, nil
}

func (s *Source) wrapErr(sub *strat.DataSub, phase, format string, args ...any) error {
	symbol := ""
	tf := s.info.TimeFrame
	if sub != nil {
		if sub.ExSymbol != nil {
			symbol = sub.ExSymbol.Symbol
		}
		if sub.TimeFrame != "" {
			tf = sub.TimeFrame
		}
	}
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("source=%s symbol=%s timeframe=%s endpoint=%s phase=%s: %s", s.info.Name, symbol, tf, s.endpoint, phase, msg)
}

var _ data.DataSource = (*Source)(nil)
