package crosssection

import (
	"context"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	botconfig "github.com/banbox/banbot/config"
	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"github.com/go-viper/mapstructure/v2"
)

func decodeExample(t *testing.T, input any, output any) {
	t.Helper()
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{Result: output, ErrorUnused: true,
		MatchName: func(key, field string) bool { return strings.EqualFold(strings.ReplaceAll(key, "_", ""), field) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(input); err != nil {
		t.Fatal(err)
	}
}

func exampleContext(at int64, sequence uint64) factor.PortfolioContext {
	sids := []int32{}
	scores := map[int32]factor.Numeric{}
	marks := map[int32]float64{}
	names := map[int32]string{}
	for sid := int32(1); sid <= 24; sid++ {
		sids = append(sids, sid)
		scores[sid] = factor.Numeric{Value: float64(sid), Validity: factor.Valid}
		marks[sid] = 100
		names[sid] = "ASSET" + string(rune('A'+sid-1))
	}
	spec := factor.PortfolioSpec{StrategyID: "example", AccountID: "example", DecisionTime: at, ExecutableAt: at + 1, ExpireAt: at + 999, PlanSequence: sequence,
		SnapshotID: "synthetic-snapshot", PlanHash: "synthetic-strategy", FactorPlanHash: "synthetic-factor", UniverseVersion: "synthetic-universe", Budget: factor.FrozenBudget{Version: "synthetic-budget", Currency: "USDT", NAV: 1000}, Mode: factor.Full}
	return factor.PortfolioContext{Spec: spec, GridTime: at, BarMillis: 7200000, ScoreName: "score", SIDMappingVersion: "synthetic-v1", AssetNames: names, Marks: marks,
		Universe: factor.Universe{Version: spec.UniverseVersion, Investable: sids, Tradable: sids},
		Frame:    factor.Frame{SnapshotID: spec.SnapshotID, PlanHash: spec.FactorPlanHash, DecisionTime: at, Values: map[string]map[int32]factor.Numeric{"score": scores}}}
}

func TestLifecycleOverlays(t *testing.T) {
	paths, err := filepath.Glob("lifecycle/*.yml")
	if err != nil || len(paths) != 7 {
		t.Fatalf("missing lifecycle examples: %v %v", paths, err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			// The real unified loader checks overlays and whole-policy replacement.
			spec, loadErr := botconfig.LoadRunSpec(&botconfig.CmdArgs{NoDefault: true, DataDir: t.TempDir(), Configs: botconfig.ArrString{"runtime.yml", path}}, false)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			u := spec.Config()
			if len(u.RunPolicy) != 1 || u.RunPolicy[0].Engine != botconfig.EngineFactor || len(u.RunPolicy[0].RunTimeframes) != 1 || u.RunPolicy[0].RunTimeframes[0] != "2h" {
				t.Fatal("overlay failed to replace the daily strategy")
			}
			policy := u.RunPolicy[0]
			c := config(policy.Name, policy.Params)
			c.Factor.TimeFrame = "2h"
			decodeExample(t, policy.Factor["portfolio"], &c.Manifest.Portfolio)
			var researchConfig struct{ Labels []research.LabelSpec }
			decodeExample(t, policy.Factor["research"], &researchConfig)
			c.Manifest.Labels = researchConfig.Labels
			plan, combo, err := runner.CompileDefinition(c)
			if err != nil {
				t.Fatal(err)
			}
			pc, err := c.Manifest.Portfolio.PolicyConfig()
			if err != nil {
				t.Fatal(err)
			}
			p, err := runner.NewPortfolioPolicy(pc)
			if err != nil {
				t.Fatal(err)
			}
			manifest := c.Manifest
			manifest.Currency, manifest.CodeRevision, manifest.FactorPlanHash = "USDT", "synthetic-v1", plan.Hash()
			manifest.UniverseVersion, manifest.VisibilityPolicy = "synthetic-v1", "published-and-received"
			manifest.ExecutionMode, manifest.LatencyAssumption, manifest.Costs.FundingPolicy = "weights", "synthetic-1ms", "explicit-zero"
			manifest.Combo = combo
			if _, err := research.BuildManifest(manifest); err != nil {
				t.Fatal(err)
			}
			result, err := p.Propose(exampleContext(7200000, 1), nil)
			if err != nil || result.Target == nil {
				t.Fatalf("configured policy did not propose a target: %v %+v", err, result.Reasons)
			}
			if len(result.NextState) == 0 {
				t.Fatal("missing lifecycle checkpoint")
			}
			if filepath.Base(path) == "research-horizons.yml" {
				if len(manifest.Labels) != 3 || manifest.Labels[2].Horizon != 57600000 {
					t.Fatal("multiple research horizons were lost")
				}
			}
			if filepath.Base(path) == "selection-constraints.yml" {
				gross, net := 0.0, 0.0
				for _, allocation := range result.Target.Allocations() {
					value, err := strconv.ParseFloat(allocation.Value, 64)
					if err != nil {
						t.Fatal(err)
					}
					if math.Abs(value) > 0.15+1e-10 {
						t.Fatal("asset cap failed")
					}
					gross += math.Abs(value)
					net += value
				}
				if gross > 0.9+1e-10 || math.Abs(net) > 0.1+1e-10 {
					t.Fatal("reserve/net constraints failed")
				}
			}
		})
	}
}

func parameterArtifact(t *testing.T, values map[string]float64) research.ParameterArtifact {
	t.Helper()
	a, err := research.SelectParameters([]research.ParameterObservation{
		{SID: 1, Group: "liquid", Candidate: "rules", BeginAt: 1000, EndAt: 2000, AvailableAt: 2000, NetReturn: 0.01},
		{SID: 1, Group: "liquid", Candidate: "rules", BeginAt: 2000, EndAt: 3000, AvailableAt: 3000, NetReturn: 0.02},
	}, research.ParameterSelectionSpec{ManifestID: "training-v1", AlgorithmVersion: "example-v1", TrainStart: 1000, TrainEnd: 4000, AvailableAt: 5000, MinIndependentSamples: 2, Candidates: map[string]map[string]float64{"rules": values}})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestParameterContextVisibilityOwnershipAndValidation(t *testing.T) {
	a := parameterArtifact(t, map[string]float64{"min_bars": 8, "max_bars": 16, "exit_steps": 8})
	resolver, err := NewParameterContext("training-v1", []research.ParameterArtifact{a})
	if err != nil {
		t.Fatal(err)
	}
	a.Candidates["rules"]["min_bars"] = 99
	c := exampleContext(5000, 1)
	c.Groups = map[int32]string{2: "liquid"}
	c.Positions = map[int32]factor.PositionEvidence{25: {Quantity: "1"}}
	if err := resolver(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	for _, sid := range []int32{1, 2, 3, 25} { // asset, group, global, departed position
		if c.HoldingRules[sid].MinBars != 8 || c.TransitionRules[sid].ExitSteps != 8 {
			t.Fatalf("resolver did not freeze/fallback for SID %d", sid)
		}
	}
	c.GridTime = 4999
	if err := resolver(context.Background(), &c); err == nil {
		t.Fatal("future artifact became visible")
	}
	for _, values := range []map[string]float64{{"min_bars": 1.5, "exit_steps": 8}, {"min_bars": 9, "max_bars": 8, "exit_steps": 8}, {"min_bars": 8}, {"typo": 8, "exit_steps": 8}} {
		a := parameterArtifact(t, values)
		resolver, err := NewParameterContext(a.ManifestID, []research.ParameterArtifact{a})
		if err != nil {
			t.Fatal(err)
		}
		c := exampleContext(5000, 1)
		if err := resolver(context.Background(), &c); err == nil {
			t.Fatalf("accepted invalid rule parameters: %v", values)
		}
	}
	corrupt := parameterArtifact(t, map[string]float64{"exit_steps": 8})
	corrupt.ID = "tampered"
	resolver, err = NewParameterContext(corrupt.ManifestID, []research.ParameterArtifact{corrupt})
	if err != nil {
		t.Fatal(err)
	}
	c.GridTime = 5000
	if err := resolver(context.Background(), &c); err == nil {
		t.Fatal("corrupt research product was accepted")
	}
}

func TestParameterContextControlsFillAgeAndQuantityExit(t *testing.T) {
	artifact := parameterArtifact(t, map[string]float64{"min_bars": 8, "exit_steps": 8})
	resolver, err := NewParameterContext(artifact.ManifestID, []research.ParameterArtifact{artifact})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := runner.NewPortfolioPolicy(factor.PortfolioPolicyConfig{
		Policy: "lifecycle-v1", LongNotional: 1, Selection: factor.SelectionConfig{LongK: 1},
		Transition: factor.TransitionConfig{Mode: "linear-exit", Basis: "quantity", ExitSteps: 4, OnReselect: "finish"},
	})
	if err != nil {
		t.Fatal(err)
	}
	const entry = int64(5000)
	c := exampleContext(entry, 1)
	if err := resolver(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	initial, err := policy.Propose(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := initial.NextState
	// A verified fill is now present; the selected asset drops from rank 1.
	c = exampleContext(entry+7*7200000, 2)
	c.Frame.Values["score"][24] = factor.Numeric{Value: -100, Validity: factor.Valid}
	c.Positions = map[int32]factor.PositionEvidence{24: {Quantity: "8", FirstFillTime: entry, Quantum: "1"}}
	if err := resolver(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	protected, err := policy.Propose(c, state)
	if err != nil || protected.Target == nil || protected.Target.Allocations()[24].Value == "0" {
		t.Fatalf("resolver minimum did not protect a 14h holding: %v %+v", err, protected.Target)
	}
	state = protected.NextState
	for step := 1; step <= 8; step++ {
		c = exampleContext(entry+int64(7+step)*7200000, uint64(2+step))
		c.Frame.Values["score"][24] = factor.Numeric{Value: -100, Validity: factor.Valid}
		c.Positions = map[int32]factor.PositionEvidence{24: {Quantity: strconv.Itoa(9 - step), FirstFillTime: entry, Quantum: "1"}}
		if err := resolver(context.Background(), &c); err != nil {
			t.Fatal(err)
		}
		proposal, err := policy.Propose(c, state)
		if err != nil || proposal.Target == nil {
			t.Fatalf("exit round %d failed: %v", step, err)
		}
		allocation := proposal.Target.Allocations()[24]
		if allocation.Basis != factor.AbsoluteQuantity || allocation.Value != strconv.Itoa(8-step) {
			t.Fatalf("resolver exit round %d got %+v", step, allocation)
		}
		state = proposal.NextState
	}
	// Explicit asset configuration has final authority over researched rules.
	zero := 0
	overridePolicy, err := runner.NewPortfolioPolicy(factor.PortfolioPolicyConfig{
		Policy: "lifecycle-v1", LongNotional: 1, Selection: factor.SelectionConfig{LongK: 1},
		Holding:    factor.HoldingConfig{ByAsset: map[string]factor.HoldingOverride{"ASSETX": {MinBars: &zero}}},
		Transition: factor.TransitionConfig{Mode: "linear-exit", Basis: "quantity", ExitSteps: 4, ByAsset: map[string]factor.TransitionRule{"ASSETX": {ExitSteps: 2}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c = exampleContext(entry+7*7200000, 1)
	c.Frame.Values["score"][24] = factor.Numeric{Value: -100, Validity: factor.Valid}
	c.Positions = map[int32]factor.PositionEvidence{24: {Quantity: "8", FirstFillTime: entry, Quantum: "1"}}
	if err := resolver(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	proposal, err := overridePolicy.Propose(c, nil)
	if err != nil || proposal.Target == nil || proposal.Target.Allocations()[24].Value != "4" {
		t.Fatalf("explicit asset override did not beat resolver: %v %+v", err, proposal.Target)
	}
}
