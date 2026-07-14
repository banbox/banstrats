package grid

import (
	"math"
	"testing"

	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
)

func TestGridOnOrderChangeHandlesMissingStopLoss(t *testing.T) {
	grid := NewGrid(1, 5, 6, false)
	grid.Unit = 2
	grid.OneAmt = 4
	grid.Dirt = 1

	order := &ormo.InOutOrder{
		IOrder: &ormo.IOrder{},
		Enter:  &ormo.ExOrder{Average: 100, Filled: 4},
	}
	grid.OnOrderChange(&strat.StratJob{}, order, strat.OdChgEnterFill)

	if grid.EntPrice != 100 {
		t.Fatalf("entry price = %v, want 100", grid.EntPrice)
	}
	if grid.HoldSize != 1 {
		t.Fatalf("hold size = %v, want 1", grid.HoldSize)
	}
	if got, want := grid.stopLossPrice(order), 88.0; math.Abs(got-want) > 1e-12 {
		t.Fatalf("fallback stop loss = %v, want %v", got, want)
	}
}
