// This program uses synthetic training observations and proposes one portfolio.
// It has no account sink, database or exchange, and sends no orders.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/research"
	"github.com/banbox/banbot/factor/runner"
	"github.com/banbox/banstrats/examples/crosssection"
)

func run(writer io.Writer) error {
	observations := []research.ParameterObservation{}
	for episode := int64(0); episode < 4; episode++ {
		for _, candidate := range []string{"short", "long"} {
			netReturn := 0.01
			if candidate == "long" {
				netReturn = 0.02
			}
			observations = append(observations, research.ParameterObservation{SID: 1, Group: "liquid", Candidate: candidate, BeginAt: 1000 + episode*1000, EndAt: 1500 + episode*1000, AvailableAt: 1500 + episode*1000, NetReturn: netReturn})
		}
	}
	artifact, err := research.SelectParameters(observations, research.ParameterSelectionSpec{
		ManifestID: "synthetic-training-v1", AlgorithmVersion: "example-parameter-selection-v1",
		TrainStart: 1000, TrainEnd: 5000, AvailableAt: 6000,
		MinIndependentSamples: 2, PriorSamples: 4, ConfidencePenalty: 1,
		Candidates: map[string]map[string]float64{
			"short": {"min_bars": 4, "exit_steps": 4},
			"long":  {"min_bars": 8, "exit_steps": 8},
		},
	})
	if err != nil {
		return err
	}
	resolver, err := crosssection.NewParameterContext(artifact.ManifestID, []research.ParameterArtifact{artifact})
	if err != nil {
		return err
	}
	// Attach this callback to the Config you pass to runner.Run/RunMany.
	config := runner.Config{PolicyContext: resolver}
	spec := factor.PortfolioSpec{
		StrategyID: "synthetic-policy", AccountID: "synthetic-account", PlanSequence: 1,
		DecisionTime: 7000, ExecutableAt: 7001, ExpireAt: 8000,
		SnapshotID: "synthetic-snapshot", PlanHash: "synthetic-strategy-plan", FactorPlanHash: "synthetic-factor-plan",
		UniverseVersion: "synthetic-universe", Budget: factor.FrozenBudget{Version: "synthetic-budget", Currency: "USDT", NAV: 1000}, Mode: factor.Full,
	}
	c := factor.PortfolioContext{
		Spec: spec, GridTime: spec.DecisionTime, BarMillis: 7200000, ScoreName: "score",
		Universe: factor.Universe{Version: spec.UniverseVersion, Investable: []int32{1, 2}, Tradable: []int32{1, 2}},
		Frame:    factor.Frame{SnapshotID: spec.SnapshotID, PlanHash: spec.FactorPlanHash, DecisionTime: spec.DecisionTime, Values: map[string]map[int32]factor.Numeric{"score": {1: {Value: 2, Validity: factor.Valid}, 2: {Value: 1, Validity: factor.Valid}}}},
		Groups:   map[int32]string{1: "liquid", 2: "liquid"}, Marks: map[int32]float64{1: 100, 2: 100},
		AssetNames: map[int32]string{1: "SYNTHETIC-A", 2: "SYNTHETIC-B"}, SIDMappingVersion: "synthetic-sids-v1",
	}
	if err := config.PolicyContext(context.Background(), &c); err != nil {
		return err
	}
	policy, err := runner.NewPortfolioPolicy(factor.PortfolioPolicyConfig{
		Policy: "lifecycle-v1", LongNotional: 1, Selection: factor.SelectionConfig{LongK: 1},
		Transition: factor.TransitionConfig{Mode: "linear-exit", Basis: "quantity", ExitSteps: 4},
	})
	if err != nil {
		return err
	}
	proposal, err := policy.Propose(c, nil)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		Synthetic       bool
		ArtifactID      string
		Recommendation  string
		HoldingRules    map[int32]factor.HoldingRule
		TransitionRules map[int32]factor.TransitionRule
		Target          *factor.PortfolioTarget
	}{true, artifact.ID, artifact.Global.Candidate, c.HoldingRules, c.TransitionRules, proposal.Target})
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
