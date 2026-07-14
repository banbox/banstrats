package openinterest

import (
	"context"

	"github.com/banbox/banbot/orm"
)

const (
	SourceName             = "open_interest"
	DefaultTimeframe       = "1h"
	FieldValue             = "open_interest"
	FieldOpenInterestValue = "open_interest_value"
	FieldCirculatingSupply = "circulating_supply"
)

var Info = orm.NewKLineSeriesInfo(SourceName, DefaultTimeframe, []orm.SeriesField{
	{Name: FieldValue, Type: "float", Role: "value"},
	{Name: FieldOpenInterestValue, Type: "float", Role: "value"},
	{Name: FieldCirculatingSupply, Type: "float", Role: "custom"},
})

type Row struct {
	TimeMS            int64
	OpenInterest      float64
	OpenInterestValue float64
	CirculatingSupply float64
}

func Save(ctx context.Context, exs *orm.ExSymbol, rows []Row) error {
	records := make([]*orm.DataRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, &orm.DataRecord{
			TimeMS: row.TimeMS,
			Values: map[string]any{
				FieldValue:             row.OpenInterest,
				FieldOpenInterestValue: row.OpenInterestValue,
				FieldCirculatingSupply: row.CirculatingSupply,
			},
		})
	}
	if err := orm.NewKLineSeriesStore(Info).Write(ctx, exs, records); err != nil {
		return err
	}
	return nil
}

func Load(ctx context.Context, exs *orm.ExSymbol, startMS, endMS int64, limit int) ([]*orm.DataSeries, error) {
	rows, err := orm.NewKLineSeriesStore(Info).Read(ctx, exs, startMS, endMS, limit)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
