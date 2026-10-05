package crosssection

import (
	"fmt"
	"math"
	"testing"

	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banbot/orm"
)

func config(name string, params map[string]float64) runner.Config {
	return runner.Config{Definition: name, Factor: research.MomentumVolConfig{Source: "kline", Field: "close", TimeFrame: "1d"}, Manifest: research.ManifestSpec{Parameters: params}}
}

func snapshot(t *testing.T, bar int, values map[int32]map[string]any) *factor.Snapshot {
	t.Helper()
	event := int64(bar+1) * 86400000
	var sids []int32
	var rows []factor.VersionRecord
	var required []factor.Requirement
	sidMap := map[int32]string{}
	for sid, fields := range values {
		sids = append(sids, sid)
		sidMap[sid] = fmt.Sprintf("ASSET%d", sid)
		rows = append(rows, factor.Record(orm.DataSeries{Source: "kline", Sid: sid, TimeMS: event - 86400000, EndMS: event, TimeFrame: "1d", Closed: true, Values: fields}, 1, event, event, "v1"))
		required = append(required, factor.Requirement{SID: sid, Source: "kline", TimeFrame: "1d", EventTime: event})
	}
	snap, err := factor.Freeze(factor.SnapshotSpec{DecisionTime: event, ReplayTime: event, Universe: factor.Universe{Version: "u1", Investable: sids, Reference: sids, Tradable: sids, Evaluation: sids, Static: true}, SIDMap: sidMap, Schemas: map[string]string{"kline": "schema-v1"}, SourceVersions: map[string]string{"kline": "v1"}, VisibilityPolicy: "published-and-received"}, rows, required)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func evaluate(t *testing.T, name string, params map[string]float64, bars int, data func(int) map[int32]map[string]any) (factor.Frame, map[int32]factor.Numeric) {
	t.Helper()
	plan, combo, err := runner.CompileDefinition(config(name, params))
	if err != nil {
		t.Fatal(err)
	}
	session, err := factor.NewSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	var frame factor.Frame
	var snap *factor.Snapshot
	for i := 0; i < bars; i++ {
		snap = snapshot(t, i, data(i))
		frame, err = session.Evaluate(snap)
		if err != nil {
			t.Fatal(err)
		}
		if i < plan.WarmupLength() {
			scores, _, err := research.Combine(frame, snap.Spec().Universe, combo, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, score := range scores {
				if score.Validity == factor.Valid {
					t.Fatalf("%s emitted score during warmup at %d", name, i)
				}
			}
		}
	}
	scores, _, err := research.Combine(frame, snap.Spec().Universe, combo, nil)
	if err != nil {
		t.Fatal(err)
	}
	return frame, scores
}

func TestRegistrationDefaultsAndParameters(t *testing.T) {
	warmups := []int{21, 3, 20, 19, 21, 21, 21}
	for i, name := range Names() {
		plan, combo, err := runner.CompileDefinition(config(name, nil))
		if err != nil {
			t.Fatal(err)
		}
		if plan.WarmupLength() != warmups[i] || len(combo.Columns) == 0 {
			t.Fatalf("%s invalid defaults", name)
		}
		for _, input := range plan.Inputs() {
			if input.Source != "kline" || input.TimeFrame != "1d" {
				t.Fatal("subscription mismatch")
			}
		}
		if _, _, err := build(name, config(name, map[string]float64{"typo": 3})); err == nil {
			t.Fatal("unknown parameter accepted")
		}
	}
	n := Names()
	n[0] = "changed"
	if Names()[0] != "CSMomentum" {
		t.Fatal("mutable names")
	}
	for _, value := range []float64{-1, 0, .5, 10001, math.NaN(), math.Inf(1), 1e100} {
		if _, _, err := build("CSMomentum", config("CSMomentum", map[string]float64{"momentum_window": value})); err == nil {
			t.Fatalf("accepted window %g", value)
		}
	}
	for _, value := range []float64{-1, .1, math.Inf(-1)} {
		if _, _, err := build("CSMomentum", config("CSMomentum", map[string]float64{"skip": value})); err == nil {
			t.Fatal("accepted skip")
		}
	}
	if _, _, err := build("CSLowVol", config("CSLowVol", map[string]float64{"volatility_window": 1})); err == nil {
		t.Fatal("accepted one-bar volatility")
	}
	if _, _, err := build("CSMomentum", runner.Config{}); err == nil {
		t.Fatal("accepted absent stream")
	}
	baseline, _, err := build("CSMomentum", config("CSMomentum", nil))
	if err != nil {
		t.Fatal(err)
	}
	withK, _, err := build("CSMomentum", config("CSMomentum", map[string]float64{"k": 3}))
	if err != nil || withK.Hash() != baseline.Hash() {
		t.Fatal("portfolio k changed factor graph")
	}
	if _, _, err := build("CSMomentum", config("CSMomentum", map[string]float64{"k": 0})); err == nil {
		t.Fatal("accepted invalid k")
	}
}

func TestMomentumSkipIgnoresLatestReversalAndChangesIdentity(t *testing.T) {
	data := func(i int) map[int32]map[string]any {
		price := 100 + float64(i)*10
		if i == 4 {
			price = 50
		}
		return map[int32]map[string]any{1: {"close": price}, 2: {"close": 100.0}, 3: {"close": 100 - float64(i)}}
	}
	_, skipped := evaluate(t, "CSMomentum", map[string]float64{"momentum_window": 3, "skip": 1}, 5, data)
	_, current := evaluate(t, "CSMomentum", map[string]float64{"momentum_window": 3, "skip": 0}, 5, data)
	if skipped[1].Value <= skipped[2].Value || current[1].Value >= current[2].Value {
		t.Fatal("skip did not exclude last bar")
	}
	a, _, err := build("CSMomentum", config("CSMomentum", map[string]float64{"skip": 0}))
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := build("CSMomentum", config("CSMomentum", map[string]float64{"skip": 1}))
	if err != nil {
		t.Fatal(err)
	}
	if a.Hash() == b.Hash() {
		t.Fatal("different skip shares plan identity")
	}
}

func TestSessionClassicDirectionsAndStandardization(t *testing.T) {
	data := func(i int) map[int32]map[string]any {
		return map[int32]map[string]any{1: {"close": 100 * math.Pow(1.02, float64(i)), "volume": 1000.0}, 2: {"close": 100.0, "volume": 100.0}, 3: {"close": 100 * math.Pow(.98, float64(i)), "volume": 10.0}}
	}
	for _, name := range []string{"CSMomentum", "CSReversal", "CSTrend", "CSVolumeMomentum", "CSMultiFactor"} {
		frame, scores := evaluate(t, name, nil, 70, data)
		if scores[1].Validity != factor.Valid || scores[3].Validity != factor.Valid {
			t.Fatal("invalid mature scores")
		}
		if name == "CSReversal" {
			if scores[1].Value >= scores[3].Value {
				t.Fatal("reversal must favor losers")
			}
		} else if scores[1].Value <= scores[3].Value {
			t.Fatalf("%s must favor winners", name)
		}
		for _, column := range frame.Values {
			mean, variance := 0.0, 0.0
			for _, value := range column {
				mean += value.Value
				variance += value.Value * value.Value
			}
			if math.Abs(mean) > 1e-8 {
				t.Fatal("non-centered factor")
			}
			if variance > 1e-8 && math.Abs(variance/3-1) > 1e-8 {
				t.Fatal("non-standardized factor")
			}
		}
	}
	_, scores := evaluate(t, "CSLowVol", nil, 60, func(i int) map[int32]map[string]any {
		return map[int32]map[string]any{1: {"close": 100.0}, 2: {"close": 100 + 10*math.Sin(float64(i))}, 3: {"close": 100 + 20*math.Sin(float64(i))}}
	})
	if !(scores[1].Value > scores[2].Value && scores[2].Value > scores[3].Value) {
		t.Fatal("low volatility direction")
	}
}

func TestResidualNeutralizesLiquidity(t *testing.T) {
	data := func(i int) map[int32]map[string]any {
		rows := map[int32]map[string]any{}
		returns := []float64{.1, .3, .25, .6, .5}
		for j, r := range returns {
			price := 100 * (1 + r*float64(i)/3)
			rows[int32(j+1)] = map[string]any{"close": price, "volume": math.Exp(float64(j+1)) / price}
		}
		return rows
	}
	params := map[string]float64{"momentum_window": 3, "skip": 0}
	_, neutral := evaluate(t, "CSLiquidityNeutralMomentum", params, 4, data)
	_, ordinary := evaluate(t, "CSMomentum", params, 4, data)
	sum, exposure, different := 0.0, 0.0, false
	for sid, n := range neutral {
		if n.Validity != factor.Valid {
			t.Fatal("invalid neutral score")
		}
		sum += n.Value
		exposure += n.Value * float64(sid)
		if math.Abs(n.Value-ordinary[sid].Value) > 1e-5 {
			different = true
		}
	}
	if math.Abs(sum) > 1e-8 || math.Abs(exposure) > 1e-8 || !different {
		t.Fatalf("residual not neutral: sum=%g exposure=%g different=%v", sum, exposure, different)
	}
}

func TestInvalidObservationsRemainInvalid(t *testing.T) {
	for _, name := range []string{"CSVolumeMomentum", "CSLiquidityNeutralMomentum"} {
		for _, tc := range []struct {
			value   any
			missing bool
			want    factor.Validity
		}{{nil, false, factor.Null}, {nil, true, factor.Missing}, {"bad", false, factor.NotNumeric}, {0.0, false, factor.NonFinite}, {-1.0, false, factor.NonFinite}, {math.Inf(1), false, factor.NonFinite}} {
			_, scores := evaluate(t, name, map[string]float64{"momentum_window": 2, "skip": 0}, 5, func(i int) map[int32]map[string]any {
				rows := map[int32]map[string]any{}
				for sid := int32(1); sid <= 4; sid++ {
					rows[sid] = map[string]any{"close": 100 + float64(i*int(sid)), "volume": float64(100 * sid)}
				}
				if tc.missing {
					delete(rows[4], "volume")
				} else {
					rows[4]["volume"] = tc.value
				}
				return rows
			})
			if scores[4].Validity != tc.want {
				t.Fatalf("%s got %s want %s", name, scores[4].Validity, tc.want)
			}
		}
	}
	_, scores := evaluate(t, "CSMomentum", map[string]float64{"momentum_window": 2, "skip": 0}, 4, func(i int) map[int32]map[string]any {
		return map[int32]map[string]any{1: {"close": 100 + float64(i)}, 2: {"close": nil}, 3: {}}
	})
	if scores[2].Validity != factor.Null || scores[3].Validity != factor.Missing {
		t.Fatal("price NULL/missing lost")
	}
}
