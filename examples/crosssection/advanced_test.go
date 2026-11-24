package crosssection

import (
	"maps"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banstrats/examples/internal/yamlcheck"
)

// loadAdvanced runs one advanced overlay through the real unified loader and the
// factor draft validation, then compiles its factor plan.
func loadAdvanced(t *testing.T, file string) (runner.Config, *factor.Plan, []string) {
	t.Helper()
	spec, err := yamlcheck.Load(t.TempDir(), filepath.Join("advanced", file))
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Config().RunPolicy) != 1 {
		t.Fatalf("%s: expected one run_policy item", file)
	}
	c, err := yamlcheck.Draft(spec, 0)
	if err != nil {
		t.Fatal(err)
	}
	plan, combo, err := runner.CompileDefinition(c)
	if err != nil {
		t.Fatal(err)
	}
	return c, plan, combo.Columns
}

var advancedFiles = []string{"liquid-top-erc.yml", "risk-hrp-exits.yml", "risk-minvar-beta.yml", "neutral-sector-quantile.yml", "regime-gate.yml", "operators.yml"}

func TestAdvancedOverlaysAreComplete(t *testing.T) {
	paths, err := filepath.Glob("advanced/*.yml")
	if err != nil || len(paths) != len(advancedFiles) {
		t.Fatalf("advanced overlays %v do not match the tested list %v (%v)", paths, advancedFiles, err)
	}
	for _, file := range advancedFiles {
		t.Run(file, func(t *testing.T) {
			c, plan, columns := loadAdvanced(t, file)
			if plan.WarmupLength() < 2 || len(columns) == 0 || c.Prices.TimeFrame != "1m" {
				t.Fatalf("incomplete draft: warmup=%d columns=%v prices=%+v", plan.WarmupLength(), columns, c.Prices)
			}
		})
	}
}

// sideVariances gives every asset a distinct volatility so risk-based sizing is observable.
func sideVariances() ([]int32, [][]float64, map[int32]float64) {
	sids := make([]int32, 0, 24)
	vol := map[int32]float64{}
	matrix := make([][]float64, 24)
	for i := range matrix {
		sid := int32(i + 1)
		sids = append(sids, sid)
		vol[sid] = 0.01 * float64(1+sid%5)
		matrix[i] = make([]float64, 24)
		matrix[i][i] = vol[sid] * vol[sid]
	}
	return sids, matrix, vol
}

// proposeFirst builds the policy configured by a YAML overlay and proposes the first target.
func proposeFirst(t *testing.T, c runner.Config) (map[int32]float64, factor.PortfolioProposal) {
	t.Helper()
	pc, err := c.Manifest.Portfolio.PolicyConfig()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := runner.NewPortfolioPolicy(pc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := exampleContext(7200000, 1)
	sids, matrix, vol := sideVariances()
	ctx.Covariance = &factor.CovarianceMatrix{SIDs: sids, Matrix: matrix}
	ctx.Volatility = vol
	ctx.Beta = map[int32]float64{}
	for _, sid := range sids {
		ctx.Beta[sid] = 1
	}
	proposal, err := policy.Propose(ctx, nil)
	if err != nil || proposal.Target == nil {
		t.Fatalf("no target: %v %+v", err, proposal.Reasons)
	}
	weights := map[int32]float64{}
	for sid, allocation := range proposal.Target.Allocations() {
		w, err := strconv.ParseFloat(allocation.Value, 64)
		if err != nil {
			t.Fatal(err)
		}
		weights[sid] = w
	}
	return weights, proposal
}

// Within each side, a lower-volatility asset must receive a larger weight than a
// higher-volatility one under every covariance-based method.
func assertRiskOrdered(t *testing.T, weights map[int32]float64, longK, shortK int) {
	t.Helper()
	_, _, vol := sideVariances()
	check := func(label string, side []int32) {
		if len(side) != longK && len(side) != shortK {
			t.Fatalf("%s side has %d names", label, len(side))
		}
		distinct := map[float64]bool{}
		for _, a := range side {
			distinct[math.Abs(weights[a])] = true
			for _, b := range side {
				if vol[a] < vol[b] && math.Abs(weights[a]) <= math.Abs(weights[b]) {
					t.Fatalf("%s: SID %d (vol %g, w %g) must outweigh SID %d (vol %g, w %g)", label, a, vol[a], weights[a], b, vol[b], weights[b])
				}
			}
		}
		if len(distinct) < 2 {
			t.Fatalf("%s: weights are equal, risk sizing had no effect: %v", label, weights)
		}
	}
	var longs, shorts []int32
	for sid, w := range weights {
		switch {
		case w > 0:
			longs = append(longs, sid)
		case w < 0:
			shorts = append(shorts, sid)
		}
	}
	check("long", longs)
	check("short", shorts)
}

func TestLiquidTopERC(t *testing.T) {
	c, plan, columns := loadAdvanced(t, "liquid-top-erc.yml")
	norm, err := factor.NormalizeRollingUniverseConfig(*c.RollingUniverse)
	if err != nil || norm.Mode != "rolling_liquid_top" || norm.TopN != 30 || norm.LookbackDays != 30 || norm.MinListingDays != 14 || norm.ReconstituteDays != 30 || len(norm.AlwaysInclude) != 2 {
		t.Fatalf("rolling universe %+v: %v", norm, err)
	}
	if !slices.Equal(columns, []string{"risk_adjusted"}) || !slices.Contains(plan.Outputs(), "volatility") {
		t.Fatalf("the reserved volatility column must stay out of the score: columns=%v outputs=%v", columns, plan.Outputs())
	}
	allocation := c.Manifest.Portfolio.Allocation
	if allocation == nil || allocation.Method != "erc" || allocation.RiskWindow != 60 || allocation.Shrinkage != 0.1 || c.Manifest.Portfolio.Rebalance.DriftThreshold != 0.02 {
		t.Fatalf("allocation/drift settings lost: %+v %+v", allocation, c.Manifest.Portfolio.Rebalance)
	}
	weights, _ := proposeFirst(t, c)
	assertRiskOrdered(t, weights, 5, 5)
	long, short := 0.0, 0.0
	for _, w := range weights {
		if w > 0 {
			long += w
		} else {
			short -= w
		}
	}
	if long > 0.5+1e-9 || short > 0.5+1e-9 || long < 0.4 || short < 0.4 {
		t.Fatalf("sides must stay inside the 0.5 budgets minus the reserve: long %g short %g", long, short)
	}
}

func TestHRPWithHybridExits(t *testing.T) {
	c, _, _ := loadAdvanced(t, "risk-hrp-exits.yml")
	holding := c.Manifest.Portfolio.Holding
	if holding == nil || holding.StopLossPct != 0.1 || holding.TrailingStopPct != 0.15 || holding.CooldownBars != 2 {
		t.Fatalf("hybrid exit settings lost: %+v", holding)
	}
	if rule, ok := holding.ByAsset["BTC/USDT:USDT"]; !ok || rule.TrailingStopPct == nil || *rule.TrailingStopPct != 0 {
		t.Fatalf("explicit zero trailing override lost: %+v", holding.ByAsset)
	}
	if allocation := c.Manifest.Portfolio.Allocation; allocation == nil || allocation.Method != "hrp" || allocation.DriftThreshold != 0.03 {
		t.Fatalf("allocation settings lost: %+v", allocation)
	}
	weights, first := proposeFirst(t, c)
	assertRiskOrdered(t, weights, 4, 4)

	// Two bars later (no rebalance is due: every 5 bars) a held long loses 15%: only that
	// asset is closed, through a Patch target, and the other holdings are not touched.
	pc, err := c.Manifest.Portfolio.PolicyConfig()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := runner.NewPortfolioPolicy(pc)
	if err != nil {
		t.Fatal(err)
	}
	held := map[int32]factor.PositionEvidence{}
	for sid, w := range weights {
		if w > 0 {
			held[sid] = factor.PositionEvidence{Quantity: "1", FirstFillTime: 7200001, EntryPrice: "100"}
		}
	}
	quiet := exampleContext(2*7200000, 2)
	quiet.Positions = held
	if p, err := policy.Propose(quiet, first.NextState); err != nil || p.Target != nil {
		t.Fatalf("a quiet off-schedule bar must not trade: %v %+v", err, p.Target)
	}
	crash := exampleContext(3*7200000, 3)
	crash.Positions = held
	victim := slices.Min(slices.Collect(maps.Keys(held)))
	crash.Marks[victim] = 85
	crash.BarClose, crash.BarLow, crash.BarHigh = map[int32]float64{victim: 85}, map[int32]float64{victim: 85}, map[int32]float64{victim: 100}
	p, err := policy.Propose(crash, first.NextState)
	if err != nil || p.Target == nil || p.Target.Spec().Mode != factor.Patch {
		t.Fatalf("stop loss must emit a Patch target: %v %+v", err, p.Target)
	}
	allocations := p.Target.Allocations()
	if len(allocations) != 1 || allocations[victim].Value != "0" {
		t.Fatalf("only the stopped asset may be closed: %+v", allocations)
	}
}

func TestMinVarianceBetaCap(t *testing.T) {
	c, plan, columns := loadAdvanced(t, "risk-minvar-beta.yml")
	allocation := c.Manifest.Portfolio.Allocation
	if allocation == nil || allocation.Method != "min-variance" || allocation.BetaCap != 0.1 || allocation.NetCap != 0.1 {
		t.Fatalf("allocation settings lost: %+v", allocation)
	}
	if !slices.Equal(columns, []string{"momentum"}) || !slices.Contains(plan.Outputs(), "beta") {
		t.Fatalf("beta must be a reserved output, not a score column: %v %v", columns, plan.Outputs())
	}
	if refs := plan.ReferenceSymbols(); !slices.Equal(refs, []string{"BTC/USDT:USDT"}) {
		t.Fatalf("BTC reference stream missing: %v", refs)
	}
	weights, _ := proposeFirst(t, c)
	assertRiskOrdered(t, weights, 5, 5)
	net := 0.0
	for _, w := range weights {
		net += w
	}
	if math.Abs(net) > 0.1+1e-9 {
		t.Fatalf("net_cap violated: %g", net)
	}
}

func TestRegimeGateAndExposureScale(t *testing.T) {
	c, plan, columns := loadAdvanced(t, "regime-gate.yml")
	if !slices.Equal(columns, []string{"score"}) || !slices.Contains(plan.Outputs(), "exposure_scale") {
		t.Fatalf("exposure_scale must be reserved: %v %v", columns, plan.Outputs())
	}
	sources := map[string]bool{}
	for _, input := range plan.Inputs() {
		sources[input.Source+"/"+input.TimeFrame] = true
	}
	if !sources["kline/4h"] || !sources["kline/1d"] || !slices.Equal(plan.ReferenceSymbols(), []string{"BTC/USDT:USDT"}) {
		t.Fatalf("expected 4h kline, native 1d kline and the BTC reference: %v %v", plan.Inputs(), plan.ReferenceSymbols())
	}
	portfolio := c.Manifest.Portfolio
	if portfolio.LongMinScore == nil || *portfolio.LongMinScore != 0.25 || portfolio.ShortMaxScore == nil || *portfolio.ShortMaxScore != -0.25 || portfolio.UnfilledSlotPolicy != "cash" || portfolio.K != 4 {
		t.Fatalf("gate settings lost: %+v", portfolio)
	}
	// Gate semantics with the default top/bottom-K path: only scores beyond the thresholds are
	// selected, each passing name gets notional/K and the unfilled slots stay in cash.
	ctx := exampleContext(7200000, 1)
	scores := map[int32]factor.Numeric{}
	for sid := int32(1); sid <= 24; sid++ {
		scores[sid] = factor.Numeric{Value: 0, Validity: factor.Valid}
	}
	scores[24], scores[23], scores[1] = factor.Numeric{Value: 0.4, Validity: factor.Valid}, factor.Numeric{Value: 0.3, Validity: factor.Valid}, factor.Numeric{Value: -0.4, Validity: factor.Valid}
	ctx.Frame.Values["score"] = scores
	target, _, err := factor.TopBottomKGated(ctx.Frame, "score", ctx.Universe, ctx.Spec, portfolio.K, portfolio.LongNotional, portfolio.ShortNotional,
		factor.ScoreGate{LongMinScore: portfolio.LongMinScore, ShortMaxScore: portfolio.ShortMaxScore, UnfilledSlotPolicy: portfolio.UnfilledSlotPolicy})
	if err != nil || target == nil {
		t.Fatalf("gated selection failed: %v", err)
	}
	got := target.Targets()
	want := map[int32]float64{24: 0.125, 23: 0.125, 1: -0.125}
	if len(got) != len(want) {
		t.Fatalf("expected %v got %v", want, got)
	}
	for sid, w := range want {
		if math.Abs(got[sid]-w) > 1e-12 {
			t.Fatalf("SID %d: expected %g got %g (%v)", sid, w, got[sid], got)
		}
	}
	// Nothing passes the gate: the strategy goes flat instead of forcing positions.
	for sid := range scores {
		scores[sid] = factor.Numeric{Value: 0, Validity: factor.Valid}
	}
	flat, _, err := factor.TopBottomKGated(ctx.Frame, "score", ctx.Universe, ctx.Spec, portfolio.K, portfolio.LongNotional, portfolio.ShortNotional,
		factor.ScoreGate{LongMinScore: portfolio.LongMinScore, ShortMaxScore: portfolio.ShortMaxScore, UnfilledSlotPolicy: portfolio.UnfilledSlotPolicy})
	if err != nil || flat == nil || len(flat.Targets()) != 0 {
		t.Fatalf("expected an explicit flat target: %v %v", err, flat)
	}
}

// panel builds a deterministic daily kline panel for named assets.
type panel struct {
	names map[int32]string
	sids  []int32
}

func newPanel(symbols ...string) panel {
	p := panel{names: map[int32]string{}}
	for i, symbol := range symbols {
		sid := int32(i + 1)
		p.names[sid] = symbol
		p.sids = append(p.sids, sid)
	}
	return p
}

func (p panel) snapshot(t *testing.T, bar int, rows map[int32]map[string]any) *factor.Snapshot {
	t.Helper()
	const day = int64(86400000)
	event := int64(bar+1) * day
	var records []factor.VersionRecord
	var required []factor.Requirement
	for sid, fields := range rows {
		records = append(records, factor.Record(orm.DataSeries{Source: "kline", Sid: sid, TimeMS: event - day, EndMS: event, TimeFrame: "1d", Closed: true, Values: fields}, 1, event, event, "v1"))
		required = append(required, factor.Requirement{SID: sid, Source: "kline", TimeFrame: "1d", EventTime: event})
	}
	snap, err := factor.Freeze(factor.SnapshotSpec{DecisionTime: event, ReplayTime: event,
		Universe: factor.Universe{Version: "u1", Investable: p.sids, Reference: p.sids, Tradable: p.sids, Evaluation: p.sids, Static: true},
		SIDMap:   p.names, Schemas: map[string]string{"kline": "schema-v1"}, SourceVersions: map[string]string{"kline": "v1"}, VisibilityPolicy: "published-and-received"}, records, required)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func (p panel) run(t *testing.T, plan *factor.Plan, bars int) factor.Frame {
	t.Helper()
	session, err := factor.NewSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	var frame factor.Frame
	for bar := 0; bar < bars; bar++ {
		rows := map[int32]map[string]any{}
		for i, sid := range p.sids {
			f := float64(bar)
			price := 100 * math.Exp(0.002*float64(i-len(p.sids)/2)*f+0.03*math.Sin(0.21*f*float64(i+1)))
			volume := 1000 * (1 + 0.5*math.Sin(0.4*f+float64(i)) + 0.1*float64(i))
			rows[sid] = map[string]any{"close": price, "volume": volume, "quote": price * volume}
		}
		frame, err = session.Evaluate(p.snapshot(t, bar, rows))
		if err != nil {
			t.Fatal(err)
		}
	}
	return frame
}

func TestSectorAndQuantileNeutralization(t *testing.T) {
	_, plan, columns := loadAdvanced(t, "neutral-sector-quantile.yml")
	if !slices.Equal(columns, []string{"size_momentum", "sector_momentum"}) && !slices.Equal(columns, []string{"sector_momentum", "size_momentum"}) {
		t.Fatalf("combination columns %v", columns)
	}
	// Two names per built-in sector so every group has a non-trivial demeaned value.
	p := newPanel("BTC/USDT:USDT", "ETH/USDT:USDT", "SOL/USDT:USDT", "AVAX/USDT:USDT", "ARB/USDT:USDT", "OP/USDT:USDT", "UNI/USDT:USDT", "AAVE/USDT:USDT", "DOGE/USDT:USDT", "1000PEPE/USDT:USDT", "FET/USDT:USDT", "RNDR/USDT:USDT")
	frame := p.run(t, plan, 60)
	groups := map[string]float64{}
	members := map[string]int{}
	total := 0.0
	for sid, symbol := range p.names {
		sector, size := frame.Values["sector_momentum"][sid], frame.Values["size_momentum"][sid]
		if sector.Validity != factor.Valid || size.Validity != factor.Valid {
			t.Fatalf("%s: invalid neutralized value %+v %+v", symbol, sector, size)
		}
		group := factor.CryptoSector(symbol)
		groups[group] += sector.Value
		members[group]++
		total += size.Value
	}
	if len(groups) < 3 {
		t.Fatalf("expected several sectors, got %v", members)
	}
	for group, sum := range groups {
		if math.Abs(sum) > 1e-8 {
			t.Fatalf("sector %s is not neutral: sum %g over %d names", group, sum, members[group])
		}
	}
	// Every liquidity tercile is standardised, so the whole cross-section also sums to zero.
	if math.Abs(total) > 1e-8 {
		t.Fatalf("quantile buckets are not neutral: total %g", total)
	}
}

func TestOperatorTour(t *testing.T) {
	_, plan, columns := loadAdvanced(t, "operators.yml")
	if len(columns) != 4 {
		t.Fatalf("columns %v", columns)
	}
	// The CS -> TS -> CS chain accumulates warmup: a 30-bar return, then a 5-bar average on top.
	if plan.WarmupLength() < 34 {
		t.Fatalf("multi-stage warmup was not accumulated: %d", plan.WarmupLength())
	}
	stages := map[factor.StageKind]bool{}
	for _, stage := range plan.Stages() {
		stages[stage.Kind] = true
	}
	if !stages[factor.StageCS] || !stages[factor.StageTS] {
		t.Fatalf("expected alternating TS and CS stages, got %v", plan.Stages())
	}
	p := newPanel("A/USDT:USDT", "B/USDT:USDT", "C/USDT:USDT", "D/USDT:USDT", "E/USDT:USDT", "F/USDT:USDT")
	frame := p.run(t, plan, 80)
	for _, column := range columns {
		sum := 0.0
		for _, sid := range p.sids {
			value := frame.Values[column][sid]
			if value.Validity != factor.Valid {
				t.Fatalf("%s SID %d: %+v", column, sid, value)
			}
			sum += value.Value
		}
		// Standardised columns are centred; the volume gate rescales some names, so skip that one.
		if column != "volume_confirmed_trend" && math.Abs(sum) > 1e-8 {
			t.Fatalf("%s is not centred: %g", column, sum)
		}
	}
	// Warmup: before it is filled the score must not be valid.
	session, err := factor.NewSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	early, err := session.Evaluate(p.snapshot(t, 0, map[int32]map[string]any{1: {"close": 100.0, "volume": 1.0}, 2: {"close": 101.0, "volume": 1.0}, 3: {"close": 102.0, "volume": 1.0}, 4: {"close": 103.0, "volume": 1.0}, 5: {"close": 104.0, "volume": 1.0}, 6: {"close": 105.0, "volume": 1.0}}))
	if err != nil {
		t.Fatal(err)
	}
	if early.Values["smooth_rank"][1].Validity == factor.Valid {
		t.Fatal("score emitted during warmup")
	}
}
