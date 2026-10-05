package crosssection

import (
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	botconfig "github.com/banbox/banbot/config"
	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/expr"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"gopkg.in/yaml.v3"
)

func TestDefinitionParametersAndCombinationParity(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]float64
		badKey string
		badVal float64
	}{
		{"CSMomentum", map[string]float64{"momentum_window": 3, "skip": 0}, "momentum_window", 0},
		{"CSReversal", map[string]float64{"reversal_window": 2}, "reversal_window", .5},
		{"CSLowVol", map[string]float64{"volatility_window": 3}, "volatility_window", 1},
		{"CSTrend", map[string]float64{"trend_window": 3}, "trend_window", math.NaN()},
		{"CSVolumeMomentum", map[string]float64{"momentum_window": 3, "skip": 1}, "skip", -1},
		{"CSMultiFactor", map[string]float64{"momentum_window": 3, "skip": 1, "reversal_window": 2, "volatility_window": 3}, "volatility_window", 10001},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, params := range []map[string]float64{nil, tc.params} {
				goPlan, goCombo, err := runner.CompileDefinition(config(tc.name, params))
				if err != nil {
					t.Fatal(err)
				}
				exprPlan, exprCombo, err := runner.CompileDefinition(config(tc.name+"Expr", params))
				if err != nil {
					t.Fatal(err)
				}
				if goPlan.Hash() != exprPlan.Hash() || goPlan.WarmupLength() != exprPlan.WarmupLength() || !reflect.DeepEqual(goPlan.Inputs(), exprPlan.Inputs()) || !reflect.DeepEqual(goCombo, exprCombo) {
					t.Fatal("Go and expression definitions disagree on subscriptions, warmup or weights")
				}
			}
			for _, name := range []string{tc.name, tc.name + "Expr"} {
				for _, bad := range []map[string]float64{{tc.badKey: tc.badVal}, {"typo": 3}, {"k": 0}, {"k": math.Inf(1)}} {
					if _, _, err := runner.CompileDefinition(config(name, bad)); err == nil {
						t.Fatalf("%s accepted invalid parameters: %v", name, bad)
					}
				}
				baseline, combo, err := runner.CompileDefinition(config(name, nil))
				if err != nil {
					t.Fatal(err)
				}
				// Callers may edit their returned settings without affecting another run.
				for column := range combo.Weights {
					combo.Weights[column] = 99
				}
				withK, nextCombo, err := runner.CompileDefinition(config(name, map[string]float64{"k": 3}))
				if err != nil || withK.Hash() != baseline.Hash() {
					t.Fatalf("%s portfolio selection changed the factor graph: %v", name, err)
				}
				for _, weight := range nextCombo.Weights {
					if weight == 99 {
						t.Fatal("combination settings leaked across runs")
					}
				}
				c := config(name, tc.params)
				c.Factor.TimeFrame = ""
				if _, _, err := runner.CompileDefinition(c); err == nil {
					t.Fatal("accepted absent input timeframe")
				}
			}
		})
	}
}

// Compare both definitions at every bar, including warmup, invalid raw values
// and recovery; final rankings alone hide drift.
func TestExpressionsMatchGoGraphs(t *testing.T) {
	for _, name := range ExpressionNames() {
		t.Run(name, func(t *testing.T) {
			name = strings.TrimSuffix(name, "Expr")
			params := map[string]float64{}
			switch name {
			case "CSMomentum", "CSVolumeMomentum", "CSLiquidityNeutralMomentum":
				params = map[string]float64{"momentum_window": 3, "skip": 0}
			case "CSReversal":
				params["reversal_window"] = 2
			case "CSLowVol":
				params["volatility_window"] = 3
			case "CSTrend":
				params["trend_window"] = 3
			case "CSMultiFactor":
				params = map[string]float64{"momentum_window": 3, "skip": 1, "reversal_window": 2, "volatility_window": 3}
			}
			c := config(name, params)
			plan, combo, err := exprBuild(name, c)
			if err != nil {
				t.Fatal(err)
			}
			legacy, legacyCombo, err := build(name, c)
			if err != nil {
				t.Fatal(err)
			}
			if plan.WarmupLength() != legacy.WarmupLength() {
				t.Fatal("warmup changed")
			}
			session, err := factor.NewSession(plan)
			if err != nil {
				t.Fatal(err)
			}
			oldSession, err := factor.NewSession(legacy)
			if err != nil {
				t.Fatal(err)
			}
			for bar := 0; bar < 24; bar++ {
				rows := map[int32]map[string]any{}
				for sid := int32(1); sid <= 6; sid++ {
					rows[sid] = map[string]any{"close": 100 + float64(sid)*float64(bar) + math.Sin(float64(bar)*float64(sid)), "volume": 10 * float64(sid)}
				}
				if bar == 8 {
					rows[4]["close"] = nil
					rows[5]["volume"] = -1.0
					delete(rows[6], "close")
				}
				if bar == 9 {
					rows[5]["close"] = "invalid"
				}
				snap := snapshot(t, bar, rows)
				got, err := session.Evaluate(snap)
				if err != nil {
					t.Fatal(err)
				}
				want, err := oldSession.Evaluate(snap)
				if err != nil {
					t.Fatal(err)
				}
				for column, values := range want.Values {
					for sid, value := range values {
						assertNumeric(t, bar, column, sid, got.Values[column][sid], value)
					}
				}
				gotScores, _, err := research.Combine(got, snap.Spec().Universe, combo, nil)
				if err != nil {
					t.Fatal(err)
				}
				wantScores, _, err := research.Combine(want, snap.Spec().Universe, legacyCombo, nil)
				if err != nil {
					t.Fatal(err)
				}
				for sid, value := range wantScores {
					assertNumeric(t, bar, "score", sid, gotScores[sid], value)
				}
			}
		})
	}
}

func assertNumeric(t *testing.T, bar int, column string, sid int32, got, want factor.Numeric) {
	t.Helper()
	if got.Validity != want.Validity || (want.Validity == factor.Valid && math.Abs(got.Value-want.Value) > 1e-10+1e-8*math.Abs(want.Value)) {
		t.Fatalf("bar %d %s SID %d got %+v want %+v", bar, column, sid, got, want)
	}
}

func TestExpressionHandCalculationAndResearch(t *testing.T) {
	frame, scores := evaluate(t, "CSVolumeMomentumExpr", map[string]float64{"momentum_window": 1, "skip": 0}, 2, func(bar int) map[int32]map[string]any {
		rows := map[int32]map[string]any{}
		for sid := int32(1); sid <= 3; sid++ {
			price := 100.0
			if bar == 1 {
				price *= 1 + float64(sid-1)*.1
			}
			// Dollar volume = exp(SID), so its logs are exactly 1,2,3.
			rows[sid] = map[string]any{"close": price, "volume": math.Exp(float64(sid)) / price}
		}
		return rows
	})
	for sid := int32(1); sid <= 3; sid++ {
		// Population z-score of [0,.1,.2] and [1,2,3] is identical.
		want := float64(sid-2) * math.Sqrt(1.5)
		assertNumeric(t, 1, "momentum", sid, frame.Values["momentum"][sid], factor.Numeric{Value: want, Validity: factor.Valid})
		assertNumeric(t, 1, "liquidity", sid, frame.Values["liquidity"][sid], factor.Numeric{Value: want, Validity: factor.Valid})
		assertNumeric(t, 1, "score", sid, scores[sid], factor.Numeric{Value: want, Validity: factor.Valid})
	}
	var labels []research.Label
	for sid := int32(1); sid <= 3; sid++ {
		labels = append(labels, research.Label{Name: "forward", Kind: research.CloseToClose, SID: sid, DecisionTime: frame.DecisionTime, BeginAt: frame.DecisionTime, EndAt: frame.DecisionTime + 86400000, MatureAt: frame.DecisionTime + 86400000, AvailableAt: frame.DecisionTime + 86400000, Value: factor.Numeric{Value: float64(sid) * .01, Validity: factor.Valid}, Resolved: true})
	}
	report, err := research.Evaluate(frame, factor.Universe{Evaluation: []int32{1, 2, 3}, Static: true}, labels, research.EvaluationSpec{AsOf: frame.DecisionTime + 86400000})
	if err != nil {
		t.Fatal(err)
	}
	for name, metrics := range report.Columns {
		label := metrics.Labels["forward"]
		if metrics.Coverage != 1 || label.Pairs != 3 || label.RankIC.Validity != factor.Valid || math.Abs(label.RankIC.Value-1) > 1e-10 {
			t.Fatalf("%s research report: %+v", name, metrics)
		}
	}
}

func TestGoOverlay(t *testing.T) {
	raw, err := os.ReadFile("runtime.yml")
	if err != nil {
		t.Fatal(err)
	}
	u, parseErr := botconfig.ParseUnifiedYAML(raw, "runtime.yml")
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if len(u.RunPolicy) != 1 || u.RunPolicy[0].Engine != botconfig.EngineFactor {
		t.Fatal("missing Go factor strategy")
	}
	policy := u.RunPolicy[0]
	c := config(policy.Name, policy.Params)
	c.Factor.TimeFrame = policy.RunTimeframes[0]
	plan, combo, err := runner.CompileDefinition(c)
	if err != nil {
		t.Fatal(err)
	}
	if plan.WarmupLength() != 21 || len(plan.Outputs()) != 3 || len(combo.Columns) != 3 {
		t.Fatal("Go overlay lost factor settings")
	}
}

func TestExpressionOverlay(t *testing.T) {
	raw, err := os.ReadFile("expressions.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := botconfig.ParseUnifiedYAML(raw, "expressions.yml"); err != nil {
		t.Fatal(err)
	}
	var overlay struct {
		Policies []struct {
			Expressions expr.Spec `yaml:"expressions"`
		} `yaml:"run_policy"`
	}
	if err := yaml.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	if len(overlay.Policies) != 1 {
		t.Fatal("missing strategy")
	}
	plan, combo, err := runner.CompileDefinition(runner.Config{Expressions: &overlay.Policies[0].Expressions, DecisionInterval: 86400000})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Outputs()) != 2 || len(combo.Columns) != 2 {
		t.Fatal("overlay lost outputs or weights")
	}
	session, err := factor.NewSession(plan)
	if err != nil {
		t.Fatal(err)
	}
	var frame factor.Frame
	var snap *factor.Snapshot
	for bar := 0; bar < 45; bar++ {
		rows := map[int32]map[string]any{}
		for sid := int32(1); sid <= 4; sid++ {
			rows[sid] = map[string]any{"close": 100 + float64(sid)*float64(bar) + math.Sin(float64(bar)*float64(sid))}
		}
		if bar == 44 {
			rows[4]["close"] = nil
		}
		snap = snapshot(t, bar, rows)
		frame, err = session.Evaluate(snap)
		if err != nil {
			t.Fatal(err)
		}
	}
	scores, _, err := research.Combine(frame, snap.Spec().Universe, combo, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, sid := range []int32{1, 2, 3} {
		if scores[sid].Validity != factor.Valid {
			t.Fatalf("valid asset %d not scored", sid)
		}
	}
	if scores[4].Validity != factor.Null {
		t.Fatal("risk denominator floor filled NULL")
	}
}

func TestExpressionCustomPriceField(t *testing.T) {
	for _, field := range []string{"adjusted-close", "price\"quoted"} {
		c := config("CSMomentum", map[string]float64{"momentum_window": 1, "skip": 0})
		c.Factor.Field = field
		plan, _, err := exprBuild("CSMomentum", c)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Inputs()) != 1 || len(plan.Inputs()[0].Fields) != 1 || plan.Inputs()[0].Fields[0] != field {
			t.Fatalf("field binding lost %q: %+v", field, plan.Inputs())
		}
		session, err := factor.NewSession(plan)
		if err != nil {
			t.Fatal(err)
		}
		for bar := 0; bar < 2; bar++ {
			frame, err := session.Evaluate(snapshot(t, bar, map[int32]map[string]any{1: {field: 100.0 + 10*float64(bar)}, 2: {field: 100.0}, 3: {field: nil}}))
			if err != nil {
				t.Fatal(err)
			}
			if bar == 1 && (frame.Values["momentum"][1].Validity != factor.Valid || frame.Values["momentum"][1].Value <= 0 || frame.Values["momentum"][3].Validity != factor.Null) {
				t.Fatalf("custom field semantics changed: %+v", frame.Values)
			}
		}
	}
}
