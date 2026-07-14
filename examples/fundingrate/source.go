package fundingrate

import (
	"context"
	"fmt"
	"sort"

	"github.com/banbox/banbot/data"
	"github.com/banbox/banbot/exg"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banexg"
	utils2 "github.com/banbox/banexg/utils"
)

const (
	SourceName       = "funding_rate"
	DefaultTimeframe = "8h"
	DefaultLimit     = 1000
)

var Info = &orm.SeriesInfo{
	Name:      SourceName,
	TimeFrame: DefaultTimeframe,
	Binding: orm.SeriesBinding{
		Table:      "funding_rate",
		TimeColumn: "ts",
		EndColumn:  "end_ms",
		SIDColumn:  "sid",
		Fields: []orm.SeriesField{
			{Name: "rate", Type: "float", Role: "value"},
		},
	},
}

type FetchFunc func(ctx context.Context, symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.FundingRate, error)

func init() {
	if err := RegisterDefaultSource(); err != nil {
		panic(fmt.Sprintf("register %s: %v", SourceName, err))
	}
}

func RegisterDefaultSource() error {
	return data.RegisterFuncDataSource(Info, FetchHistory, nil)
}

func FetchHistory(ctx context.Context, sub *strat.DataSub, startMS, endMS int64) ([]*orm.DataRecord, error) {
	return FetchHistoryWith(ctx, sub, startMS, endMS, defaultFetchFunc)
}

func FetchHistoryWith(ctx context.Context, sub *strat.DataSub, startMS, endMS int64, fetch FetchFunc) ([]*orm.DataRecord, error) {
	if fetch == nil {
		return nil, wrapErr(sub, "fetch", "fetch function is required")
	}
	normalized, err := validateSub(sub)
	if err != nil {
		return nil, err
	}
	if startMS <= 0 {
		return nil, wrapErr(normalized, "request", "start time is required")
	}
	if endMS <= startMS {
		return nil, wrapErr(normalized, "request", "end time must be greater than start time")
	}
	intervalMS := int64(utils2.TFToSecs(Info.TimeFrame)) * 1000
	rowsByTime := make(map[int64]*orm.DataRecord)
	for cursor := startMS; cursor < endMS; {
		pageEnd := cursor + intervalMS*DefaultLimit
		if pageEnd > endMS {
			pageEnd = endMS
		}
		page, err := fetch(ctx, normalized.ExSymbol.Symbol, cursor, DefaultLimit, map[string]interface{}{banexg.ParamUntil: pageEnd - 1})
		if err != nil {
			return nil, wrapErr(normalized, "fetch", "%v", err)
		}
		for _, item := range page {
			if item == nil {
				continue
			}
			if item.Timestamp < cursor || item.Timestamp >= pageEnd {
				continue
			}
			if item.Symbol != "" && item.Symbol != normalized.ExSymbol.Symbol {
				return nil, wrapErr(normalized, "parse", "symbol %q does not match sub symbol %q", item.Symbol, normalized.ExSymbol.Symbol)
			}
			rowsByTime[item.Timestamp] = &orm.DataRecord{
				TimeMS: item.Timestamp,
				EndMS:  item.Timestamp + intervalMS,
				Closed: true,
				Values: map[string]any{
					"rate": item.FundingRate,
				},
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

func defaultFetchFunc(ctx context.Context, symbol string, since int64, limit int, params map[string]interface{}) ([]*banexg.FundingRate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if exg.Default == nil {
		return nil, fmt.Errorf("default exchange is not configured")
	}
	rows, err := exg.Default.FetchFundingRateHistory(symbol, since, limit, params)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func validateSub(sub *strat.DataSub) (*strat.DataSub, error) {
	normalized, err := data.NormalizeDataSub(Info, sub)
	if err != nil {
		return nil, wrapErr(sub, "request", "%v", err)
	}
	return normalized, nil
}

func wrapErr(sub *strat.DataSub, phase, format string, args ...any) error {
	symbol := ""
	timeframe := Info.TimeFrame
	if sub != nil {
		if sub.ExSymbol != nil {
			symbol = sub.ExSymbol.Symbol
		}
		if sub.TimeFrame != "" {
			timeframe = sub.TimeFrame
		}
	}
	return fmt.Errorf("source=%s symbol=%s timeframe=%s phase=%s: %s",
		SourceName, symbol, timeframe, phase, fmt.Sprintf(format, args...))
}
