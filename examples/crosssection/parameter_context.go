package crosssection

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/banbox/banbot/factor"
	"github.com/banbox/banbot/factor/research"
)

// NewParameterContext freezes research products for one runner.Config.PolicyContext.
// ResolveParameters checks the manifest, hash, training cutoff and publication
// time on each decision. Missing/corrupt products return an error, so this example
// never silently trades with an unvalidated recommendation.
// Explicit portfolio.holding/transition.by_asset settings still take precedence.
func NewParameterContext(manifestID string, artifacts []research.ParameterArtifact) (func(context.Context, *factor.PortfolioContext) error, error) {
	if manifestID == "" || len(artifacts) == 0 {
		return nil, fmt.Errorf("crosssection: parameter resolver needs a training manifest and artifacts")
	}
	raw, err := json.Marshal(artifacts)
	if err != nil {
		return nil, err
	}
	var owned []research.ParameterArtifact
	if err := json.Unmarshal(raw, &owned); err != nil {
		return nil, err
	}
	return func(ctx context.Context, c *factor.PortfolioContext) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if c == nil {
			return fmt.Errorf("crosssection: nil portfolio context")
		}
		artifact, err := research.ResolveParameters(owned, c.GridTime, manifestID)
		if err != nil {
			return err
		}
		holding := map[int32]factor.HoldingRule{}
		transition := map[int32]factor.TransitionRule{}
		// Include positions that have left the selection pool; their exit rules
		// still matter. The runner maintains these execution subscriptions.
		sids := map[int32]bool{}
		for _, sid := range c.Universe.Investable {
			sids[sid] = true
		}
		for sid := range c.Positions {
			sids[sid] = true
		}
		for sid := range sids {
			recommendation := artifact.Global
			if group, ok := artifact.ByGroup[c.Groups[sid]]; ok {
				recommendation = group
			}
			if asset, ok := artifact.ByAsset[sid]; ok {
				recommendation = asset
			}
			parameters, ok := artifact.Candidates[recommendation.Candidate]
			if !ok {
				return fmt.Errorf("crosssection: candidate %q has no declared parameters", recommendation.Candidate)
			}
			for key := range parameters {
				if key != "min_bars" && key != "max_bars" && key != "exit_steps" {
					return fmt.Errorf("crosssection: unsupported lifecycle parameter %q", key)
				}
			}
			barCount := func(key string, minimum int) (int, error) {
				value, exists := parameters[key]
				if !exists && minimum > 0 || math.IsNaN(value) || math.IsInf(value, 0) || value < float64(minimum) || value > 10000 || math.Trunc(value) != value {
					return 0, fmt.Errorf("crosssection: %s must be an integer in [%d,10000]", key, minimum)
				}
				return int(value), nil
			}
			minBars, err := barCount("min_bars", 0)
			if err != nil {
				return err
			}
			maxBars, err := barCount("max_bars", 0)
			if err != nil {
				return err
			}
			steps, err := barCount("exit_steps", 1)
			if err != nil {
				return err
			}
			if maxBars > 0 && minBars > maxBars {
				return fmt.Errorf("crosssection: minimum holding exceeds maximum for SID %d", sid)
			}
			holding[sid] = factor.HoldingRule{MinBars: minBars, MaxBars: maxBars}
			transition[sid] = factor.TransitionRule{ExitSteps: steps}
		}
		c.HoldingRules, c.TransitionRules = holding, transition
		return nil
	}, nil
}
