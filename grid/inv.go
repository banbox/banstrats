package grid

import (
	"math"

	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/orm/ormo"
	"github.com/banbox/banbot/strat"
	ta "github.com/banbox/banta"
)

/*
Counter trend grid, buy down, sell up. Suitable for markets with strong volatility and unclear direction.
Using a certain proportion of large period atroBase as the unit size of the grid
Initial magnification: InitRate

反趋势网格，下跌买入，上涨卖出。适合波动性较强，方向不明的市场。

以大周期atrBase的一定比例作为网格的单位大小
初始倍率: InitRate

*/

type GridV1 struct {
	*Grid
	bigER float64 // 大周期ER系数
}

func InvGrid(pol *config.RunPolicyConfig) *strat.TradeStrat {
	lenAtr := 20
	baseAtrLen := int(float64(lenAtr) * 4.3)
	unitRate := pol.Def("unitRate", 1.3, core.PNorm(0.5, 5))
	initRate := int(pol.Def("initRate", 3, core.PNorm(0.7, 7.3)))
	maxAdd := 5
	return &strat.TradeStrat{
		WarmupNum:     100,
		StopEnterBars: 9999999,
		OnPairInfos: func(s *strat.StratJob) []*strat.PairSub {
			return []*strat.PairSub{
				{Pair: "_cur_", TimeFrame: "15m", WarmupNum: 100},
			}
		},
		OnStartUp: func(s *strat.StratJob) {
			s.More = &GridV1{Grid: NewGrid(initRate, maxAdd, 6, true)}
		},
		OnBar: func(s *strat.StratJob) {
			m, _ := s.More.(*GridV1)
			if s.OrderNum == 0 {
				if m.Unit <= 0 || math.IsNaN(m.Unit) || math.IsInf(m.Unit, 0) || m.bigER >= 0.3 || s.IsWarmUp {
					return
				}
				m.Open(s)
			} else {
				m.CheckPos(s)
			}
		},
		OnInfoBar: func(s *strat.StratJob, e *ta.BarEnv, pair, tf string) {
			m, _ := s.More.(*GridV1)
			m.bigER = ta.ER(e.Close, 20).Get(0)
			atr := ta.ATR(e.High, e.Low, e.Close, lenAtr)
			atrBase := ta.Lowest(ta.Highest(atr, lenAtr), baseAtrLen).Get(0)
			ma5 := ta.SMA(e.Close, 10).Get(0)
			ma20 := ta.SMA(e.Close, 50).Get(0)
			if s.OrderNum == 0 {
				// Update grid cell size when not in position
				// 未持仓时更新网格单元大小
				m.Unit = atrBase * unitRate
				m.Dirt = core.OdDirtShort
				if ma5 < ma20 {
					m.Dirt = core.OdDirtLong
				}
			}
		},
		OnOrderChange: func(s *strat.StratJob, od *ormo.InOutOrder, chgType int) {
			m, _ := s.More.(*GridV1)
			m.OnOrderChange(s, od, chgType)
		},
	}
}
