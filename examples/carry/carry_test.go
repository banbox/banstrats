package carry

import (
	"math"
	"testing"

	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banstrats/examples/internal/yamlcheck"
)

const (
	hourMS   = int64(3600000)
	btcSpot  = "BTC/USDT"
	btcPerp  = "BTC/USDT:USDT"
	ethSpot  = "ETH/USDT"
	ethPerp  = "ETH/USDT:USDT"
	spotSID  = int32(1)
	perpSID  = int32(2)
	ethSpotS = int32(3)
	ethPerpS = int32(4)
)

func config(params map[string]float64) runner.Config {
	sidMap := map[int32]string{spotSID: btcSpot, perpSID: btcPerp, ethSpotS: ethSpot, ethPerpS: ethPerp}
	sids := []int32{spotSID, perpSID, ethSpotS, ethPerpS}
	return runner.Config{
		Definition: Name,
		Factor:     research.MomentumVolConfig{Source: "kline", Field: "close", TimeFrame: "1h"},
		Manifest:   research.ManifestSpec{Parameters: params},
		SpotSIDs:   []int32{spotSID, ethSpotS},
		Snapshot: factor.SnapshotSpec{
			Universe: factor.Universe{Version: "u1", Investable: sids, Reference: sids, Tradable: sids, Evaluation: sids, Static: true},
			SIDMap:   sidMap,
		},
	}
}

func feed(t *testing.T, b runner.PortfolioBuilder, column string, seq uint64, logp map[int32]float64) map[int32]float64 {
	t.Helper()
	values := map[int32]factor.Numeric{}
	tradable := make([]int32, 0, len(logp))
	for sid, v := range logp {
		values[sid] = factor.Numeric{Value: v, Validity: factor.Valid}
		tradable = append(tradable, sid)
	}
	at := int64(seq) * hourMS
	spec := factor.PortfolioSpec{
		StrategyID: "s", AccountID: "a", SnapshotID: "snap", PlanHash: "p", FactorPlanHash: "f", UniverseVersion: "u",
		Budget: factor.FrozenBudget{Version: "b", Currency: "USDT", NAV: 10000}, PlanSequence: seq,
		DecisionTime: at, ExecutableAt: at + 1, ExpireAt: at + 60000, Mode: factor.Full,
	}
	frame := factor.Frame{DecisionTime: at, Values: map[string]map[int32]factor.Numeric{column: values}}
	target, _, err := b(frame, factor.Universe{Version: "u", Investable: tradable, Tradable: tradable}, spec, research.PortfolioDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if target == nil {
		return nil
	}
	return target.Targets()
}

func TestOverlay(t *testing.T) {
	spec, err := yamlcheck.Load(t.TempDir(), "carry.yml")
	if err != nil {
		t.Fatal(err)
	}
	u := spec.Config()
	if u.Root.MarketType != "linear" || len(u.Root.ExtraMarkets) != 1 || u.Root.ExtraMarkets[0] != "spot" {
		t.Fatalf("multi-market declaration lost: %q %v", u.Root.MarketType, u.Root.ExtraMarkets)
	}
	c, err := yamlcheck.Draft(spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Manifest.Costs.FundingPolicy != "storage" || c.FundingSource == "" {
		t.Fatalf("funding settings lost: %+v", c.Manifest.Costs)
	}
	prepared := config(c.Manifest.Parameters)
	prepared.Manifest = c.Manifest
	if _, err := runner.PreparePortfolioConfig(prepared); err != nil {
		t.Fatal(err)
	}
	for _, symbol := range u.Root.Pairs {
		found := false
		for _, name := range config(nil).Snapshot.SIDMap {
			found = found || name == symbol
		}
		if !found {
			t.Fatalf("overlay pool symbol %s is not one of the strategy legs", symbol)
		}
	}
}

func TestRejectsInvalidParameters(t *testing.T) {
	for _, bad := range []map[string]float64{{"typo": 1}, {"entry": 1, "exit": 1}, {"gross": 0.6}, {"window": 2}, {"maxHold": -1}} {
		if _, err := runner.PreparePortfolioConfig(config(bad)); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	// Both legs of every pair must be in the pool.
	c := config(nil)
	delete(c.Snapshot.SIDMap, ethSpotS)
	if _, err := runner.PreparePortfolioConfig(c); err == nil {
		t.Fatal("accepted a pool without the ETH spot leg")
	}
}

// A rich perpetual basis opens short perp / long spot; a perpetual discount never
// produces a position because that would require shorting spot.
func TestShortsRichBasisAndNeverShortsSpot(t *testing.T) {
	params := map[string]float64{"window": 20, "entry": 2, "exit": 0.5, "stop": 0, "maxHold": 0, "gross": 0.5, "driftBand": 0}
	plan, combo, err := runner.CompileDefinition(config(params))
	if err != nil || len(plan.Outputs()) == 0 {
		t.Fatal(err)
	}
	run := func(shock float64) map[int32]float64 {
		// Builder state is private to one run, so every scenario prepares its own.
		builder, err := runner.PreparePortfolioConfig(config(params))
		if err != nil {
			t.Fatal(err)
		}
		var weights map[int32]float64
		for i := 0; i < 25; i++ {
			f := float64(i)
			spot := 10.8 + 0.05*math.Sin(0.7*f) + 0.02*math.Cos(0.23*f)
			basis := 0.001 + 0.0004*math.Sin(1.9*f)
			if i == 24 {
				basis += shock
			}
			weights = feed(t, builder.PortfolioBuilder, combo.Columns[0], uint64(i+1), map[int32]float64{
				spotSID: spot, perpSID: spot + basis,
				ethSpotS: spot - 1, ethPerpS: spot - 1 + 0.001 + 0.0004*math.Cos(1.3*f),
			})
		}
		return weights
	}
	rich := run(0.01)
	if rich[perpSID] >= 0 || rich[spotSID] <= 0 || math.Abs(rich[perpSID]+rich[spotSID]) > 1e-9 || math.Abs(rich[perpSID])+math.Abs(rich[spotSID]) > 0.5+1e-9 {
		t.Fatalf("expected dollar-neutral short perp / long spot within 0.5 gross, got %v", rich)
	}
	for sid, w := range run(-0.01) {
		if (sid == spotSID || sid == ethSpotS) && w < 0 || w != 0 && (sid == perpSID || sid == ethPerpS) {
			t.Fatalf("discount basis must not open a position, got %d=%v", sid, w)
		}
	}
}
