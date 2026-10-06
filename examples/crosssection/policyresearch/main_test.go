package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/banbox/banbot/factor"
)

func TestSyntheticParameterResearchProposesPortfolio(t *testing.T) {
	var output bytes.Buffer
	if err := run(&output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Synthetic       bool
		ArtifactID      string
		Recommendation  string
		HoldingRules    map[int32]factor.HoldingRule
		TransitionRules map[int32]factor.TransitionRule
		Target          *factor.PortfolioTarget
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Synthetic || result.ArtifactID == "" || result.Recommendation != "long" || result.HoldingRules[2].MinBars != 8 || result.TransitionRules[2].ExitSteps != 8 {
		t.Fatalf("parameter research/resolution failed: %+v", result)
	}
	if result.Target == nil || result.Target.Allocations()[1].Value != "1" {
		t.Fatal("resolved policy did not allocate to the stronger synthetic score")
	}
}
