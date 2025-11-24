package ma

import (
	"github.com/banbox/banbot/config"
	"github.com/banbox/banbot/core"
	"github.com/banbox/banbot/strat"
)

type DCAState struct {
	LastInvestTime int64 // 上次定投时间
}

func DCA(pol *config.RunPolicyConfig) *strat.TradeStrat {
	// 定义可调超参数
	intvDays := pol.DefInt("intvDays", 7, core.PNorm(1, 365))
	intvMSecs := int64(intvDays) * 86400000

	return &strat.TradeStrat{
		WarmupNum: 1, // 定投策略不需要太多预热数据
		OnStartUp: func(s *strat.StratJob) {
			// 初始化每个品种的状态
			s.More = &DCAState{
				LastInvestTime: 0,
			}
		},

		OnBar: func(s *strat.StratJob) {
			state, _ := s.More.(*DCAState)
			currentTime := s.Env.TimeStop

			// 检查是否到了定投时间间隔
			if state.LastInvestTime > 0 && currentTime-state.LastInvestTime < intvMSecs {
				return // 还未到定投时间
			}

			// 执行定投买入
			err := s.OpenOrder(&strat.EnterReq{
				Tag: "buy",
				Log: true,
			})

			if err == nil {
				// 更新上次定投时间
				state.LastInvestTime = currentTime
			}
		},
	}
}
