package main

import (
	"testing"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/data"
	"github.com/banbox/banbot/entry"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/strat"
	"github.com/banbox/banstrats/examples/fundingrate"
	"github.com/banbox/banstrats/examples/longshort"
)

func TestMainUnifiedCommandTree(t *testing.T) {
	root := entry.NewRootCommand()
	for _, tc := range []struct {
		path  []string
		flags []string
	}{
		{[]string{"backtest"}, []string{"config", "mode", "no-default", "datadir"}},
		{[]string{"trade"}, []string{"config", "dry-run", "live-provider"}},
		{[]string{"research"}, []string{"config", "no-default", "datadir"}},
		{[]string{"validate"}, []string{"spec"}},
		{[]string{"explain"}, []string{"spec"}},
		{[]string{"data", "archive"}, []string{"input", "schema", "out"}},
	} {
		command, remaining, err := root.Find(tc.path)
		if err != nil || len(remaining) != 0 || command.Name() != tc.path[len(tc.path)-1] {
			t.Fatalf("command %v is unavailable: %v", tc.path, err)
		}
		for _, flag := range tc.flags {
			if command.Flags().Lookup(flag) == nil {
				t.Fatalf("command %v is missing --%s", tc.path, flag)
			}
		}
	}
	for _, command := range root.Commands() {
		if command.Name() == "factor" {
			t.Fatal("obsolete factor command group is still registered")
		}
	}
}

func TestMainRegistersCrossSectionDefinitions(t *testing.T) {
	// Keep expected names here so this test depends on main.go's registration import.
	for _, name := range []string{
		"CSMomentum", "CSReversal", "CSLowVol", "CSTrend", "CSVolumeMomentum", "CSLiquidityNeutralMomentum", "CSMultiFactor",
		"CSMomentumExpr", "CSReversalExpr", "CSLowVolExpr", "CSTrendExpr", "CSVolumeMomentumExpr", "CSMultiFactorExpr",
	} {
		t.Run(name, func(t *testing.T) {
			cfg := runner.Config{Definition: name, Factor: research.MomentumVolConfig{Source: "kline", Field: "close", TimeFrame: "1d"}}
			if _, _, err := runner.CompileDefinition(cfg); err != nil {
				t.Fatalf("factor definition %s was not registered: %v", name, err)
			}
		})
	}
}

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

func TestMainRegistersAllStrategies(t *testing.T) {
	for _, name := range []string{
		"ma:dca", "ma:openClose", "ma:demo", "ma:demo_er", "ma:demo2",
		"ma:demo_batch", "ma:demo_exit", "ma:edit_pairs", "ma:trail_stop", "ma:ws", "ma:postApi",
		"grid:inv", "idea:cl", "rpc_ai:trade1",
		"tmp:limit", "tmp:demo", "tmp:chg_pair", "tmp:llm_run", "tmp:trigger",
		"fundingrate:funding_rate_demo", "longshort:binance_ls_example",
	} {
		t.Run(name, func(t *testing.T) {
			if stgy := strat.New(&config.RunPolicyConfig{Name: name}); stgy == nil {
				t.Fatalf("strategy %s was not constructed", name)
			}
		})
	}
}
