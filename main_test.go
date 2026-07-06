package main

import (
	"testing"

	"github.com/banbox/banbot/data"
	"github.com/banbox/banstrats/longshort"
)

func TestMainBlankImportRegistersBinanceLongShortSource(t *testing.T) {
	src := data.GetDataSource(longshort.SourceName)
	if src == nil {
		t.Fatalf("expected main package imports to register %s", longshort.SourceName)
	}
	if src.Info().TimeFrame != longshort.DefaultTimeframe {
		t.Fatalf("expected registered timeframe %s, got %s", longshort.DefaultTimeframe, src.Info().TimeFrame)
	}
}
