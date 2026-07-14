package grid

import (
	"testing"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/strat"
)

func TestInvGridWaitsForPositiveUnit(t *testing.T) {
	stgy := InvGrid(&config.RunPolicyConfig{})
	job := &strat.StratJob{
		More: &GridV1{Grid: NewGrid(1, 5, 6, false)},
	}

	stgy.OnBar(job)

	if len(job.Entrys) != 0 {
		t.Fatalf("grid opened before its unit was initialized: %+v", job.Entrys)
	}
}
