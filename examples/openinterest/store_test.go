package openinterest

import (
	"testing"

	"github.com/banbox/banbot/orm"
)

func TestOpenInterestInfoBindsKlineTable(t *testing.T) {
	if Info.Name != SourceName {
		t.Fatalf("expected source %s, got %s", SourceName, Info.Name)
	}
	if Info.Binding.Table != orm.SeriesTableName(orm.SeriesSourceKline, DefaultTimeframe) {
		t.Fatalf("expected kline table binding, got %s", Info.Binding.Table)
	}
	if Info.Binding.EndColumn != "" {
		t.Fatalf("kline series should not define end column, got %s", Info.Binding.EndColumn)
	}
	want := []string{FieldValue, FieldOpenInterestValue, FieldCirculatingSupply}
	if len(Info.Binding.Fields) != len(want) {
		t.Fatalf("unexpected fields: %+v", Info.Binding.Fields)
	}
	for idx, name := range want {
		if Info.Binding.Fields[idx].Name != name {
			t.Fatalf("expected field %d to be %s, got %+v", idx, name, Info.Binding.Fields)
		}
	}
}
