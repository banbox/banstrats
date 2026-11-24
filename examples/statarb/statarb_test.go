package statarb

import (
	"math"
	"slices"
	"testing"

	botconfig "github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banstrats/examples/internal/yamlcheck"
)

const (
	hourMS = int64(3600000)
	btc    = "BTC/USDT:USDT"
	eth    = "ETH/USDT:USDT"
	sol    = "SOL/USDT:USDT"
)

func baseConfig(name string, params map[string]float64, symbols ...string) runner.Config {
	sids := make([]int32, len(symbols))
	sidMap := map[int32]string{}
	for i, symbol := range symbols {
		sids[i] = int32(i + 1)
		sidMap[sids[i]] = symbol
	}
	return runner.Config{
		Definition: name,
		Factor:     research.MomentumVolConfig{Source: "kline", Field: "close", TimeFrame: "1h"},
		Manifest:   research.ManifestSpec{Parameters: params},
		Snapshot: factor.SnapshotSpec{
			Universe: factor.Universe{Version: "u1", Investable: sids, Reference: sids, Tradable: sids, Evaluation: sids, Static: true},
			SIDMap:   sidMap,
		},
	}
}

func sidOf(c runner.Config, symbol string) int32 {
	for sid, name := range c.Snapshot.SIDMap {
		if name == symbol {
			return sid
		}
	}
	return 0
}

// feed pushes one decision bar of log prices through a portfolio builder.
func feed(t *testing.T, builder runner.PortfolioBuilder, output string, seq uint64, nav float64, logp map[int32]float64, tradable []int32) (map[int32]float64, []factor.Diagnostic) {
	t.Helper()
	values := map[int32]factor.Numeric{}
	for sid, v := range logp {
		values[sid] = factor.Numeric{Value: v, Validity: factor.Valid}
	}
	at := int64(seq) * hourMS
	frame := factor.Frame{DecisionTime: at, Values: map[string]map[int32]factor.Numeric{output: values}}
	spec := factor.PortfolioSpec{
		StrategyID: "s", AccountID: "a", SnapshotID: "snap", PlanHash: "p", FactorPlanHash: "f", UniverseVersion: "u",
		Budget: factor.FrozenBudget{Version: "b", Currency: "USDT", NAV: nav}, PlanSequence: seq,
		DecisionTime: at, ExecutableAt: at + 1, ExpireAt: at + 60000, Mode: factor.Full,
	}
	target, diags, err := builder(frame, factor.Universe{Version: "u", Investable: tradable, Tradable: tradable}, spec, research.PortfolioDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if target == nil {
		return nil, diags
	}
	return target.Targets(), target.Spec().Diagnostics
}

func outputName(t *testing.T, c runner.Config) string {
	t.Helper()
	plan, combo, err := runner.CompileDefinition(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Outputs()) < 1 || len(combo.Columns) != 1 {
		t.Fatalf("unexpected plan outputs %v columns %v", plan.Outputs(), combo.Columns)
	}
	for _, input := range plan.Inputs() {
		if input.Source != "kline" || input.TimeFrame != "1h" {
			t.Fatalf("unexpected subscription %+v", input)
		}
	}
	return combo.Columns[0]
}

func TestRegisteredDefinitionsCompile(t *testing.T) {
	for _, name := range Names() {
		c := baseConfig(name, nil, btc, eth)
		if outputName(t, c) == "" {
			t.Fatalf("%s: no output", name)
		}
		for _, bad := range []map[string]float64{{"typo": 1}, {"entry": math.NaN()}} {
			c := baseConfig(name, bad, btc, eth)
			if _, _, err := runner.CompileDefinition(c); err == nil {
				t.Fatalf("%s accepted invalid params %v", name, bad)
			}
		}
	}
	n := Names()
	n[0] = "changed"
	if Names()[0] != NamePairs {
		t.Fatal("Names must return a copy")
	}
}

func TestOverlays(t *testing.T) {
	cases := []struct {
		file, name string
		rolling    bool
	}{
		{"pairs.yml", NamePairs, false},
		{"rolling-pairs.yml", NameRollingPairs, false},
		{"basket.yml", NameBasket, false},
		{"residual-pca.yml", NameResidualPCA, true},
		{"residual-benchmark.yml", NameResidualBench, false},
	}
	covered := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			spec, err := yamlcheck.Load(t.TempDir(), tc.file)
			if err != nil {
				t.Fatal(err)
			}
			u := spec.Config()
			if len(u.RunPolicy) != 1 || u.RunPolicy[0].Name != tc.name || u.RunPolicy[0].Engine != botconfig.EngineFactor {
				t.Fatalf("unexpected policy %+v", u.RunPolicy)
			}
			c, err := yamlcheck.Draft(spec, 0)
			if err != nil {
				t.Fatal(err)
			}
			covered[tc.name] = true
			if c.Prices.Source != "kline" || c.Prices.TimeFrame != "1m" || c.Manifest.Portfolio.Mode != factor.Full {
				t.Fatalf("execution settings lost: prices=%+v mode=%s", c.Prices, c.Manifest.Portfolio.Mode)
			}
			if (c.RollingUniverse != nil) != tc.rolling {
				t.Fatalf("rolling universe presence = %v", c.RollingUniverse != nil)
			}
			// Build the real private per-run state against the overlay's own symbol pool.
			pool := u.Root.Pairs
			prepared := baseConfig(tc.name, c.Manifest.Parameters, pool...)
			prepared.Manifest = c.Manifest
			prepared.RollingUniverse = c.RollingUniverse
			prepared.Factor.TimeFrame = c.Factor.TimeFrame
			prepared, err = runner.PreparePortfolioConfig(prepared)
			if err != nil || prepared.PortfolioBuilder == nil {
				t.Fatalf("builder construction failed: %v", err)
			}
			if tc.rolling {
				norm, err := factor.NormalizeRollingUniverseConfig(*c.RollingUniverse)
				if err != nil || norm.TopN != 20 || norm.Mode != "rolling_liquid_top" {
					t.Fatalf("rolling universe %+v: %v", norm, err)
				}
				if _, err := factor.NewRollingUniverseSelector(norm, prepared.Snapshot.Universe, prepared.Snapshot.SIDMap, c.DecisionInterval, "kline", "1h"); err != nil {
					t.Fatal(err)
				}
				if _, err := factor.ResolveAlwaysIncludeSIDs(norm.AlwaysInclude, prepared.Snapshot.Universe.Investable, prepared.Snapshot.SIDMap); err != nil {
					t.Fatal(err)
				}
			}
			// A misspelled parameter must fail instead of silently using the default.
			typo := prepared
			typo.PortfolioBuilder = nil
			typo.Manifest.Parameters = map[string]float64{"entrey": 2}
			if _, err := runner.PreparePortfolioConfig(typo); err == nil {
				t.Fatal("misspelled parameter accepted")
			}
		})
	}
	for _, name := range Names() {
		if !covered[name] {
			t.Fatalf("no overlay validates %s", name)
		}
	}
}

func TestPairsRejectInvalidSettings(t *testing.T) {
	for _, bad := range []map[string]float64{
		{"entry": 1, "exit": 1}, // exit must be below entry
		{"stop": 1.5},           // stop must exceed entry
		{"window": 2},           // window >= 3
		{"gross": 0.6},          // two pairs would exceed 100% NAV
		{"minBeta": -0.1},       // non-negative
		{"driftBand": -0.01},    // non-negative
		{"maxHalfLife": 5, "minHalfLife": 8},
		{"maxHurst": 1.5},               // within [0,1]
		{"inverseVol": 2},               // booleans are 0 or 1
		{"kalman": 1, "kalmanDelta": 1}, // delta in (0,1)
	} {
		c := baseConfig(NamePairs, bad, btc, eth, sol)
		if _, err := runner.PreparePortfolioConfig(c); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	// Legs absent from the pool are refused when the builder is constructed.
	if _, err := runner.PreparePortfolioConfig(baseConfig(NamePairs, nil, btc, eth)); err == nil {
		t.Fatal("accepted a pool without the SOL leg")
	}
	// Alternative spread model and dynamic hedge construct cleanly.
	c := baseConfig(NamePairs, map[string]float64{"kalman": 1, "kalmanDelta": 0.0001, "kalmanVe": 0.001, "dynamicHedge": 1, "dynamicExitZ": 1}, btc, eth, sol)
	if _, err := runner.PreparePortfolioConfig(c); err != nil {
		t.Fatal(err)
	}
}

func TestVolatilityGate(t *testing.T) {
	gate := volatilityGate(0.02)
	ctx := func(y, x float64) runner.PairFilterContext {
		return runner.PairFilterContext{FeaturesY: map[string]float64{legVolFeature: y}, FeaturesX: map[string]float64{legVolFeature: x}}
	}
	if !gate(ctx(0.01, 0.02)) || gate(ctx(0.03, 0.01)) || gate(ctx(0.01, 0.03)) {
		t.Fatal("gate must pass calm legs only")
	}
	if gate(runner.PairFilterContext{}) || gate(runner.PairFilterContext{FeaturesY: map[string]float64{legVolFeature: math.NaN()}, FeaturesX: map[string]float64{legVolFeature: 0.01}}) {
		t.Fatal("gate must block while the feature is unavailable")
	}
	// The feature expression is part of the compiled plan only when the gate is on.
	with := baseConfig(NamePairs, nil, btc, eth, sol)
	without := baseConfig(NamePairs, map[string]float64{"maxLegVol": 0}, btc, eth, sol)
	planWith, _, err := runner.CompileDefinition(with)
	if err != nil {
		t.Fatal(err)
	}
	planWithout, _, err := runner.CompileDefinition(without)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(planWith.Outputs(), "feat:"+legVolFeature) || slices.Contains(planWithout.Outputs(), "feat:"+legVolFeature) {
		t.Fatalf("feature outputs: %v / %v", planWith.Outputs(), planWithout.Outputs())
	}
	if planWith.WarmupLength() <= planWithout.WarmupLength() {
		t.Fatal("volatility feature must extend the warmup")
	}
}

// ETH jumps while BTC and SOL stay on their regression lines: the ETH/BTC pair
// opens a short spread (sell ETH, buy BTC) and the SOL/ETH pair a long spread
// (buy SOL, sell ETH). Positions net per symbol. The loose settings switch the
// optional gates off.
func TestPairsTradeSyntheticDislocation(t *testing.T) {
	params := map[string]float64{"window": 20, "entry": 2, "exit": 0.5, "stop": 0, "minBeta": 0, "inverseVol": 0, "driftBand": 0,
		"minCorr": 0, "maxHurst": 0, "maxHalfLife": 0, "maxLegVol": 0, "gross": 0.5}
	c := baseConfig(NamePairs, params, btc, eth, sol)
	column := outputName(t, c)
	c, err := runner.PreparePortfolioConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	sids := []int32{sidOf(c, btc), sidOf(c, eth), sidOf(c, sol)}
	var weights map[int32]float64
	for i := 0; i < 25; i++ {
		f := float64(i)
		x := 4 + 0.05*math.Sin(0.9*f) + 0.03*math.Cos(0.37*f)
		y := 0.2 + x + 0.002*math.Sin(2.3*f)
		if i == 24 {
			y += 0.1
		}
		z := 0.5 + 0.7*x + 0.001*math.Cos(f)
		weights, _ = feed(t, c.PortfolioBuilder, column, uint64(i+1), 10000, map[int32]float64{sids[0]: x, sids[1]: y, sids[2]: z}, sids)
		if i < 19 && len(weights) != 0 {
			t.Fatalf("bar %d traded before the window was full: %v", i, weights)
		}
	}
	wEth, wBtc, wSol := weights[sidOf(c, eth)], weights[sidOf(c, btc)], weights[sidOf(c, sol)]
	gross := math.Abs(wEth) + math.Abs(wBtc) + math.Abs(wSol)
	if wEth >= 0 || wBtc <= 0 || wSol <= 0 || gross > 1+1e-9 {
		t.Fatalf("expected short ETH, long BTC and long SOL, got %v", weights)
	}
}

func TestBasketsAndResidualRegistrations(t *testing.T) {
	pool := []string{btc, eth, "BNB/USDT:USDT", sol, "AVAX/USDT:USDT", "NEAR/USDT:USDT", "XRP/USDT:USDT", "DOGE/USDT:USDT"}
	for name, build := range map[string]func(*runner.Params) (runner.BasketsSpec, error){NameBasket: basketSpec} {
		reg, err := runner.BuildBasketsParamsRegistration(name+"-test", build)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []map[string]float64{{"confidence": 0.5}, {"window": 5}, {"entry": 1, "exit": 2}, {"maxGross": 2}, {"typo": 1}} {
			c := baseConfig(name, bad, pool...)
			if _, err := reg.Factory(c); err == nil {
				t.Fatalf("%s accepted %v", name, bad)
			}
		}
		for _, ok := range []map[string]float64{nil, {"ols": 1, "rawPrice": 1}, {"requireCoint": 0, "dynamicWeights": 1, "confidence": 0.99}} {
			if _, err := reg.Factory(baseConfig(name, ok, pool...)); err != nil {
				t.Fatalf("%s rejected %v: %v", name, ok, err)
			}
		}
		// A basket with a leg outside the pool cannot be constructed.
		if _, err := reg.Factory(baseConfig(name, nil, btc, eth)); err == nil {
			t.Fatal("accepted a pool without the basket legs")
		}
	}
	for _, build := range []func(*runner.Params) (runner.ResidualStatArbSpec, error){residualPCASpec, residualBenchSpec} {
		reg, err := runner.BuildResidualStatArbParamsRegistration("residual-test", build)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []map[string]float64{{"window": 4}, {"entryS": 0.2}, {"stopS": 1}, {"maxWeight": 2}, {"ouWindow": 100}, {"typo": 1}} {
			if _, err := reg.Factory(baseConfig("residual-test", bad, pool...)); err == nil {
				t.Fatalf("accepted %v", bad)
			}
		}
		if _, err := reg.Factory(baseConfig("residual-test", map[string]float64{"inverseSigma": 0, "ouWindow": 30}, pool...)); err != nil {
			t.Fatal(err)
		}
	}
	// The benchmark variant needs BTC and ETH in the run's pool.
	if _, err := runner.PreparePortfolioConfig(baseConfig(NameResidualBench, nil, sol, "BNB/USDT:USDT")); err == nil {
		t.Fatal("benchmark model accepted a pool without its benchmarks")
	}
}

// The residual book must stay dollar neutral whatever the factor model.
func TestResidualBooksAreDollarNeutral(t *testing.T) {
	pool := []string{btc, eth, "BNB/USDT:USDT", sol, "AVAX/USDT:USDT", "NEAR/USDT:USDT", "XRP/USDT:USDT", "DOGE/USDT:USDT", "ADA/USDT:USDT", "LINK/USDT:USDT"}
	for _, name := range []string{NameResidualPCA, NameResidualBench} {
		c := baseConfig(name, map[string]float64{"window": 30, "entryS": 0.6, "exitS": 0.2, "stopS": 0, "maxHalfLife": 0, "maxWeight": 0, "driftBand": 0}, pool...)
		column := outputName(t, c)
		c, err := runner.PreparePortfolioConfig(c)
		if err != nil {
			t.Fatal(err)
		}
		sids := c.Snapshot.Universe.Tradable
		traded := false
		level := make([]float64, len(sids))
		for i := range level {
			level[i] = 3 + 0.1*float64(i)
		}
		for bar := 0; bar < 120; bar++ {
			f := float64(bar)
			market := 0.01 * math.Sin(0.31*f)
			logp := map[int32]float64{}
			for i, sid := range sids {
				// Common market move, a mean-reverting idiosyncratic wave and a slow drift per asset.
				level[i] += market*(0.5+0.1*float64(i)) + 0.004*(math.Sin(0.77*f*float64(i+1))-math.Sin(0.77*(f-1)*float64(i+1)))
				logp[sid] = level[i]
			}
			weights, _ := feed(t, c.PortfolioBuilder, column, uint64(bar+1), 10000, logp, sids)
			sum, gross := 0.0, 0.0
			for _, w := range weights {
				sum += w
				gross += math.Abs(w)
			}
			if math.Abs(sum) > 1e-8 || gross > 1+1e-9 {
				t.Fatalf("%s bar %d: net %g gross %g", name, bar, sum, gross)
			}
			traded = traded || len(weights) > 0
		}
		if !traded {
			t.Fatalf("%s never traded on the synthetic panel", name)
		}
	}
}

func TestCoreVersionIsLinked(t *testing.T) {
	if core.Version == "" {
		t.Fatal("banbot core version missing")
	}
}
