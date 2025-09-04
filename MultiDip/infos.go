package newstrategy4

import (
	ta "github.com/banbox/banta"
	"github.com/banbox/banbot/strat"
)

func onInfoBar1h(s *strat.StratJob, e *ta.BarEnv, pair, tf string) {
	m, _ := s.More.(*New4More)
	if tf != "1h" {
		return
	}
	m.Cmf1h = cmfFromEnv(e, 20)
	m.Rsi14_1h = ta.RSI(e.Close, 14).Get(0)
	m.Roc1h = rocrFromEnv(e, 168)
	m.R480_1h = williamsRFromEnv(e, 480)
	m.CtI40_1h = ta.CTI(e.Close, 40).Get(0)
	m.Ema200IsRising = ta.EMA(e.Close, 200).Get(0) > ta.EMA(e.Close, 200).Get(12)
	m.SupLevel1h = supLevelFromEnv(e, 5)
} 