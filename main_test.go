package main

import (
	"testing"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/data"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banstrats/examples/fundingrate"
	"github.com/banbox/banstrats/examples/longshort"
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

func TestMainBlankImportRegistersFundingRateSource(t *testing.T) {
	src := data.GetDataSource(fundingrate.SourceName)
	if src == nil {
		t.Fatalf("expected main package imports to register %s", fundingrate.SourceName)
	}
	if src.Info().TimeFrame != fundingrate.DefaultTimeframe {
		t.Fatalf("expected registered timeframe %s, got %s", fundingrate.DefaultTimeframe, src.Info().TimeFrame)
	}
}

func TestMainBlankImportRegistersIdeaStrategy(t *testing.T) {
	stgy := strat.New(&config.RunPolicyConfig{Name: "idea:cl"})
	if stgy == nil {
		t.Fatal("expected main package imports to register idea:cl")
	}
}
