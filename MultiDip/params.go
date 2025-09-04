package newstrategy4

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
)

// loadParams reads and returns all tunable params used by NewStrategy4
// Centralizing keeps strategy.go concise and consistent with ma/grid style.
type stratParams struct {
	BaseNbCandlesBuy  int
	BaseNbCandlesSell int

	LowOffset   float64
	HighOffset  float64
	HighOffset2 float64

	FastEwo int
	SlowEwo int

	InitialSafetyTrigger  float64
	MaxSafetyOrders       int
	SafetyOrderStepScale  float64
	SafetyOrderVolumeScale float64

	BuyBbDelta     float64
	BuyBbWidth     float64
	BuyClosedelta  float64
	BuyBbFactor    float64
	BuyRoc1h       float64

	SellFisher        float64
	SellBbMiddleClose float64
}

func loadParams(pol *config.RunPolicyConfig) stratParams {
	return stratParams{
		BaseNbCandlesBuy:  int(pol.Def("base_nb_candles_buy", 12, core.PNorm(8, 20))),
		BaseNbCandlesSell: int(pol.Def("base_nb_candles_sell", 22, core.PNorm(10, 40))),

		LowOffset:   pol.Def("low_offset", 0.985, core.PNorm(0.99, 0.003)),
		HighOffset:  pol.Def("high_offset", 1.014, core.PNorm(1.01, 0.003)),
		HighOffset2: pol.Def("high_offset_2", 1.01, core.PNorm(1.01, 0.003)),

		FastEwo: int(pol.Param("fast_ewo", 50)),
		SlowEwo: int(pol.Param("slow_ewo", 200)),

		InitialSafetyTrigger:  pol.Def("initial_safety_order_trigger", -0.018, core.PNorm(-0.02, 0.01)),
		MaxSafetyOrders:       int(pol.Def("max_safety_orders", 8, core.PNorm(3, 12))),
		SafetyOrderStepScale:  pol.Def("safety_order_step_scale", 1.2, core.PNorm(1.2, 0.5)),
		SafetyOrderVolumeScale: pol.Def("safety_order_volume_scale", 1.4, core.PNorm(1.2, 0.5)),

		BuyBbDelta:    pol.Def("buy_bb_delta", 0.025, core.PNorm(0.02, 0.01)),
		BuyBbWidth:    pol.Def("buy_bb_width", 0.095, core.PNorm(0.08, 0.05)),
		BuyClosedelta: pol.Def("buy_closedelta", 15.0, core.PNorm(12, 6)),
		BuyBbFactor:   pol.Def("buy_bb_factor", 0.995, core.PNorm(0.99, 0.005)),
		BuyRoc1h:      pol.Def("buy_roc_1h", 10.0, core.PNorm(5, 20)),

		SellFisher:        pol.Def("sell_fisher", 0.38, core.PNorm(0.3, 0.2)),
		SellBbMiddleClose: pol.Def("sell_bbmiddle_close", 1.07634, core.PNorm(1.05, 0.05)),
	}
} 