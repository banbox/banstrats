package ma

import (
	"testing"

	"github.com/banbox/banbot/strat"
)

func TestCalcCorrsIgnoresIncompleteBatch(t *testing.T) {
	job := &strat.StratJob{More: &BatchSta{}}
	calcCorrs([]*strat.StratJob{job}, false)
	state, _ := job.More.(*BatchSta)
	if state.smlCorrReady || state.bigCorrReady {
		t.Fatalf("incomplete batch must not mark correlation ready: %+v", state)
	}
}
