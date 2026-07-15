package ma

import (
	"testing"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/orm"
	"github.com/banbox/banbot/strat"
)

func TestStrategiesUseOnDataOnly(t *testing.T) {
	factories := map[string]strat.FuncMakeStrat{
		"demo": Demo, "demo_er": DemoER, "demo2": DemoInfo, "demo_batch": BatchDemo,
		"dca": DCA, "openClose": openClose, "demo_exit": CustomExitDemo,
		"edit_pairs": editPairs, "trail_stop": TrailStop, "postApi": PostApi,
		"ws": ws, "stoploss": stoploss, "takeprofit": takeprofit,
		"draw_down": DrawDown, "ta_undo": taUndo,
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			stgy := factory(&config.RunPolicyConfig{})
			if stgy.OnData == nil || stgy.OnBar != nil || stgy.OnInfoBar != nil {
				t.Fatalf("callbacks not fully migrated: OnData=%v OnBar=%v OnInfoBar=%v",
					stgy.OnData != nil, stgy.OnBar != nil, stgy.OnInfoBar != nil)
			}
		})
	}
}

func setTestData(job *strat.StratJob, source string, sid int32, tf string, timeMS int64, price float64) *strat.DataFields {
	return job.SetData(&orm.DataSeries{
		Source: source, Sid: sid, TimeFrame: tf, TimeMS: timeMS, EndMS: timeMS + 1, Closed: true,
		Values: map[string]any{
			"open": price, "high": price, "low": price, "close": price, "volume": 1.0,
		},
	})
}
